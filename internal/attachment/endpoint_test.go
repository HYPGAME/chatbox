package attachment

import "testing"

func TestBaseURLFromPeer(t *testing.T) {
	got, err := BaseURLFromPeer("10.77.1.4:7331")
	if err != nil {
		t.Fatalf("BaseURLFromPeer returned error: %v", err)
	}
	if got != "http://10.77.1.4:7332" {
		t.Fatalf("expected peer base url %q, got %q", "http://10.77.1.4:7332", got)
	}
}

func TestBaseURLFromListenAddrUsesLoopbackForWildcardHost(t *testing.T) {
	got, err := BaseURLFromListenAddr("0.0.0.0:7331")
	if err != nil {
		t.Fatalf("BaseURLFromListenAddr returned error: %v", err)
	}
	if got != "http://127.0.0.1:7332" {
		t.Fatalf("expected local base url %q, got %q", "http://127.0.0.1:7332", got)
	}
}
