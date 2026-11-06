package process

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestWorkspaceTrackerAccessDeniedPausesUntilVisibleRetry(t *testing.T) {
	wt := newGraceTestTracker(t, graceNeverFires)
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	wt.logger = log
	wt.SetDiagnosticIdentity("task-1", "session-1")

	denied := errors.New("permission denied")
	for range 3 {
		wt.recordFilesystemFailure("workspace.git_poll", "poll", denied)
	}
	if got := wt.GetPollMode(); got != PollModePaused {
		t.Fatalf("poll mode after denial = %v, want paused", got)
	}
	if !wt.accessDenied.Load() {
		t.Fatal("access denial was not retained")
	}
	if got := logs.Len(); got != 2 {
		t.Fatalf("warning count after repeated denial = %d, want initial denial and pause", got)
	}

	wt.SetPollMode(PollModeSlow)
	if got := wt.GetPollMode(); got != PollModePaused {
		t.Fatalf("poll mode after slow background update = %v, want paused", got)
	}
	wt.SetPollMode(PollModeFast)
	if got := wt.GetPollMode(); got != PollModeFast {
		t.Fatalf("poll mode after focused retry = %v, want fast", got)
	}
	if wt.accessDenied.Load() {
		t.Fatal("focused retry did not clear access denial")
	}
}

func TestWorkspaceTrackerCapturesRuntimeModeAtConstruction(t *testing.T) {
	t.Setenv("KANDEV_DESKTOP_RUNTIME", "true")
	tracker := NewWorkspaceTracker(t.TempDir(), newTestLogger(t))
	t.Cleanup(tracker.Stop)

	t.Setenv("KANDEV_DESKTOP_RUNTIME", "false")
	if got := tracker.diagnosticContext("workspace.file_monitor", "poll").Runtime; got != "desktop" {
		t.Fatalf("diagnostic runtime = %q, want desktop", got)
	}
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-003.3
func TestWorkspaceDiagnosticSafeClassification(t *testing.T) {
	wt := newGraceTestTracker(t, graceNeverFires)
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	wt.logger = log
	wt.SetDiagnosticIdentity("task-1", "session-1")

	wt.recordFilesystemFailure("workspace.git_poll", "poll", context.DeadlineExceeded)
	wt.recordFilesystemFailure("workspace.file_tree", "user_select", errors.New("permission denied"))

	entries := logs.All()
	if len(entries) < 2 {
		t.Fatalf("workspace diagnostic entries = %d, want at least 2", len(entries))
	}
	deadline := entries[0].ContextMap()
	if deadline["operation"] != "git_inspection" || deadline["error_class"] != "deadline_exceeded" {
		t.Fatalf("git inspection diagnostic = %+v, want git_inspection/deadline_exceeded", deadline)
	}
	denied := entries[1].ContextMap()
	if denied["operation"] != "read_directory" || denied["error_class"] != "permission_denied" {
		t.Fatalf("filesystem denial diagnostic = %+v, want read_directory/permission_denied", denied)
	}
}
