package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func TestGetAgentInstanceProjectsPersistedEnabled(t *testing.T) {
	repo, db := newTestRepoWithDB(t)
	ctx := context.Background()
	agent := &models.AgentInstance{ID: "enabled-projection", WorkspaceID: "ws-1", Name: "CEO"}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{true, false} {
		if _, err := db.Exec(`UPDATE agent_profiles SET enabled = ? WHERE id = ?`, enabled, agent.ID); err != nil {
			t.Fatal(err)
		}
		got, err := repo.GetAgentInstance(ctx, agent.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.Enabled != enabled {
			t.Fatalf("persisted enabled=%v, projected enabled=%v", enabled, got.Enabled)
		}
	}
}
