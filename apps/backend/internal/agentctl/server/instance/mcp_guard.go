package instance

import (
	"crypto/sha256"
	"crypto/subtle"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

const (
	mcpRefusalLogInterval = time.Minute

	mcpReasonMissing   = "missing"
	mcpReasonMalformed = "malformed"
	mcpReasonMismatch  = "mismatch"
)

// isMCPPath reports whether path is one of the MCP routes: exactly /mcp, /sse
// or /message, or any path beneath one of them. The path is not normalized.
func isMCPPath(path string) bool {
	for _, p := range [...]string{"/mcp", "/sse", "/message"} {
		if path == p || strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// newMCPGuard returns a handler wrapper that admits an MCP route of an
// enforcing instance only when the request carries the instance's session
// token, answers 503 once revoked, and leaves a legacy instance unguarded. It is installed outside the
// activity accounting, so a refused request moves no counter.
func newMCPGuard(token *config.SessionMCPToken, log *logger.Logger) func(http.Handler) http.Handler {
	var lastLogged atomic.Int64
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !isMCPPath(r.URL.Path) {
				next.ServeHTTP(w, r)
				return
			}
			if !token.Enforcing() && !token.Revoked() {
				next.ServeHTTP(w, r)
				return
			}
			expected := token.Value()
			if expected == "" {
				writeMCPError(w, http.StatusServiceUnavailable, "mcp_token_unconfigured")
				return
			}
			if reason := mcpTokenRefusal(r, expected); reason != "" {
				logMCPRefusal(&lastLogged, log, r.URL.Path, reason)
				writeMCPError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// mcpTokenRefusal returns "" when the request presents the expected token, else
// the refusal reason.
func mcpTokenRefusal(r *http.Request, expected string) string {
	values := r.Header.Values("Authorization")
	switch {
	case len(values) == 0 || (len(values) == 1 && values[0] == ""):
		return mcpReasonMissing
	case len(values) > 1:
		return mcpReasonMalformed
	}
	const prefix = "bearer "
	v := values[0]
	if len(v) <= len(prefix) || !strings.EqualFold(v[:len(prefix)], prefix) || strings.TrimSpace(v[len(prefix):]) == "" || v[len(prefix)] == ' ' {
		return mcpReasonMalformed
	}
	got := sha256.Sum256([]byte(v[len(prefix):]))
	want := sha256.Sum256([]byte(expected))
	if subtle.ConstantTimeCompare(got[:], want[:]) != 1 {
		return mcpReasonMismatch
	}
	return ""
}

func logMCPRefusal(last *atomic.Int64, log *logger.Logger, path, reason string) {
	now := time.Now().UnixNano()
	prev := last.Load()
	if prev != 0 && now-prev < int64(mcpRefusalLogInterval) {
		return
	}
	if !last.CompareAndSwap(prev, now) {
		return
	}
	log.Warn("mcp request refused", zap.String("route", path), zap.String("reason", reason))
}

func writeMCPError(w http.ResponseWriter, status int, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + code + `"}`))
}
