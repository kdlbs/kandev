package orchestrator

import (
	"context"
	"testing"

	"go.uber.org/zap/zapcore"

	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Dependency resolution must launch only tasks that recorded a
// start-when-unblocked intent. Without this, a blocked task sitting in a step
// with on_enter:auto_start_agent would launch the moment its gate opened, even
// though nobody asked for a start: the chokepoint's step-based auto-start is
// meant for a task ENTERING that step, not one already parked in it.
func TestResolutionLaunchesOnlyTasksWithARecordedIntent(t *testing.T) {
	chainStep := &models.Task{Metadata: map[string]interface{}{
		models.MetaKeyDeferredLaunch: map[string]interface{}{
			"intent": "start", "agent_profile_id": "p",
			models.DeferredLaunchStartWhenUnblockedKey: true,
		},
	}}
	if !taskservice.HasStartWhenUnblockedIntent(chainStep) {
		t.Error("a chain step must be recognised as launchable on resolution")
	}

	// A WIP-overflow intent is the same record without the flag; resolution must
	// not adopt it, or a queued task would launch for the wrong reason.
	wipOverflow := &models.Task{Metadata: map[string]interface{}{
		models.MetaKeyDeferredLaunch: map[string]interface{}{
			"intent": "start", "agent_profile_id": "p",
		},
	}}
	if taskservice.HasStartWhenUnblockedIntent(wipOverflow) {
		t.Error("a WIP-overflow intent must not be treated as a chain intent")
	}

	for name, task := range map[string]*models.Task{
		"no metadata": {},
		"nil task":    nil,
	} {
		if taskservice.HasStartWhenUnblockedIntent(task) {
			t.Errorf("%s: must not be launchable on resolution", name)
		}
	}
}

// TestDependencyBlocksAutoStartWarnsOnBlockedSkip is the regression test for
// issue #3720: the blocked verdict was logged at Debug, invisible at the
// default INFO level, so operators saw tasks sit on an auto-start step with
// no launch and no log. A deliberate skip of an auto-start is
// operator-visible by definition and must warn, matching the lookup-error
// path right above it.
//
// Expected pre-fix failure: the only matching entry is at Debug, so the
// WarnLevel filter finds none.
func TestDependencyBlocksAutoStartWarnsOnBlockedSkip(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	svc := createTestService(repo, newMockStepGetter(), newMockTaskRepo())
	log, logs := observedTestLogger(t)
	svc.logger = log
	svc.SetTaskDependencyReader(&launchDependencyReader{blocked: true})

	const taskID = "task-blocked-skip"
	const eventName = "task.workflow_step_entered"
	blocked, gateErrored := svc.dependencyBlocksAutoStart(ctx, taskID, eventName)
	if !blocked || gateErrored {
		t.Fatalf("blocked = %v, gateErrored = %v; want true, false", blocked, gateErrored)
	}

	entries := logs.
		FilterMessage(eventName + ": task has unresolved dependencies; skipping auto-start").
		FilterLevelExact(zapcore.WarnLevel).All()
	if len(entries) != 1 {
		t.Fatalf("blocked-skip WARN logs = %#v, want exactly one", entries)
	}
	fields := entries[0].ContextMap()
	if got := fields["task_id"]; got != taskID {
		t.Fatalf("task_id = %v, want %q", got, taskID)
	}
	if got := fields["blocked_reason"]; got != "pending" {
		t.Fatalf("blocked_reason = %v, want \"pending\"", got)
	}
}

// The failure reason compared in the resolution path must be the exported
// constant, not a local copy that can drift out of sync silently.
func TestFailedReasonUsesTheExportedConstant(t *testing.T) {
	if taskservice.BlockedReasonFailed != "failed" {
		t.Fatalf("BlockedReasonFailed = %q; the resolution path compares against it",
			taskservice.BlockedReasonFailed)
	}
}
