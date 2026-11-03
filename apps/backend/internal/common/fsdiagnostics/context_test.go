package fsdiagnostics

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestWarningLimiterBoundsRepeatedAccessDenials(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	clock := time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC)
	limiter := NewWarningLimiter(time.Minute)
	limiter.now = func() time.Time { return clock }
	operation := Context{
		Operation:   "repository.discovery.scan",
		Target:      filepath.Join(t.TempDir(), "repo"),
		Trigger:     "poll",
		Runtime:     "desktop",
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		SessionID:   "session-1",
		PollMode:    "slow",
	}
	denied := errors.New("permission denied")

	for range 3 {
		limiter.Warn(zap.New(core), "filesystem.access_denied", operation, denied)
	}
	if got := logs.Len(); got != 1 {
		t.Fatalf("warning count before interval = %d, want 1", got)
	}

	clock = clock.Add(time.Minute)
	limiter.Warn(zap.New(core), "filesystem.access_denied", operation, denied)
	if got := logs.Len(); got != 2 {
		t.Fatalf("warning count after interval = %d, want 2", got)
	}
	if got := logs.All()[1].ContextMap()["suppressed_count"]; got != int64(2) {
		t.Fatalf("suppressed_count = %v, want 2", got)
	}
}

func TestWarningLimiterDoesNotGrowPerErrorMessage(t *testing.T) {
	core, logs := observer.New(zapcore.WarnLevel)
	limiter := NewWarningLimiter(time.Minute)
	operation := Context{
		Operation: "workspace.file_monitor",
		Target:    filepath.Join(t.TempDir(), "repo"),
		Trigger:   "poll",
		Runtime:   "desktop",
	}

	limiter.Warn(zap.New(core), "filesystem.access_denied", operation, errors.New("permission denied: one"))
	limiter.Warn(zap.New(core), "filesystem.access_denied", operation, errors.New("permission denied: two"))

	if got := logs.Len(); got != 1 {
		t.Fatalf("warning count for changing errors = %d, want 1", got)
	}
}

func TestContextFieldsCanonicalizeTargetAndIncludeIdentity(t *testing.T) {
	root := t.TempDir()
	operation := Context{
		Operation:   "workspace.file_monitor",
		Target:      filepath.Join(root, ".", "repo"),
		Trigger:     "stale_refresh",
		Runtime:     "server",
		WorkspaceID: "workspace-1",
		TaskID:      "task-1",
		SessionID:   "session-1",
		PollMode:    "fast",
	}
	fields := operation.Fields(errors.New("permission denied"))
	core, logs := observer.New(zapcore.WarnLevel)
	zap.New(core).Warn("filesystem.access_denied", fields...)
	entry := logs.All()[0].ContextMap()

	if got, want := entry["operation"], "stat"; got != want {
		t.Fatalf("operation = %v, want %v", got, want)
	}
	if got, want := entry["error_class"], "permission_denied"; got != want {
		t.Fatalf("error_class = %v, want %v", got, want)
	}
	if got, want := entry["target"], CanonicalPath(operation.Target); got != want {
		t.Fatalf("target = %v, want %v", got, want)
	}
	for key, want := range map[string]string{
		"trigger":      "stale_refresh",
		"runtime":      "server",
		"workspace_id": "workspace-1",
		"task_id":      "task-1",
		"session_id":   "session-1",
		"poll_mode":    "fast",
	} {
		if got := entry[key]; got != want {
			t.Errorf("%s = %v, want %v", key, got, want)
		}
	}
}

func TestIsAccessDenied(t *testing.T) {
	if !IsAccessDenied(errors.New("permission denied")) {
		t.Error("permission denied was not classified as access denied")
	}
	if IsAccessDenied(errors.New("temporary index lock")) {
		t.Error("index lock was classified as access denied")
	}
}

func TestErrorClassUsesClosedVocabulary(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{name: "canceled", err: context.Canceled, want: "context_canceled"},
		{name: "deadline", err: context.DeadlineExceeded, want: "deadline_exceeded"},
		{name: "permission", err: os.ErrPermission, want: "permission_denied"},
		{name: "not found", err: os.ErrNotExist, want: "not_found"},
		{name: "git start", err: &exec.Error{Name: "git", Err: exec.ErrNotFound}, want: "git_start_failed"},
		{name: "git command", err: &exec.ExitError{}, want: "git_command_failed"},
		{name: "git repository context", err: &exec.ExitError{Stderr: []byte("fatal: not a git repository")}, want: "repository_context_unavailable"},
		{name: "stat through regular file", err: &os.PathError{Op: "stat", Path: "file/child", Err: syscall.ENOTDIR}, want: "io_error"},
		{name: "repository context", err: errors.New("fatal: not a git repository"), want: "repository_context_unavailable"},
		{name: "io", err: &os.PathError{Op: "read", Path: "file", Err: errors.New("broken")}, want: "io_error"},
		{name: "unknown", err: errors.New("unexpected"), want: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ErrorClass(tt.err); got != tt.want {
				t.Fatalf("ErrorClass(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}
