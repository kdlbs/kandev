package lifecycle

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/runtime/activity"
)

func TestAcquireOpenCodeMigrationIgnoresUnrelatedHostActivity(t *testing.T) {
	manager := newTestManager(t)
	coordinator := activity.NewCoordinator(activity.Options{})
	manager.SetActivityCoordinator(coordinator)
	setupLease, err := coordinator.AcquireTask(context.Background(), activity.KindSetupScript)
	if err != nil {
		t.Fatalf("AcquireTask: %v", err)
	}
	defer setupLease.Release()

	_, release, err := manager.AcquireOpenCodeMigration(context.Background())
	if err != nil {
		t.Fatalf("OpenCode migration should proceed while unrelated setup work is active: %v", err)
	}
	if manager.openCodeAdmission.TryRLock() {
		manager.openCodeAdmission.RUnlock()
		t.Fatal("migration lease did not block a new OpenCode launch")
	}
	release()
	if !manager.openCodeAdmission.TryRLock() {
		t.Fatal("OpenCode launch gate remained blocked after migration release")
	}
	manager.openCodeAdmission.RUnlock()

	if err := manager.executionStore.Add(&AgentExecution{ID: "open-code-execution", AgentID: agents.OpenCodeACPAgentID}); err != nil {
		t.Fatalf("add active OpenCode execution: %v", err)
	}
	if _, _, err := manager.AcquireOpenCodeMigration(context.Background()); !errors.Is(err, ErrOpenCodeExecutionActive) {
		t.Fatalf("active OpenCode execution admission error = %v, want %v", err, ErrOpenCodeExecutionActive)
	}
}
