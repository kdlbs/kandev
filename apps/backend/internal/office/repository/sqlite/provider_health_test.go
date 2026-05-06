package sqlite_test

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
)

func TestScopeFromCode(t *testing.T) {
	cases := []struct {
		name      string
		code      string
		model     string
		tier      routing.Tier
		wantScope string
		wantValue string
	}{
		{"model_unavailable", "model_unavailable", "opus", routing.TierFrontier,
			sqlite.HealthScopeModel, "opus"},
		{"provider_not_configured", "provider_not_configured", "", routing.TierFrontier,
			sqlite.HealthScopeTier, "frontier"},
		{"auth_required", "auth_required", "opus", routing.TierBalanced,
			sqlite.HealthScopeProvider, ""},
		{"quota_limited", "quota_limited", "opus", routing.TierBalanced,
			sqlite.HealthScopeProvider, ""},
		{"unknown_code_defaults_provider", "bogus", "", routing.TierBalanced,
			sqlite.HealthScopeProvider, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scope, value := sqlite.ScopeFromCode(tc.code, tc.model, tc.tier)
			if scope != tc.wantScope || value != tc.wantValue {
				t.Fatalf("ScopeFromCode = (%q,%q), want (%q,%q)",
					scope, value, tc.wantScope, tc.wantValue)
			}
		})
	}
}

func TestGetProviderHealth_MissingReturnsNil(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	got, err := repo.GetProviderHealth(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, "")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got != nil {
		t.Fatalf("expected nil for missing row, got %+v", got)
	}
}

func TestMarkProviderDegraded_FirstWriteThenBumpStep(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	retry := time.Now().UTC().Add(2 * time.Minute).Truncate(time.Second)

	h := models.ProviderHealth{
		WorkspaceID: "ws-1",
		ProviderID:  "claude-acp",
		Scope:       sqlite.HealthScopeProvider,
		ScopeValue:  "",
		State:       sqlite.HealthStateDegraded,
		ErrorCode:   "quota_limited",
		RetryAt:     &retry,
		BackoffStep: 0,
		RawExcerpt:  "anthropic_quota_exceeded",
	}
	if err := repo.MarkProviderDegraded(ctx, h); err != nil {
		t.Fatalf("first mark: %v", err)
	}
	got, err := repo.GetProviderHealth(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, "")
	if err != nil || got == nil {
		t.Fatalf("get: %v got=%v", err, got)
	}
	if got.BackoffStep != 0 {
		t.Errorf("first step = %d, want 0", got.BackoffStep)
	}
	if got.State != sqlite.HealthStateDegraded {
		t.Errorf("state = %q", got.State)
	}
	if got.ErrorCode != "quota_limited" {
		t.Errorf("error code = %q", got.ErrorCode)
	}

	// Second degraded → should bump step.
	if err := repo.MarkProviderDegraded(ctx, h); err != nil {
		t.Fatalf("second mark: %v", err)
	}
	got, _ = repo.GetProviderHealth(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, "")
	if got.BackoffStep != 1 {
		t.Errorf("after second mark step = %d, want 1", got.BackoffStep)
	}
	if err := repo.MarkProviderDegraded(ctx, h); err != nil {
		t.Fatalf("third mark: %v", err)
	}
	got, _ = repo.GetProviderHealth(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, "")
	if got.BackoffStep != 2 {
		t.Errorf("after third mark step = %d, want 2", got.BackoffStep)
	}
}

func TestMarkProviderDegraded_RejectsHealthy(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	h := models.ProviderHealth{
		WorkspaceID: "ws-1", ProviderID: "claude-acp",
		Scope: sqlite.HealthScopeProvider, ScopeValue: "",
		State: sqlite.HealthStateHealthy,
	}
	if err := repo.MarkProviderDegraded(ctx, h); err == nil {
		t.Fatal("expected error when marking degraded with state=healthy")
	}
}

func TestMarkProviderHealthy_ClearsBackoff(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	retry := time.Now().UTC().Add(5 * time.Minute)
	h := models.ProviderHealth{
		WorkspaceID: "ws-1", ProviderID: "claude-acp",
		Scope: sqlite.HealthScopeProvider, ScopeValue: "",
		State: sqlite.HealthStateDegraded, ErrorCode: "quota_limited",
		RetryAt: &retry, BackoffStep: 3,
	}
	if err := repo.MarkProviderDegraded(ctx, h); err != nil {
		t.Fatalf("seed degraded: %v", err)
	}

	if err := repo.MarkProviderHealthy(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, ""); err != nil {
		t.Fatalf("mark healthy: %v", err)
	}
	got, _ := repo.GetProviderHealth(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, "")
	if got == nil {
		t.Fatal("expected row after mark healthy")
	}
	if got.State != sqlite.HealthStateHealthy {
		t.Errorf("state = %q", got.State)
	}
	if got.BackoffStep != 0 {
		t.Errorf("step = %d, want 0", got.BackoffStep)
	}
	if got.RetryAt != nil {
		t.Errorf("retry_at = %v, want nil", got.RetryAt)
	}
	if got.ErrorCode != "" {
		t.Errorf("error_code = %q, want empty", got.ErrorCode)
	}
}

