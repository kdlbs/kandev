package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// spendWindowFixture seeds the six-event scenario AC-OFFICE-BUDGET-002.10/.15
// and REQ-OFFICE-BUDGET-004 exercise: two agents in two workspaces, two
// projects, one event before the window, one exactly at the window's
// exclusive upper bound, and one unpriced event inside the window.
//
//	e1  agent-1 ws-1 proj-1  T0-2h   cost=100  priced   (before window)
//	e2  agent-1 ws-1 proj-1  T0+1h   cost=200  priced   (in window)
//	e3  agent-1 ws-1 proj-2  T0+1h   cost=50   priced   (in window, other project)
//	e4  agent-2 ws-2 proj-3  T0+1h   cost=999  priced   (in window, other workspace)
//	e5  agent-1 ws-1 proj-1  T1      cost=500  priced   (at exclusive upper bound)
//	e6  agent-1 ws-1 proj-1  T0+2h   cost=0    unpriced (in window)
func spendWindowFixture(t *testing.T, repo *sqlite.Repository) (t0, t1 time.Time) {
	t.Helper()
	ctx := context.Background()

	createSpendWindowTestAgent(t, repo, "ws-1", "agent-1")
	createSpendWindowTestAgent(t, repo, "ws-2", "agent-2")

	t0 = time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	t1 = time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	unpriced := models.CostSourceUnpriced

	events := []*models.CostEvent{
		{ID: "e1", AgentProfileID: "agent-1", ProjectID: "proj-1", CostSubcents: 100, OccurredAt: t0.Add(-2 * time.Hour)},
		{ID: "e2", AgentProfileID: "agent-1", ProjectID: "proj-1", CostSubcents: 200, OccurredAt: t0.Add(1 * time.Hour)},
		{ID: "e3", AgentProfileID: "agent-1", ProjectID: "proj-2", CostSubcents: 50, OccurredAt: t0.Add(1 * time.Hour)},
		{ID: "e4", AgentProfileID: "agent-2", ProjectID: "proj-3", CostSubcents: 999, OccurredAt: t0.Add(1 * time.Hour)},
		{ID: "e5", AgentProfileID: "agent-1", ProjectID: "proj-1", CostSubcents: 500, OccurredAt: t1},
		{ID: "e6", AgentProfileID: "agent-1", ProjectID: "proj-1", CostSubcents: 0, OccurredAt: t0.Add(2 * time.Hour), CostSource: &unpriced},
	}
	for _, e := range events {
		if err := repo.CreateCostEvent(ctx, e); err != nil {
			t.Fatalf("create cost event %s: %v", e.ID, err)
		}
	}
	return t0, t1
}

func createSpendWindowTestAgent(t *testing.T, repo *sqlite.Repository, wsID, agentID string) {
	t.Helper()
	agent := &models.AgentInstance{
		ID:          agentID,
		WorkspaceID: wsID,
		Name:        "test-" + agentID,
		Role:        models.AgentRoleWorker,
		Status:      models.AgentStatusIdle,
	}
	if err := repo.CreateAgentInstance(context.Background(), agent); err != nil {
		t.Fatalf("create test agent %s: %v", agentID, err)
	}
}

func TestSpendWindowForWorkspace(t *testing.T) {
	repo := newTestRepo(t)
	t0, t1 := spendWindowFixture(t, repo)
	ctx := context.Background()

	got, err := repo.SpendWindowForWorkspace(ctx, "ws-1", t0, true, t1)
	if err != nil {
		t.Fatalf("SpendWindowForWorkspace: %v", err)
	}
	if got.PricedSubcents != 250 {
		t.Errorf("PricedSubcents = %d, want 250 (e2=200 + e3=50 + e6=0; excludes e1 before window, e4 other workspace, e5 at upper bound)", got.PricedSubcents)
	}
	if !got.Degraded {
		t.Error("Degraded = false, want true (e6 is unpriced and in window)")
	}
}

func TestSpendWindowForWorkspace_TotalPeriodOmitsLowerBound(t *testing.T) {
	repo := newTestRepo(t)
	_, t1 := spendWindowFixture(t, repo)
	ctx := context.Background()

	got, err := repo.SpendWindowForWorkspace(ctx, "ws-1", time.Time{}, false, t1)
	if err != nil {
		t.Fatalf("SpendWindowForWorkspace: %v", err)
	}
	if got.PricedSubcents != 350 {
		t.Errorf("PricedSubcents = %d, want 350 (e1=100 + e2=200 + e3=50 + e6=0; still excludes e5 at upper bound, e4 other workspace)", got.PricedSubcents)
	}
}

