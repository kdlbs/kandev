package jira

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

// parseSessionCookieExpiry extracts the `exp` claim from a Jira session-token
// JWT. The stored secret is the bare token value (the user pastes it from
// DevTools → Application → Cookies). Returns nil when the secret isn't a JWT
// or when the `exp` claim is missing — callers treat that as "unknown" rather
// than an error, since some tenant tokens are opaque.
func parseSessionCookieExpiry(secret string) *time.Time {
	parts := strings.Split(secret, ".")
	if len(parts) != 3 {
		return nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some JWTs include padding; try the padded decoder as a fallback.
		raw, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return nil
		}
	}
	var claims struct {
		Exp int64 `json:"exp"`
	}
	if err := json.Unmarshal(raw, &claims); err != nil || claims.Exp == 0 {
		return nil
	}
	t := time.Unix(claims.Exp, 0).UTC()
	return &t
}
