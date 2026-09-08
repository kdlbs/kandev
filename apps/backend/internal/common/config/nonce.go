package config

// nonceFingerprintLength is the number of leading hex characters of a
// bootstrap nonce kept for diagnostics. Long enough to distinguish nonces
// across a handful of concurrent launches, short enough that logging it never
// exposes anything close to the full secret.
const nonceFingerprintLength = 8

// NonceFingerprint returns the first nonceFingerprintLength hex characters of
// a hex-encoded bootstrap nonce, safe to log on both the backend launcher and
// the agentctl control server. It lets a rejected handshake be correlated
// against the nonce each side actually used without ever logging the nonce
// itself.
func NonceFingerprint(nonce string) string {
	if len(nonce) <= nonceFingerprintLength {
		return nonce
	}
	return nonce[:nonceFingerprintLength]
}
