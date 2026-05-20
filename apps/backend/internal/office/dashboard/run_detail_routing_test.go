package dashboard

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

// fakeRunDetailRepo satisfies just the RunDetailRepo methods exercised
// by buildRunRouting. The other methods are unused by these tests.
type fakeRunDetailRepo struct {
	attempts []models.RouteAttempt
	err      error
}

func (f *fakeRunDetailRepo) GetRunWithCosts(_ context.Context, _ string) (*models.Run, *sqlite.RunCostRollup, error) {
	return nil, nil, nil
}
func (f *fakeRunDetailRepo) ListTasksTouchedByRun(_ context.Context, _ string) ([]string, error) {
	return nil, nil
}
func (f *fakeRunDetailRepo) ListRunEvents(_ context.Context, _ string, _, _ int) ([]*models.RunEvent, error) {
	return nil, nil
}
func (f *fakeRunDetailRepo) ListRunSkillSnapshots(_ context.Context, _ string) ([]models.RunSkillSnapshot, error) {
	return nil, nil
}
func (f *fakeRunDetailRepo) ListRunsForAgentPaged(_ context.Context, _ string, _ time.Time, _ string, _ int) ([]*models.Run, error) {
	return nil, nil
}
func (f *fakeRunDetailRepo) GetAgentInstance(_ context.Context, _ string) (*models.AgentInstance, error) {
	return nil, nil
}
func (f *fakeRunDetailRepo) ListRouteAttempts(_ context.Context, _ string) ([]models.RouteAttempt, error) {
	return f.attempts, f.err
}

func TestBuildRunRouting_NilWhenNoAttemptsAndNoSnapshot(t *testing.T) {
	repo := &fakeRunDetailRepo{}
	out, err := buildRunRouting(context.Background(), repo, &models.Run{ID: "r"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out != nil {
		t.Fatalf("expected nil, got %+v", out)
	}
}

func TestBuildRunRouting_PopulatesAttemptsAndSnapshot(t *testing.T) {
	order := `["claude-acp","codex-acp"]`
	tier := "frontier"
	prov := "claude-acp"
	model := "opus"
	blocked := models.RoutingBlockedStatus("waiting_for_provider_capacity")
	retry := time.Now().UTC().Add(time.Minute)
	run := &models.Run{
		ID:                   "r1",
		LogicalProviderOrder: &order,
		RequestedTier:        &tier,
		ResolvedProviderID:   &prov,
		ResolvedModel:        &model,
		RoutingBlockedStatus: &blocked,
		EarliestRetryAt:      &retry,
	}
	repo := &fakeRunDetailRepo{
		attempts: []models.RouteAttempt{
			{RunID: "r1", Seq: 1, ProviderID: "claude-acp", Tier: "frontier",
				Outcome: "failed_provider_unavailable", ErrorCode: "quota_limited",
				StartedAt: time.Now().UTC()},
		},
	}
	out, err := buildRunRouting(context.Background(), repo, run)
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if out == nil {
		t.Fatalf("expected non-nil routing block")
	}
	if out.RequestedTier != "frontier" || out.ResolvedProviderID != "claude-acp" {
		t.Errorf("snapshot mismatch: %+v", out)
	}
	if len(out.LogicalProviderOrder) != 2 || out.LogicalProviderOrder[0] != "claude-acp" {
		t.Errorf("order = %v", out.LogicalProviderOrder)
	}
	if out.BlockedStatus != string(blocked) {
		t.Errorf("blocked = %q", out.BlockedStatus)
	}
	if out.EarliestRetryAt == nil {
		t.Errorf("earliest retry should be set")
	}
	if len(out.Attempts) != 1 || out.Attempts[0].ProviderID != "claude-acp" {
		t.Errorf("attempts = %+v", out.Attempts)
	}
}