func TestMarkProviderHealthy_NoopOnMissingRow(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if err := repo.MarkProviderHealthy(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestMarkProviderDegraded_TransitionFromHealthyResetsStep(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	h := models.ProviderHealth{
		WorkspaceID: "ws-1", ProviderID: "claude-acp",
		Scope: sqlite.HealthScopeProvider, ScopeValue: "",
		State: sqlite.HealthStateDegraded, ErrorCode: "quota_limited",
		BackoffStep: 0,
	}
	if err := repo.MarkProviderDegraded(ctx, h); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := repo.MarkProviderHealthy(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, ""); err != nil {
		t.Fatalf("recover: %v", err)
	}
	if err := repo.MarkProviderDegraded(ctx, h); err != nil {
		t.Fatalf("second cycle: %v", err)
	}
	got, _ := repo.GetProviderHealth(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, "")
	if got.BackoffStep != 0 {
		t.Errorf("step after healthy-cycle = %d, want 0 (caller floor)",
			got.BackoffStep)
	}
}

// Multi-workspace isolation: every provider_health write must be
// scoped by workspace_id. A degrade on workspace A must not bleed into
// workspace B's view, and a recover on workspace A must not flip
// workspace B's row. The triple (workspace_id, provider_id, scope,
// scope_value) is the row key — this test pins that contract.
func TestProviderHealth_MultiWorkspaceIsolation(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	// Seed identical (provider, scope) rows in two different workspaces.
	wsA := models.ProviderHealth{
		WorkspaceID: "ws-a", ProviderID: "claude-acp",
		Scope: sqlite.HealthScopeProvider, ScopeValue: "",
		State: sqlite.HealthStateDegraded, ErrorCode: "rate_limited",
	}
	wsB := models.ProviderHealth{
		WorkspaceID: "ws-b", ProviderID: "claude-acp",
		Scope: sqlite.HealthScopeProvider, ScopeValue: "",
		State: sqlite.HealthStateDegraded, ErrorCode: "quota_limited",
	}
	if err := repo.MarkProviderDegraded(ctx, wsA); err != nil {
		t.Fatalf("seed ws-a: %v", err)
	}
	if err := repo.MarkProviderDegraded(ctx, wsB); err != nil {
		t.Fatalf("seed ws-b: %v", err)
	}

	// ListProviderHealth(ws-a) must not return ws-b's row.
	gotA, err := repo.ListProviderHealth(ctx, "ws-a")
	if err != nil {
		t.Fatalf("list ws-a: %v", err)
	}
	if len(gotA) != 1 || gotA[0].WorkspaceID != "ws-a" {
		t.Fatalf("ws-a list leaked rows: %+v", gotA)
	}
	if gotA[0].ErrorCode != "rate_limited" {
		t.Errorf("ws-a row picked up ws-b's error code: %+v", gotA[0])
	}

	// And the inverse: ws-b's view must only have ws-b's row.
	gotB, err := repo.ListProviderHealth(ctx, "ws-b")
	if err != nil {
		t.Fatalf("list ws-b: %v", err)
	}
	if len(gotB) != 1 || gotB[0].WorkspaceID != "ws-b" {
		t.Fatalf("ws-b list leaked rows: %+v", gotB)
	}

	// Recovering ws-a's row must NOT touch ws-b's row.
	if err := repo.MarkProviderHealthy(ctx, "ws-a", "claude-acp",
		sqlite.HealthScopeProvider, ""); err != nil {
		t.Fatalf("recover ws-a: %v", err)
	}
	stillB, err := repo.GetProviderHealth(ctx, "ws-b", "claude-acp",
		sqlite.HealthScopeProvider, "")
	if err != nil {
		t.Fatalf("get ws-b: %v", err)
	}
	if stillB == nil {
		t.Fatal("ws-b row was deleted by recovering ws-a")
	}
	if stillB.State != sqlite.HealthStateDegraded {
		t.Errorf("ws-b state flipped due to ws-a recover: %q", stillB.State)
	}
	if stillB.ErrorCode != "quota_limited" {
		t.Errorf("ws-b error code changed: %q", stillB.ErrorCode)
	}

	// And the inverse: degrading ws-b further must not raise ws-a's
	// backoff_step (its row is now healthy).
	if err := repo.MarkProviderDegraded(ctx, wsB); err != nil {
		t.Fatalf("redegrade ws-b: %v", err)
	}
	stillA, _ := repo.GetProviderHealth(ctx, "ws-a", "claude-acp",
		sqlite.HealthScopeProvider, "")
	if stillA != nil && stillA.State != sqlite.HealthStateHealthy {
		t.Errorf("ws-a row flipped degraded after ws-b degrade: %+v", stillA)
	}
}

func TestListProviderHealth_OmitsHealthy(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	degraded := models.ProviderHealth{
		WorkspaceID: "ws-1", ProviderID: "claude-acp",
		Scope: sqlite.HealthScopeProvider, ScopeValue: "",
		State: sqlite.HealthStateDegraded, ErrorCode: "quota_limited",
	}
	if err := repo.MarkProviderDegraded(ctx, degraded); err != nil {
		t.Fatalf("seed: %v", err)
	}
	other := models.ProviderHealth{
		WorkspaceID: "ws-1", ProviderID: "codex-acp",
		Scope: sqlite.HealthScopeModel, ScopeValue: "gpt-5.5",
		State: sqlite.HealthStateUserActionRequired, ErrorCode: "auth_required",
	}
	if err := repo.MarkProviderDegraded(ctx, other); err != nil {
		t.Fatalf("seed other: %v", err)
	}
	// Recover the first row — it should drop out of List.
	if err := repo.MarkProviderHealthy(ctx, "ws-1", "claude-acp",
		sqlite.HealthScopeProvider, ""); err != nil {
		t.Fatalf("recover: %v", err)
	}

	got, err := repo.ListProviderHealth(ctx, "ws-1")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 non-healthy row, got %d", len(got))
	}
	if got[0].ProviderID != "codex-acp" || got[0].Scope != sqlite.HealthScopeModel {
		t.Errorf("unexpected row: %+v", got[0])
	}
}
