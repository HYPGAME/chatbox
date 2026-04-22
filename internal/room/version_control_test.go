package room

import "testing"

func TestVersionAnnounceRoundTrip(t *testing.T) {
	t.Parallel()

	body := VersionAnnounceBody(VersionAnnounce{
		Version:       1,
		ClientVersion: "v0.1.31",
	})

	parsed, ok := ParseVersionAnnounce(body)
	if !ok {
		t.Fatalf("expected version announce to parse, got %q", body)
	}
	if parsed.Version != 1 || parsed.ClientVersion != "v0.1.31" {
		t.Fatalf("unexpected parsed version announce: %#v", parsed)
	}
}

func TestParseVersionAnnounceRejectsOtherBodies(t *testing.T) {
	t.Parallel()

	if _, ok := ParseVersionAnnounce(StatusRequestBody()); ok {
		t.Fatal("expected status request not to parse as version announce")
	}
	if _, ok := ParseVersionAnnounce("version"); ok {
		t.Fatal("expected arbitrary body not to parse as version announce")
	}
}
