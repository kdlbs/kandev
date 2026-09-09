package config

import "testing"

func TestNonceFingerprint(t *testing.T) {
	cases := []struct {
		name  string
		nonce string
		want  string
	}{
		{
			name:  "32-byte hex nonce hashes to a short digest",
			nonce: "0123456789abcdef0123456789abcdef",
			want:  "3eb1bd43",
		},
		{
			name:  "nonce shorter than the fingerprint length is hashed",
			nonce: "abc123",
			want:  "6ca13d52",
		},
		{
			name:  "nonce exactly the fingerprint length is hashed",
			nonce: "abcdef01",
			want:  "aa280d2e",
		},
		{
			name:  "empty nonce fingerprints to empty",
			nonce: "",
			want:  "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := NonceFingerprint(tc.nonce); got != tc.want {
				t.Fatalf("NonceFingerprint(%q) = %q, want %q", tc.nonce, got, tc.want)
			}
		})
	}
}

func TestNonceFingerprintDoesNotExposeNoncePrefix(t *testing.T) {
	nonce := "deadbeef0000000000000000000000000000000000000000000000000000"
	got := NonceFingerprint(nonce)
	if got == nonce[:nonceFingerprintLength] {
		t.Fatalf("fingerprint %q exposes the nonce prefix", got)
	}
	if len(got) != nonceFingerprintLength {
		t.Fatalf("fingerprint length = %d, want %d", len(got), nonceFingerprintLength)
	}
}
