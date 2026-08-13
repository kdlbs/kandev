package orchestrator

import (
	"testing"

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

// The failure reason compared in the resolution path must be the exported
// constant, not a local copy that can drift out of sync silently.
func TestFailedReasonUsesTheExportedConstant(t *testing.T) {
	if taskservice.BlockedReasonFailed != "failed" {
		t.Fatalf("BlockedReasonFailed = %q; the resolution path compares against it",
			taskservice.BlockedReasonFailed)
	}
}
