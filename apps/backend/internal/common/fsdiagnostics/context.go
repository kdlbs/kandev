// Package fsdiagnostics provides shared context and rate limiting for
// filesystem access diagnostics.
package fsdiagnostics

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

const (
	defaultWarningInterval = 30 * time.Second
	runtimeDesktop         = "desktop"
	runtimeServer          = "server"
)

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
		zap.String("operation", NormalizeOperation(c.Operation)),
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
		fields = append(fields, zap.String("error_class", ErrorClass(err)), zap.Error(err))
	}
	return fields
}

// NormalizeOperation maps internal operation names onto the closed vocabulary
// the parent launcher forwards. Unknown names remain unchanged for local
// diagnostics.
func NormalizeOperation(operation string) string {
	switch operation {
	case "workspace.git_status", "workspace.git_poll", "git_inspection":
		return "git_inspection"
	case "workspace.file_monitor", "stat":
		return "stat"
	case "workspace.file_tree", "read_directory":
		return "read_directory"
	default:
		return operation
	}
}

// ErrorClass classifies an operation failure without exposing its text. The
// closed vocabulary matches the parent launcher's forwarding allowlist.
func ErrorClass(err error) string {
	if err == nil {
		return ""
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	}
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		return "git_start_failed"
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr.Op == "fork/exec" {
		return "git_start_failed"
	}
	if errors.Is(err, os.ErrPermission) || IsAccessDenied(err) {
		return "permission_denied"
	}
	if errors.Is(err, os.ErrNotExist) {
		return "not_found"
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if isRepositoryContextUnavailableText(strings.ToLower(string(exitErr.Stderr))) {
			return "repository_context_unavailable"
		}
		return "git_command_failed"
	}
	if errors.As(err, &pathErr) {
		return "io_error"
	}
	if isRepositoryContextUnavailable(err) {
		return "repository_context_unavailable"
	}
	return "unknown"
}

func isRepositoryContextUnavailable(err error) bool {
	return isRepositoryContextUnavailableText(strings.ToLower(err.Error()))
}

func isRepositoryContextUnavailableText(message string) bool {
	return strings.Contains(message, "not a git repository") ||
		strings.Contains(message, "cannot change to") ||
		strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "not a directory")
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
		return runtimeDesktop
	}
	return runtimeServer
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
