package tui

import (
	"bytes"
	"fmt"
	"testing"
	"time"

	"chatbox/internal/historymeta"
	"chatbox/internal/identity"
	"chatbox/internal/transcript"
)

func BenchmarkReplayHistoricalWindowLargeChunk(b *testing.B) {
	const recordCount = 5000

	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	records := make([]transcript.Record, 0, recordCount)
	for i := 0; i < recordCount; i++ {
		records = append(records, transcript.Record{
			MessageID:      fmt.Sprintf("bench-%05d", i),
			Direction:      transcript.DirectionIncoming,
			From:           "bob",
			AuthorIdentity: "identity-bob",
			Body:           fmt.Sprintf("historical message %05d", i),
			At:             joinedAt.Add(time.Duration(i) * time.Second),
			Status:         transcript.StatusSent,
		})
	}

	for i := 0; i < b.N; i++ {
		store := &fakeTranscriptStore{}
		uiModel := newModel(modelOptions{
			mode:          "join",
			uiMode:        uiModeScrollback,
			listeningAddr: "203.0.113.10:7331",
			transcriptOpener: func(string) (transcriptStore, error) {
				return store, nil
			},
			identityLoader: func() (identity.Store, error) {
				return identity.Store{IdentityID: "identity-local"}, nil
			},
			roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
				return historymeta.Record{
					RoomKey:    roomKey,
					IdentityID: identityID,
					JoinedAt:   joinedAt,
				}, nil
			},
		})
		uiModel.identityID = "identity-local"
		uiModel.roomAuthorization = historymeta.Record{
			RoomKey:    "join:203.0.113.10:7331",
			IdentityID: "identity-local",
			JoinedAt:   joinedAt,
		}
		uiModel.transcript = store
		uiModel.transcriptConversationKey = "join:203.0.113.10:7331"

		if !uiModel.replayHistoricalWindow("join:203.0.113.10:7331", "identity-local", "identity-bob", records, nil, false) {
			b.Fatal("expected historical replay to be accepted")
		}
		if got := len(store.appends); got != recordCount {
			b.Fatalf("expected %d appended records, got %d", recordCount, got)
		}
	}
}

func BenchmarkReplayHistoricalWindowLargeChunkWithTranscriptStore(b *testing.B) {
	const recordCount = 5000

	joinedAt := time.Date(2026, 4, 20, 20, 0, 0, 0, time.UTC)
	records := make([]transcript.Record, 0, recordCount)
	for i := 0; i < recordCount; i++ {
		records = append(records, transcript.Record{
			MessageID:      fmt.Sprintf("bench-real-%05d", i),
			Direction:      transcript.DirectionIncoming,
			From:           "bob",
			AuthorIdentity: "identity-bob",
			Body:           fmt.Sprintf("historical message %05d", i),
			At:             joinedAt.Add(time.Duration(i) * time.Second),
			Status:         transcript.StatusSent,
		})
	}

	for i := 0; i < b.N; i++ {
		store, err := transcript.OpenStore(b.TempDir(), "alice", "join:203.0.113.10:7331", bytes.Repeat([]byte{0x42}, 32))
		if err != nil {
			b.Fatalf("open transcript store: %v", err)
		}
		uiModel := newModel(modelOptions{
			mode:          "join",
			uiMode:        uiModeScrollback,
			listeningAddr: "203.0.113.10:7331",
			transcriptOpener: func(string) (transcriptStore, error) {
				return store, nil
			},
			identityLoader: func() (identity.Store, error) {
				return identity.Store{IdentityID: "identity-local"}, nil
			},
			roomAuthLoader: func(roomKey, identityID string) (historymeta.Record, error) {
				return historymeta.Record{
					RoomKey:    roomKey,
					IdentityID: identityID,
					JoinedAt:   joinedAt,
				}, nil
			},
		})
		uiModel.identityID = "identity-local"
		uiModel.roomAuthorization = historymeta.Record{
			RoomKey:    "join:203.0.113.10:7331",
			IdentityID: "identity-local",
			JoinedAt:   joinedAt,
		}
		uiModel.transcript = store
		uiModel.transcriptConversationKey = "join:203.0.113.10:7331"

		if !uiModel.replayHistoricalWindow("join:203.0.113.10:7331", "identity-local", "identity-bob", records, nil, false) {
			b.Fatal("expected historical replay to be accepted")
		}
	}
}
