package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestListAgentInstancesByIDs verifies the batched lookup returns only the
// requested rows, ignores unknown ids, and handles the empty-input case.
func TestListAgentInstancesByIDs(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	for _, a := range []*models.AgentInstance{
		{ID: "a1", WorkspaceID: "ws1", Name: "alpha", Role: models.AgentRoleWorker, Status: models.AgentStatusIdle},
		{ID: "a2", WorkspaceID: "ws1", Name: "bravo", Role: models.AgentRoleWorker, Status: models.AgentStatusIdle},
		{ID: "a3", WorkspaceID: "ws2", Name: "charlie", Role: models.AgentRoleWorker, Status: models.AgentStatusIdle},
	} {
		if err := repo.CreateAgentInstance(ctx, a); err != nil {
			t.Fatalf("create %s: %v", a.ID, err)
		}
	}

	t.Run("empty input", func(t *testing.T) {
		got, err := repo.ListAgentInstancesByIDs(ctx, nil)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("subset across workspaces", func(t *testing.T) {
		got, err := repo.ListAgentInstancesByIDs(ctx, []string{"a1", "a3", "missing"})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		seen := map[string]string{}
		for _, a := range got {
			seen[a.ID] = a.Name
		}
		if seen["a1"] != "alpha" || seen["a3"] != "charlie" {
			t.Errorf("unexpected rows: %+v", seen)
		}
	})
}
