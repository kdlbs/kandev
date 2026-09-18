package service_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// TestMarkAgentPausedFixed_LeavesInboxUndismissedWhenClearFails pins the
// AC-OFFICE-RUNTIME-001.10 ordering: MarkAgentPausedFixed must not dismiss
// the inbox entry until clearAutoPause actually succeeds. A trigger blocks
// the exact UPDATE UnpauseAgentIfCurrent issues, forcing clearAutoPause to
// return an error deterministically (no goroutine or real race needed) so
// MarkAgentPausedFixed returns before reaching dismiss, counter reset, or
// task recovery.
func TestMarkAgentPausedFixed_LeavesInboxUndismissedWhenClearFails(t *testing.T) {
	svc, _ := newTestServiceWithBus(t)
	ctx := context.Background()

	createTestAgent(t, svc, "ws-1", "agent-clear-fails")
	autoPauseAgent(t, svc, "ws-1", "agent-clear-fails", 3)

	svc.ExecSQL(t, `
		CREATE TRIGGER block_unpause
		BEFORE UPDATE ON agent_profiles
		WHEN NEW.status = 'idle' AND OLD.status = 'paused'
		BEGIN
			SELECT RAISE(ABORT, 'injected: unpause write blocked');
		END
	`)

	if err := svc.MarkAgentPausedFixed(ctx, "user-1", "agent-clear-fails"); err == nil {
		t.Fatal("MarkAgentPausedFixed = nil, want an error from the blocked unpause write")
	}

	dismissed, err := svc.IsInboxItemDismissed(
		ctx, "user-1", service.InboxKindAgentPausedAfterFails, "agent-clear-fails",
	)
	if err != nil {
		t.Fatalf("check dismissed: %v", err)
	}
	if dismissed {
		t.Fatal("inbox entry was dismissed despite the clear failing; " +
			"dismiss must happen only after a successful clearAutoPause")
	}

	agent, err := svc.GetAgentInstance(ctx, "agent-clear-fails")
	if err != nil {
		t.Fatalf("get agent: %v", err)
	}
	if agent.Status != models.AgentStatusPaused {
		t.Fatalf("agent status = %q, want still paused (unpause write was blocked)", agent.Status)
	}
	if agent.ConsecutiveFailures == 0 {
		t.Fatal("consecutive-failure counter was reset despite the clear failing")
	}
}
