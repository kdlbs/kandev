package lifecycle

import "testing"

func TestOpenSSHRuntimeAPITunnel_LeavesReachableURLsUnchanged(t *testing.T) {
	const rawURL = "https://kandev.example.test/api/v1"

	tunnel, gotURL, err := openSSHRuntimeAPITunnel(nil, rawURL)
	if err != nil {
		t.Fatalf("openSSHRuntimeAPITunnel: %v", err)
	}
	if tunnel != nil {
		t.Fatal("reachable URL unexpectedly created a tunnel")
	}
	if gotURL != rawURL {
		t.Fatalf("URL = %q, want %q", gotURL, rawURL)
	}
}

func TestOpenSSHRuntimeAPITunnel_RequiresSSHClientForLoopbackURL(t *testing.T) {
	_, _, err := openSSHRuntimeAPITunnel(nil, "http://127.0.0.1:38429/api/v1")
	if err == nil {
		t.Fatal("loopback URL without SSH client unexpectedly succeeded")
	}
}
