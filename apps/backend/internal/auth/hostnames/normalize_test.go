package hostnames

import "testing"

func TestNormalizeHostnameUsesFirstValidRecord(t *testing.T) {
	got := NormalizeHostname([]string{"10.2.0.192.in-addr.arpa.", "mail.example.test."})
	if got != "mail.example.test" {
		t.Fatalf("NormalizeHostname() = %q, want %q", got, "mail.example.test")
	}
}

func TestNormalizeHostname(t *testing.T) {
	tests := []struct {
		name  string
		input []string
		want  string
	}{
		{
			name:  "empty input",
			input: nil,
			want:  "",
		},
		{
			name:  "all reverse-DNS records",
			input: []string{"10.2.0.192.in-addr.arpa.", "2001:db8::1.ip6.arpa."},
			want:  "",
		},
		{
			name:  "skips arpa prefix then returns the first valid record",
			input: []string{"10.2.0.192.in-addr.arpa.", "mail.example.test."},
			want:  "mail.example.test",
		},
		{
			name:  "IPv6 ip6.arpa suffix is filtered",
			input: []string{"1.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.0.ip6.arpa.", "web.example.test"},
			want:  "web.example.test",
		},
		{
			name:  "single valid entry",
			input: []string{"mail.example.test."},
			want:  "mail.example.test",
		},
		{
			name:  "trailing-dot-only entry is skipped",
			input: []string{".", "host.example.test."},
			want:  "host.example.test",
		},
		{
			name:  "strips trailing dot and trims whitespace",
			input: []string{"  mail.example.test.  "},
			want:  "mail.example.test",
		},
		{
			name:  "case-insensitive arpa detection",
			input: []string{"10.2.0.192.IN-ADDR.ARPA.", "mail.example.test."},
			want:  "mail.example.test",
		},
		{
			name:  "invalid records produce an empty result",
			input: []string{".", "  "},
			want:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeHostname(tt.input); got != tt.want {
				t.Fatalf("NormalizeHostname() = %q, want %q", got, tt.want)
			}
		})
	}
}
