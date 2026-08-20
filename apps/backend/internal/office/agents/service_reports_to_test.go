package agents

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

// TestUpdateAgentInstance_ReportsToTwoAgentCycleRejected reproduces the
// scenario the card exists to close: bob already reports_to carol (set via
// a prior, valid update), then a PATCH sets carol reports_to bob. That
// closes a two-agent cycle and must be rejected with ErrAgentReportsToCycle,
// leaving carol's persisted reports_to unchanged.
func TestUpdateAgentInstance_ReportsToTwoAgentCycleRejected(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()

	bob := createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "bob", Role: models.AgentRoleWorker,
	})
	carol := createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "carol", Role: models.AgentRoleWorker,
	})

	bob.ReportsTo = carol.ID
	if err := svc.UpdateAgentInstance(ctx, bob); err != nil {
		t.Fatalf("bob reports_to carol: %v", err)
	}

	carol.ReportsTo = bob.ID
	err := svc.UpdateAgentInstance(ctx, carol)
	if !errors.Is(err, ErrAgentReportsToCycle) {
		t.Fatalf("carol reports_to bob: got err %v, want ErrAgentReportsToCycle", err)
	}

	stored, getErr := repo.GetAgentInstance(ctx, carol.ID)
	if getErr != nil {
		t.Fatalf("GetAgentInstance: %v", getErr)
	}
	if stored.ReportsTo != "" {
		t.Fatalf("carol reports_to persisted as %q despite rejected update", stored.ReportsTo)
	}
}

// TestUpdateAgentInstance_ReportsToChainCycleRejected covers a longer chain:
// alice -> bob -> carol, then closing the loop with carol reports_to alice.
func TestUpdateAgentInstance_ReportsToChainCycleRejected(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()

	alice := createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "alice", Role: models.AgentRoleWorker,
	})
	bob := createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "bob", Role: models.AgentRoleWorker,
	})
	carol := createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "carol", Role: models.AgentRoleWorker,
	})

	bob.ReportsTo = alice.ID
	if err := svc.UpdateAgentInstance(ctx, bob); err != nil {
		t.Fatalf("bob reports_to alice: %v", err)
	}
	carol.ReportsTo = bob.ID
	if err := svc.UpdateAgentInstance(ctx, carol); err != nil {
		t.Fatalf("carol reports_to bob: %v", err)
	}

	alice.ReportsTo = carol.ID
	err := svc.UpdateAgentInstance(ctx, alice)
	if !errors.Is(err, ErrAgentReportsToCycle) {
		t.Fatalf("alice reports_to carol: got err %v, want ErrAgentReportsToCycle", err)
	}
}

// TestUpdateAgentInstance_ReportsToDeepChainNoCycleResolves is the
// false-positive guard: a valid, non-cyclic chain of updates must all
// succeed.
func TestUpdateAgentInstance_ReportsToDeepChainNoCycleResolves(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()

	names := []string{"a", "b", "c", "d"}
	created := make([]*models.AgentInstance, len(names))
	for i, name := range names {
		created[i] = createAndGetAgent(t, svc, repo, &models.AgentInstance{
			WorkspaceID: "ws-1", Name: name, Role: models.AgentRoleWorker,
		})
	}

	// b -> a, c -> b, d -> c: a valid deep chain, no cycle anywhere.
	for i := 1; i < len(created); i++ {
		created[i].ReportsTo = created[i-1].ID
		if err := svc.UpdateAgentInstance(ctx, created[i]); err != nil {
			t.Fatalf("%s reports_to %s: %v", names[i], names[i-1], err)
		}
	}

	stored, err := repo.GetAgentInstance(ctx, created[len(created)-1].ID)
	if err != nil {
		t.Fatalf("GetAgentInstance: %v", err)
	}
	if stored.ReportsTo != created[len(created)-2].ID {
		t.Fatalf("d reports_to = %q, want %q", stored.ReportsTo, created[len(created)-2].ID)
	}
}

func TestCreateAgentInstance_ReportsToNameStaysInWorkspace(t *testing.T) {
	svc, repo := newTestAgentService(t)
	ctx := context.Background()

	createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-other", Name: "Manager", Role: models.AgentRoleWorker,
	})
	manager := createAndGetAgent(t, svc, repo, &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "Manager", Role: models.AgentRoleWorker,
	})
	worker := &models.AgentInstance{
		WorkspaceID: "ws-1", Name: "Worker", Role: models.AgentRoleWorker,
		ReportsTo: "Manager",
	}

	if err := svc.CreateAgentInstance(ctx, worker); err != nil {
		t.Fatalf("create worker: %v", err)
	}
	stored, err := repo.GetAgentInstance(ctx, worker.ID)
	if err != nil {
		t.Fatalf("get worker: %v", err)
	}
	if stored.ReportsTo != manager.ID {
		t.Fatalf("worker reports_to = %q, want local manager %q", stored.ReportsTo, manager.ID)
	}
}
