// Package fsdiagnostics provides shared context and rate limiting for
// filesystem access diagnostics.
package fsdiagnostics

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const defaultWarningInterval = 30 * time.Second

// Context identifies one filesystem operation in a structured log entry.
// Empty identity fields are omitted because some operations run before a
// task or session exists.
type Context struct {
	Operation   string
	Target      string
	Trigger     string
	Runtime     string
	WorkspaceID string
	TaskID      string
	SessionID   string
	PollMode    string
}

// Fields returns the common fields for a filesystem operation log entry.
// Target is canonicalized here so callers that fail before their own path
// resolver completes still produce a useful, stable diagnostic.
func (c Context) Fields(err error) []zap.Field {
	fields := []zap.Field{
		zap.String("operation", c.Operation),
		zap.String("target", CanonicalPath(c.Target)),
		zap.String("trigger", c.Trigger),
		zap.String("runtime", c.Runtime),
	}
	if c.WorkspaceID != "" {
		fields = append(fields, zap.String("workspace_id", c.WorkspaceID))
	}
	if c.TaskID != "" {
		fields = append(fields, zap.String("task_id", c.TaskID))
	}
	if c.SessionID != "" {
		fields = append(fields, zap.String("session_id", c.SessionID))
	}
	if c.PollMode != "" {
		fields = append(fields, zap.String("poll_mode", c.PollMode))
	}
	if err != nil {
		fields = append(fields, zap.Error(err))
	}
	return fields
}

// CanonicalPath returns an absolute, cleaned path and resolves symlinks when
// the target currently exists. It deliberately preserves a blank target.
func CanonicalPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(abs)
}

// RuntimeMode converts the desktop-process marker into the stable runtime
// labels used by diagnostics.
func RuntimeMode(desktop bool) string {
	if desktop {
		return "desktop"
	}
	return "server"
}

// IsAccessDenied reports the deterministic permission class used by the
// tracker pause policy. The text fallback covers platform-specific errors
// that do not wrap os.ErrPermission in tests or in an OS adapter.
func IsAccessDenied(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrPermission) {
		return true
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "permission denied") ||
		strings.Contains(message, "access is denied") ||
		strings.Contains(message, "operation not permitted")
}

type warningState struct {
	lastWarned time.Time
	suppressed int
}

// WarningLimiter bounds repeated warnings for an identical operation. The
// first warning is emitted immediately; the next warning after the interval
// carries the number suppressed since the previous warning.
type WarningLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	now      func() time.Time
	entries  map[string]warningState
}

// NewWarningLimiter creates a limiter with the supplied warning interval.
// Non-positive intervals use the production default.
func NewWarningLimiter(interval time.Duration) *WarningLimiter {
	if interval <= 0 {
		interval = defaultWarningInterval
	}
	return &WarningLimiter{
		interval: interval,
		now:      time.Now,
		entries:  make(map[string]warningState),
	}
}

// Warn emits a bounded warning with the operation context. The error text is
// intentionally excluded from the key so changing OS details cannot create an
// unbounded number of entries for one operation and target.
func (l *WarningLimiter) Warn(log *zap.Logger, message string, operation Context, err error) {
	if l == nil || log == nil {
		return
	}

	now := l.now()
	key := warningKey(operation)
	l.mu.Lock()
	state, exists := l.entries[key]
	if exists && now.Sub(state.lastWarned) < l.interval {
		state.suppressed++
		l.entries[key] = state
		l.mu.Unlock()
		return
	}
	suppressed := state.suppressed
	l.entries[key] = warningState{lastWarned: now}
	l.mu.Unlock()

	fields := operation.Fields(err)
	if suppressed > 0 {
		fields = append(fields, zap.Int("suppressed_count", suppressed))
	}
	log.Warn(message, fields...)
}

func warningKey(operation Context) string {
	return strings.Join([]string{
		operation.Operation,
		CanonicalPath(operation.Target),
		operation.Trigger,
		operation.Runtime,
		operation.WorkspaceID,
		operation.TaskID,
		operation.SessionID,
	}, "\x00")
}
