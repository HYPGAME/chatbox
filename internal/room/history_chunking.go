package room

import (
	"fmt"
	"strings"
	"time"

	"chatbox/internal/session"
	"chatbox/internal/transcript"
)

type transcriptWindowChunk struct {
	Records []transcript.Record
	Revokes []transcript.RevokeRecord
}

func SplitHistorySyncChunks(sourceIdentity, targetIdentity, roomKey, from string, at time.Time, records []transcript.Record, revokes []transcript.RevokeRecord, maxMessageSize int) ([]HistorySyncChunk, error) {
	chunks, err := splitTranscriptWindowByMessageSize(from, at, maxMessageSize, records, revokes, func(records []transcript.Record, revokes []transcript.RevokeRecord) string {
		return HistorySyncChunkBody(HistorySyncChunk{
			Version:        1,
			SourceIdentity: sourceIdentity,
			TargetIdentity: targetIdentity,
			RoomKey:        roomKey,
			Records:        records,
			Revokes:        revokes,
		})
	})
	if err != nil {
		return nil, err
	}

	result := make([]HistorySyncChunk, 0, len(chunks))
	for _, chunk := range chunks {
		result = append(result, HistorySyncChunk{
			Version:        1,
			SourceIdentity: sourceIdentity,
			TargetIdentity: targetIdentity,
			RoomKey:        roomKey,
			Records:        chunk.Records,
			Revokes:        chunk.Revokes,
		})
	}
	return result, nil
}

func SplitHostHistoryChunks(targetIdentity, roomKey, from string, at time.Time, records []transcript.Record, revokes []transcript.RevokeRecord, maxMessageSize int) ([]HostHistoryChunk, error) {
	chunks, err := splitTranscriptWindowByMessageSize(from, at, maxMessageSize, records, revokes, func(records []transcript.Record, revokes []transcript.RevokeRecord) string {
		return HostHistoryChunkBody(HostHistoryChunk{
			Version:        1,
			RoomKey:        roomKey,
			TargetIdentity: targetIdentity,
			Records:        records,
			Revokes:        revokes,
		})
	})
	if err != nil {
		return nil, err
	}

	result := make([]HostHistoryChunk, 0, len(chunks))
	for _, chunk := range chunks {
		result = append(result, HostHistoryChunk{
			Version:        1,
			RoomKey:        roomKey,
			TargetIdentity: targetIdentity,
			Records:        chunk.Records,
			Revokes:        chunk.Revokes,
		})
	}
	return result, nil
}

func splitTranscriptWindowByMessageSize(from string, at time.Time, maxMessageSize int, records []transcript.Record, revokes []transcript.RevokeRecord, encodeBody func([]transcript.Record, []transcript.RevokeRecord) string) ([]transcriptWindowChunk, error) {
	if maxMessageSize <= 0 {
		maxMessageSize = session.DefaultMaxMessageSize()
	}
	if strings.TrimSpace(from) == "" {
		from = "chatbox"
	}

	revokeByMessageID := make(map[string][]transcript.RevokeRecord, len(revokes))
	orderedStandaloneRevokes := make([]transcript.RevokeRecord, 0, len(revokes))
	recordIDs := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record.MessageID != "" {
			recordIDs[record.MessageID] = struct{}{}
		}
	}
	for _, revoke := range revokes {
		if _, ok := recordIDs[revoke.TargetMessageID]; ok {
			revokeByMessageID[revoke.TargetMessageID] = append(revokeByMessageID[revoke.TargetMessageID], revoke)
			continue
		}
		orderedStandaloneRevokes = append(orderedStandaloneRevokes, revoke)
	}

	chunks := make([]transcriptWindowChunk, 0, len(records))
	currentRecords := make([]transcript.Record, 0, len(records))
	currentRevokes := make([]transcript.RevokeRecord, 0, len(revokes))

	flush := func() {
		if len(currentRecords) == 0 && len(currentRevokes) == 0 {
			return
		}
		chunks = append(chunks, transcriptWindowChunk{
			Records: append([]transcript.Record(nil), currentRecords...),
			Revokes: append([]transcript.RevokeRecord(nil), currentRevokes...),
		})
		currentRecords = currentRecords[:0]
		currentRevokes = currentRevokes[:0]
	}

	fits := func(candidateRecords []transcript.Record, candidateRevokes []transcript.RevokeRecord) bool {
		body := encodeBody(candidateRecords, candidateRevokes)
		size, err := session.PayloadSize(session.Message{
			ID:   strings.Repeat("0", 32),
			From: from,
			Body: body,
			At:   at,
		})
		return err == nil && size <= maxMessageSize
	}

	for _, record := range records {
		candidateRecords := append(append([]transcript.Record(nil), currentRecords...), record)
		candidateRevokes := append([]transcript.RevokeRecord(nil), currentRevokes...)
		if related := revokeByMessageID[record.MessageID]; len(related) > 0 {
			candidateRevokes = append(candidateRevokes, related...)
		}
		if fits(candidateRecords, candidateRevokes) {
			currentRecords = append(currentRecords, record)
			if related := revokeByMessageID[record.MessageID]; len(related) > 0 {
				currentRevokes = append(currentRevokes, related...)
			}
			continue
		}
		if len(currentRecords) == 0 && len(currentRevokes) == 0 {
			return nil, fmt.Errorf("history sync record %q exceeds %d bytes", record.MessageID, maxMessageSize)
		}
		flush()
		candidateRecords = []transcript.Record{record}
		candidateRevokes = candidateRevokes[:0]
		if related := revokeByMessageID[record.MessageID]; len(related) > 0 {
			candidateRevokes = append(candidateRevokes, related...)
		}
		if !fits(candidateRecords, candidateRevokes) {
			return nil, fmt.Errorf("history sync record %q exceeds %d bytes", record.MessageID, maxMessageSize)
		}
		currentRecords = append(currentRecords, record)
		currentRevokes = append(currentRevokes, candidateRevokes...)
	}

	for _, revoke := range orderedStandaloneRevokes {
		candidateRevokes := append(append([]transcript.RevokeRecord(nil), currentRevokes...), revoke)
		if fits(currentRecords, candidateRevokes) {
			currentRevokes = append(currentRevokes, revoke)
			continue
		}
		if len(currentRecords) == 0 && len(currentRevokes) == 0 {
			return nil, fmt.Errorf("history sync revoke %q exceeds %d bytes", revoke.TargetMessageID, maxMessageSize)
		}
		flush()
		if !fits(nil, []transcript.RevokeRecord{revoke}) {
			return nil, fmt.Errorf("history sync revoke %q exceeds %d bytes", revoke.TargetMessageID, maxMessageSize)
		}
		currentRevokes = append(currentRevokes, revoke)
	}

	flush()
	return chunks, nil
}
