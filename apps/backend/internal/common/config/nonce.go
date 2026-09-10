package config

import (
	"crypto/sha256"
	"encoding/hex"
)

// nonceFingerprintLength is the number of leading hex characters of a
// SHA-256 digest kept for diagnostics.
const nonceFingerprintLength = 8

// NonceFingerprint returns a short SHA-256 digest of the bootstrap nonce,
// safe to log on both the backend launcher and the agentctl control server.
// It lets a rejected handshake be correlated against the nonce each side used
// without logging any characters from the nonce itself.
func NonceFingerprint(nonce string) string {
	if nonce == "" {
		return ""
	}
	digest := sha256.Sum256([]byte(nonce))
	encoded := hex.EncodeToString(digest[:])
	return encoded[:nonceFingerprintLength]
}
