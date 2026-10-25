// Package sessionmcp derives and fingerprints the per-instance MCP bearer
// token shared by the backend and agentctl.
package sessionmcp

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
)

// KeyLength is the required length of the backend signing key in bytes.
const KeyLength = 32

const (
	tokenDomain       = "kandev-mcp-v1:"
	fingerprintDomain = "kandev-mcp-fp-v1:"
	brokerDomain      = "kandev-broker-fp-v1:"
	nonceBytes        = 16
)

// NewNonce returns a fresh hex-encoded 16-byte nonce.
func NewNonce() (string, error) {
	b := make([]byte, nonceBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("mint session mcp nonce: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// Token is base64url-unpadded HMAC-SHA256(key, "kandev-mcp-v1:" + instanceID +
// ":" + nonce). It returns "" when the key is not exactly KeyLength bytes.
func Token(key []byte, instanceID, nonce string) string {
	if len(key) != KeyLength {
		return ""
	}
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(tokenDomain + instanceID + ":" + nonce))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// Fingerprint is base64url(SHA-256("kandev-mcp-fp-v1:" + token)), unpadded, or
// "" for an empty token.
func Fingerprint(token string) string {
	if token == "" {
		return ""
	}
	return digest(fingerprintDomain + token)
}

// BrokerFingerprint fingerprints the upstream broker URL, "" when none.
func BrokerFingerprint(upstream string) string {
	if upstream == "" {
		return ""
	}
	return digest(brokerDomain + upstream)
}

func digest(s string) string {
	sum := sha256.Sum256([]byte(s))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}