func TestSpendWindowForAgent(t *testing.T) {
	repo := newTestRepo(t)
	t0, t1 := spendWindowFixture(t, repo)
	ctx := context.Background()

	got, err := repo.SpendWindowForAgent(ctx, "agent-1", t0, true, t1)
	if err != nil {
		t.Fatalf("SpendWindowForAgent: %v", err)
	}
	if got.PricedSubcents != 250 {
		t.Errorf("PricedSubcents = %d, want 250", got.PricedSubcents)
	}
	if !got.Degraded {
		t.Error("Degraded = false, want true")
	}
}

func TestSpendWindowForAgent_NoUnpricedEventsIsNotDegraded(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	createSpendWindowTestAgent(t, repo, "ws-1", "agent-clean")

	t0 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	if err := repo.CreateCostEvent(ctx, &models.CostEvent{
		ID: "clean-1", AgentProfileID: "agent-clean", ProjectID: "proj-1",
		CostSubcents: 42, OccurredAt: t0.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create cost event: %v", err)
	}

	got, err := repo.SpendWindowForAgent(ctx, "agent-clean", t0, true, t1)
	if err != nil {
		t.Fatalf("SpendWindowForAgent: %v", err)
	}
	if got.PricedSubcents != 42 {
		t.Errorf("PricedSubcents = %d, want 42", got.PricedSubcents)
	}
	if got.Degraded {
		t.Error("Degraded = true, want false (no unpriced events)")
	}
}

func TestSpendWindowForAgent_NoEventsIsZeroNotError(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	createSpendWindowTestAgent(t, repo, "ws-1", "agent-empty")

	t0 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	got, err := repo.SpendWindowForAgent(ctx, "agent-empty", t0, true, t1)
	if err != nil {
		t.Fatalf("SpendWindowForAgent: %v", err)
	}
	if got.PricedSubcents != 0 || got.Degraded {
		t.Errorf("got %+v, want zero value", got)
	}
}

func TestSpendWindowForProject(t *testing.T) {
	repo := newTestRepo(t)
	t0, t1 := spendWindowFixture(t, repo)
	ctx := context.Background()

	got, err := repo.SpendWindowForProject(ctx, "proj-1", t0, true, t1)
	if err != nil {
		t.Fatalf("SpendWindowForProject: %v", err)
	}
	if got.PricedSubcents != 200 {
		t.Errorf("PricedSubcents = %d, want 200 (e2=200 + e6=0; excludes e3/e4 other projects, e1 before window, e5 at upper bound)", got.PricedSubcents)
	}
	if !got.Degraded {
		t.Error("Degraded = false, want true (e6 is unpriced, project proj-1, in window)")
	}
}

func TestSpendWindowForProject_DoesNotUseTaskCurrentProject(t *testing.T) {
	// Reparenting a task must not move its historical spend between
	// ceilings (AC-OFFICE-BUDGET-002.15): the query filters on the cost
	// event's own project_id, never a task join, so this test seeds no
	// tasks table at all and still resolves correctly.
	repo := newTestRepo(t)
	ctx := context.Background()
	createSpendWindowTestAgent(t, repo, "ws-1", "agent-1")

	t0 := time.Date(2026, 3, 15, 0, 0, 0, 0, time.UTC)
	t1 := time.Date(2026, 3, 16, 0, 0, 0, 0, time.UTC)
	if err := repo.CreateCostEvent(ctx, &models.CostEvent{
		ID: "reparent-1", AgentProfileID: "agent-1", ProjectID: "proj-original",
		CostSubcents: 77, OccurredAt: t0.Add(time.Hour),
	}); err != nil {
		t.Fatalf("create cost event: %v", err)
	}

	got, err := repo.SpendWindowForProject(ctx, "proj-original", t0, true, t1)
	if err != nil {
		t.Fatalf("SpendWindowForProject: %v", err)
	}
	if got.PricedSubcents != 77 {
		t.Errorf("PricedSubcents = %d, want 77", got.PricedSubcents)
	}
}
