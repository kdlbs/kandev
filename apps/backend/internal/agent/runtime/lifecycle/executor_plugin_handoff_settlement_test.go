package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func handoffFor(running *models.ExecutorRunning) []*models.UnrecordedPluginExecutor {
	return []*models.UnrecordedPluginExecutor{{
		ExecutionID: running.AgentExecutionID, SessionID: running.SessionID, TaskID: running.TaskID,
		EnvironmentID: "environment-plugin-recovery", Running: running,
	}}
}

// A record already proven absent has nothing left to destroy, so the handoff
// that names the same execution is settled rather than left to block the
// environment forever.
func TestEnvironmentCleanupSettlesAHandoffWhoseRecordIsAlreadyAbsent(t *testing.T) {
	runtime, operations, store := claimSettlementRuntime(t, pluginExecutorPhaseAbsent, nil, nil)
	store.unrecorded = handoffFor(store.record)

	if err := destroyClaimEnvironment(runtime); err != nil {
		t.Fatalf("DestroyTaskEnvironment() error = %v", err)
	}
	if operations.destroyRequest != nil {
		t.Fatal("an absent record must not be destroyed again")
	}
	if len(store.unrecorded) != 0 {
		t.Fatalf("handoffs left = %d, want the absent record's handoff settled", len(store.unrecorded))
	}
}

// Environment cleanup may only report success when no handoff remains, whatever
// the reason the loop skipped it.
func TestEnvironmentCleanupFailsWhileAnyHandoffRemains(t *testing.T) {
	runtime, _, store := claimSettlementRuntime(t, "ready", nil, workspaceResource("", ""))
	other := pluginExecutorRecoveryRecord(t, pluginExecutorPhaseCleanupPending, workspaceResource("", ""))
	other.AgentExecutionID = "execution-skipped"
	store.unrecorded = handoffFor(other)

	if err := destroyClaimEnvironment(runtime); err == nil {
		t.Fatal("cleanup reported success while a handoff is still recorded")
	}
	if len(store.unrecorded) != 1 {
		t.Fatalf("handoffs left = %d, want the unsettled handoff retained", len(store.unrecorded))
	}
}
