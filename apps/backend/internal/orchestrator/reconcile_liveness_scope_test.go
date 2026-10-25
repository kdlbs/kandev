package orchestrator

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
)

// TestReconcileExecutorSessionsOnStartupReusesOneLivenessScopePerPass pins
// design 02 "Persistence": startup reconciliation is a pass, so it must take
// exactly one adopted-server enumeration and reuse it across every row,
// never enumerate once per row.
func TestReconcileExecutorSessionsOnStartupReusesOneLivenessScopePerPass(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()

	for _, id := range []string{"1", "2"} {
		taskID := "task-scope-" + id
		sessionID := "session-scope-" + id
		seedTaskAndSession(t, repo, taskID, sessionID, models.TaskSessionStateIdle)
		if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
			ID:        sessionID,
			SessionID: sessionID,
			TaskID:    taskID,
			Runtime:   agentruntime.RuntimeStandalone,
			Status:    models.ExecutorRunningStatusRunning,
			LocalPID:  4242,
			CreatedAt: time.Now().UTC(),
			UpdatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("upsert executor row %s: %v", sessionID, err)
		}
	}

	agentMgr := &mockAgentManager{}
	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), agentMgr)

	svc.reconcileExecutorSessionsOnStartup(ctx)

	if agentMgr.newStandaloneLivenessScopeCalls != 1 {
		t.Fatalf("NewStandaloneLivenessScope calls = %d, want exactly 1 for a two-row reconciliation pass",
			agentMgr.newStandaloneLivenessScopeCalls)
	}
}

// TestReconcileExecutorSessionsOnStartupSkipsScopeWithoutStandaloneCapability
// pins the fallback: when s.agentManager doesn't implement
// standaloneLivenessScoper, reconciliation still runs (via the ordinary
// unscoped rowLiveness) without panicking on a nil scope.
func TestReconcileExecutorSessionsOnStartupSkipsScopeWithoutStandaloneCapability(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	seedTaskAndSession(t, repo, "task-noscope", "session-noscope", models.TaskSessionStateIdle)
	if err := repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{
		ID:        "session-noscope",
		SessionID: "session-noscope",
		TaskID:    "task-noscope",
		Runtime:   agentruntime.RuntimeStandalone,
		Status:    models.ExecutorRunningStatusRunning,
		LocalPID:  4242,
		CreatedAt: time.Now().UTC(),
		UpdatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("upsert executor row: %v", err)
	}

	svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), nonScopingAgentManager{&mockAgentManager{}})

	svc.reconcileExecutorSessionsOnStartup(ctx)
}

// nonScopingAgentManager embeds executor.AgentManagerClient by interface
// value, not by *mockAgentManager, so only the methods that interface
// declares are promoted -- NewStandaloneLivenessScope/RowLivenessScoped are
// not part of it, so nonScopingAgentManager does not satisfy
// standaloneLivenessScoper even though the wrapped *mockAgentManager has
// those methods.
type nonScopingAgentManager struct {
	executor.AgentManagerClient
}
