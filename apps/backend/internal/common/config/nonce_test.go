package config

import "testing"

func TestNonceFingerprint(t *testing.T) {
	cases := []struct {
		name  string
		nonce string
		want  string
	}{
		{
			name:  "32-byte hex nonce keeps only the first 8 characters",
			nonce: "0123456789abcdef0123456789abcdef",
			want:  "01234567",
		},
		{
			name:  "nonce shorter than the fingerprint length is returned unchanged",
			nonce: "abc123",
			want:  "abc123",
		},
		{
			name:  "nonce exactly the fingerprint length is returned unchanged",
			nonce: "abcdef01",
			want:  "abcdef01",
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

// TestNonceFingerprintNeverExposesFullSecret pins the truncation itself: two
// nonces that only differ after the fingerprint length must fingerprint
// identically, proving the fingerprint cannot be used to reconstruct or
// distinguish the full secret.
func TestNonceFingerprintNeverExposesFullSecret(t *testing.T) {
	a := "deadbeef0000000000000000000000000000000000000000000000000000"
	b := "deadbeefffffffffffffffffffffffffffffffffffffffffffffffffffffff"

	fa, fb := NonceFingerprint(a), NonceFingerprint(b)
	if fa != fb {
		t.Fatalf("fingerprints diverged past the truncation point: %q vs %q", fa, fb)
	}
	if len(fa) != nonceFingerprintLength {
		t.Fatalf("fingerprint length = %d, want %d", len(fa), nonceFingerprintLength)
	}
}
