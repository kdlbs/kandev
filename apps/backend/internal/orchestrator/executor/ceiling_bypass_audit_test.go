package executor

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/zap/zapcore"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// fakeCeilingBackingChecker is a call-recording, scriptable double for
// CeilingBackingChecker.
type fakeCeilingBackingChecker struct {
	calls  []string
	backed bool
	err    error
}

func (f *fakeCeilingBackingChecker) IsSessionCeilingBacked(_ context.Context, sessionID string) (bool, error) {
	f.calls = append(f.calls, sessionID)
	return f.backed, f.err
}

// TestAuditCeilingBypass_NilCheckerIsANoOp pins AC-41a: with no checker
// injected, every construction site behaves exactly as before this card —
// no detection, no log, no error.
func TestAuditCeilingBypass_NilCheckerIsANoOp(t *testing.T) {
	exec, logs := newObservedExecutor(t, zapcore.DebugLevel)
	exec.auditCeilingBypass(context.Background(), "someEntryPoint", "session-x", true)
	if logs.Len() != 0 {
		t.Fatalf("expected no log entries with no checker injected, got %d: %v", logs.Len(), logs.All())
	}
}

// TestAuditCeilingBypass_EmptySessionIDIsANoOp pins that an entry point with
// no session id yet never reaches the checker at all.
func TestAuditCeilingBypass_EmptySessionIDIsANoOp(t *testing.T) {
	exec, logs := newObservedExecutor(t, zapcore.DebugLevel)
	checker := &fakeCeilingBackingChecker{backed: false}
	exec.SetCeilingBackingChecker(checker)

	exec.auditCeilingBypass(context.Background(), "someEntryPoint", "", true)

	if len(checker.calls) != 0 {
		t.Fatalf("checker must not be consulted for an empty session id, got calls=%v", checker.calls)
	}
	if logs.Len() != 0 {
		t.Fatalf("expected no log entries, got %d: %v", logs.Len(), logs.All())
	}
}

// TestAuditCeilingBypass_BackedSessionLogsNothing covers the ordinary case:
// a launch reaching an instrumented entry point for a session the ceiling
// already backs is silent, whether or not the entry point is the ERROR site.
func TestAuditCeilingBypass_BackedSessionLogsNothing(t *testing.T) {
	for _, emitError := range []bool{true, false} {
		exec, logs := newObservedExecutor(t, zapcore.DebugLevel)
		exec.SetCeilingBackingChecker(&fakeCeilingBackingChecker{backed: true})

		exec.auditCeilingBypass(context.Background(), "someEntryPoint", "session-x", emitError)

		if logs.Len() != 0 {
			t.Fatalf("emitError=%v: expected no log entries for a backed session, got %d: %v", emitError, logs.Len(), logs.All())
		}
	}
}

// TestAuditCeilingBypass_UnbackedEmitsErrorAtTheDesignatedSite pins AC-4b1:
// the sole ERROR site (runAgentProcessAsync, emitError=true) logs at ERROR,
// carrying the session id, the entry point name, and any extra fields (the
// agent_execution_id runAgentProcessAsync passes).
func TestAuditCeilingBypass_UnbackedEmitsErrorAtTheDesignatedSite(t *testing.T) {
	exec, logs := newObservedExecutor(t, zapcore.DebugLevel)
	exec.SetCeilingBackingChecker(&fakeCeilingBackingChecker{backed: false})

	exec.auditCeilingBypass(context.Background(), "runAgentProcessAsync", "session-x", true)

	errs := logs.FilterLevelExact(zapcore.ErrorLevel).All()
	if len(errs) != 1 {
		t.Fatalf("error entries = %d, want 1; all=%v", len(errs), logs.All())
	}
	fields := errs[0].ContextMap()
	if fields["session_id"] != "session-x" || fields["entry_point"] != "runAgentProcessAsync" {
		t.Fatalf("unexpected error fields: %+v", fields)
	}
}

