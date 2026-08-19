package hostnames

import "testing"

func TestNormalizeHostnameUsesFirstValidRecord(t *testing.T) {
	got := NormalizeHostname([]string{"10.2.0.192.in-addr.arpa.", "mail.example.test."})
	if got != "mail.example.test" {
		t.Fatalf("NormalizeHostname() = %q, want %q", got, "mail.example.test")
	}
}
