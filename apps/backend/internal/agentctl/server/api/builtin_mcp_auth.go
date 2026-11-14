package api

import (
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const (
	errBuiltinMCPCredentialRequired = "kandev built-in MCP credential required"

	builtinMCPRejectMissing = "missing"
	builtinMCPRejectInvalid = "invalid"

	builtinMCPRejectionLogInterval = time.Minute
)

// builtinMCPCallerAuth gates the built-in MCP routes on the instance's own
// caller credential when the instance was created with enforcement. The
// control credential never satisfies it: instanceAuth exempts these routes,
// and this check compares only against the MCP credential.
func (s *Server) builtinMCPCallerAuth() gin.HandlerFunc {
	routes := make(map[string]bool)
	for _, path := range builtinMCPRoutePaths() {
		routes[path] = true
	}
	limiter := newRejectionLogLimiter(builtinMCPRejectionLogInterval, time.Now)
	return func(c *gin.Context) {
		path := c.Request.URL.Path
		if !s.cfg.BuiltinMCPAuthEnforced || !routes[path] {
			c.Next()
			return
		}
		reason := builtinMCPRejectionReason(extractBearerToken(c.Request), s.cfg.BuiltinMCPCredential)
		if reason == "" {
			c.Next()
			return
		}
		if emit, suppressed := limiter.allow(path + "\x00" + reason); emit {
			s.logger.Warn("rejected built-in MCP request without the instance caller credential",
				zap.String("route", path),
				zap.String("session_id", s.cfg.SessionID),
				zap.String("task_id", s.cfg.TaskID),
				zap.String("reason", reason),
				zap.Int("suppressed", suppressed))
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{errKey: errBuiltinMCPCredentialRequired})
	}
}

// handleGetBuiltinMCPCredential is mounted under /api/v1, so instanceAuth
// requires the control credential before it runs.
func (s *Server) handleGetBuiltinMCPCredential(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	c.JSON(http.StatusOK, types.BuiltinMCPCredentialResponse{
		Credential: s.cfg.BuiltinMCPCredential,
		Enforced:   s.cfg.BuiltinMCPAuthEnforced,
	})
}

func builtinMCPRejectionReason(presented, credential string) string {
	if presented == "" {
		return builtinMCPRejectMissing
	}
	if credential == "" || !tokenEqual(presented, credential) {
		return builtinMCPRejectInvalid
	}
	return ""
}

// rejectionLogLimiter emits at most one log per key per interval and counts
// the rejections it suppressed in between.
type rejectionLogLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	now      func() time.Time
	entries  map[string]*rejectionLogEntry
}

type rejectionLogEntry struct {
	last       time.Time
	suppressed int
}

func newRejectionLogLimiter(interval time.Duration, now func() time.Time) *rejectionLogLimiter {
	return &rejectionLogLimiter{interval: interval, now: now, entries: make(map[string]*rejectionLogEntry)}
}

// allow reports whether to log key now and how many rejections were
// suppressed since the previous log for key.
func (l *rejectionLogLimiter) allow(key string) (bool, int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	entry, ok := l.entries[key]
	if !ok {
		l.entries[key] = &rejectionLogEntry{last: now}
		return true, 0
	}
	if now.Sub(entry.last) < l.interval {
		entry.suppressed++
		return false, 0
	}
	suppressed := entry.suppressed
	entry.last = now
	entry.suppressed = 0
	return true, suppressed
}
