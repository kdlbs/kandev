package sqlite_test

import (
	"context"
	"github.com/kandev/kandev/internal/office/models"
	"testing"
)

func TestOrchestratorRegistryMultipleAndRoleLifecycle(t *testing.T) {
	repo := newSearchTestRepo(t)
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `INSERT INTO workspaces (id) VALUES ('ws')`); err != nil {
		t.Fatal(err)
	}
	role := &models.OrchestratorRole{ID: "custom", Name: "Coordinator", Instructions: "Delegate and report"}
	if err := repo.SaveOrchestratorRole(ctx, role); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"one", "two"} {
		a := &models.AgentInstance{ID: id, WorkspaceID: "ws", Name: id, Role: models.AgentRoleAssistant}
		if err := repo.CreateAgentInstance(ctx, a); err != nil {
			t.Fatal(err)
		}
		if err := repo.RegisterOrchestrator(ctx, id, "ws", role.ID); err != nil {
			t.Fatal(err)
		}
	}
	ids, err := repo.ListOrchestratorIDs(ctx, "ws")
	if err != nil || len(ids) != 2 {
		t.Fatalf("multiple orchestrators: %v %v", ids, err)
	}
	if err := repo.RegisterOrchestrator(ctx, "one", "foreign", role.ID); err == nil {
		t.Fatal("accepted foreign workspace")
	}
	if err := repo.DeleteOrchestratorRole(ctx, role.ID); err == nil {
		t.Fatal("deleted role in use")
	}
	if err := repo.UnregisterOrchestrator(ctx, "one"); err != nil {
		t.Fatal(err)
	}
	if err := repo.UnregisterOrchestrator(ctx, "two"); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteOrchestratorRole(ctx, role.ID); err != nil {
		t.Fatal(err)
	}
}