// TestAuditCeilingBypass_UnbackedLogsDebugAtTheOtherTwoSites pins AC-4b1: the
// other two entry points (LaunchPreparedSession, ResumeSessionWithOptions,
// emitError=false) log the same observation at DEBUG rather than ERROR, so
// the bypass is diagnosable without duplicating the reportable ERROR.
func TestAuditCeilingBypass_UnbackedLogsDebugAtTheOtherTwoSites(t *testing.T) {
	exec, logs := newObservedExecutor(t, zapcore.DebugLevel)
	exec.SetCeilingBackingChecker(&fakeCeilingBackingChecker{backed: false})

	exec.auditCeilingBypass(context.Background(), "LaunchPreparedSession", "session-x", false)

	if errs := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); errs != 0 {
		t.Fatalf("error entries = %d, want 0", errs)
	}
	debugs := logs.FilterLevelExact(zapcore.DebugLevel).All()
	if len(debugs) != 1 {
		t.Fatalf("debug entries = %d, want 1; all=%v", len(debugs), logs.All())
	}
}

// TestAuditCeilingBypass_LookupFailureNeverEmitsErrorEvenAtTheErrorSite pins
// AC-41b: a failed lookup (the AC-1 row half is a repository read that can
// fail) logs the failure at DEBUG and emits no AC-4b ERROR, even when called
// from the designated ERROR site — a missed detection is bounded by AC-8's
// re-derivation; a false one is not bounded by anything.
func TestAuditCeilingBypass_LookupFailureNeverEmitsErrorEvenAtTheErrorSite(t *testing.T) {
	exec, logs := newObservedExecutor(t, zapcore.DebugLevel)
	exec.SetCeilingBackingChecker(&fakeCeilingBackingChecker{err: errors.New("lookup failed")})

	exec.auditCeilingBypass(context.Background(), "runAgentProcessAsync", "session-x", true)

	if errs := logs.FilterLevelExact(zapcore.ErrorLevel).Len(); errs != 0 {
		t.Fatalf("error entries = %d, want 0 on a lookup failure", errs)
	}
	debugs := logs.FilterLevelExact(zapcore.DebugLevel).All()
	if len(debugs) != 1 {
		t.Fatalf("debug entries = %d, want 1; all=%v", len(debugs), logs.All())
	}
	if debugs[0].ContextMap()["session_id"] != "session-x" {
		t.Fatalf("debug entry must carry the session id: %+v", debugs[0].ContextMap())
	}
}

// TestLaunchPreparedSession_AuditsOnlyWhenStartingAnAgent pins AC-4c: a
// LaunchPreparedSession call with StartAgent false prepares a workspace and
// starts no process, so it must not be instrumented at all; StartAgent true
// must consult the checker.
func TestLaunchPreparedSession_AuditsOnlyWhenStartingAnAgent(t *testing.T) {
	for _, tt := range []struct {
		name       string
		startAgent bool
		wantCalls  int
	}{
		{"StartAgent true audits", true, 1},
		{"StartAgent false is not instrumented", false, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repo := newMockRepository()
			repo.sessions["session-x"] = &models.TaskSession{ID: "session-x", TaskID: "other-task"}
			exec := newTestExecutor(t, &mockAgentManager{}, repo)
			checker := &fakeCeilingBackingChecker{backed: true}
			exec.SetCeilingBackingChecker(checker)

			// The session belongs to a different task, so LaunchPreparedSession
			// returns right after the audit call fires, before any process
			// dispatch machinery runs.
			_, _ = exec.LaunchPreparedSession(context.Background(), &v1.Task{ID: "task-x"}, "session-x", LaunchOptions{StartAgent: tt.startAgent})

			if len(checker.calls) != tt.wantCalls {
				t.Fatalf("checker calls = %d, want %d (calls=%v)", len(checker.calls), tt.wantCalls, checker.calls)
			}
		})
	}
}
