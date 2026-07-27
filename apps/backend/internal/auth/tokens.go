package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

// Opaque credential format. Both browser-session tokens and personal access
// tokens (PATs) are 256-bit random values; only their SHA-256 hex digest is
// persisted, so a database leak does not leak usable credentials.
//
// PATs are prefixed for recognizability in logs/UIs:
//
//	kandev_pat_<8 hex prefix>_<43 char base64url secret>
//
// The prefix is stored in plaintext alongside the hash so the UI can display
// "kandev_pat_ab12cd34_…" for identification after the secret is gone.
const (
	// PATPrefix marks personal access tokens.
	PATPrefix = "kandev_pat_"

	tokenSecretBytes = 32
	patIDBytes       = 4 // 8 hex chars
)

// GenerateSessionToken returns a new opaque session token and its storage hash.
func GenerateSessionToken() (token, hash string, err error) {
	secret, err := randomToken(tokenSecretBytes)
	if err != nil {
		return "", "", err
	}
	return secret, HashToken(secret), nil
}

// GeneratePAT returns a new personal access token, its display prefix, and its
// storage hash. The full token is shown to the user exactly once.
func GeneratePAT() (token, prefix, hash string, err error) {
	idBytes := make([]byte, patIDBytes)
	if _, err := rand.Read(idBytes); err != nil {
		return "", "", "", fmt.Errorf("generate token prefix: %w", err)
	}
	secret, err := randomToken(tokenSecretBytes)
	if err != nil {
		return "", "", "", err
	}
	prefix = hex.EncodeToString(idBytes)
	token = PATPrefix + prefix + "_" + secret
	return token, prefix, HashToken(token), nil
}

// HashToken returns the hex-encoded SHA-256 digest used for storage/lookup.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// IsPATFormat reports whether a bearer credential looks like a kandev PAT.
// Used by the middleware to distinguish PATs from other bearer schemes
// (e.g. office agent JWTs) without a database round-trip.
func IsPATFormat(token string) bool {
	return strings.HasPrefix(token, PATPrefix)
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
