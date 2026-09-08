package sqlite_test

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/office/routing"
)

// TestUpsertWorkspaceRouting_RoleTiersRoundTrip pins the JSON round-trip
// for the new role_tiers column, including AC-11's drop-empty-values
// behavior on write.
func TestUpsertWorkspaceRouting_RoleTiersRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	in := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Balanced: "sonnet", Economy: "haiku"}},
		},
		RoleTiers: routing.RoleTierMap{
			"qa":     routing.TierEconomy,
			"devops": routing.TierEconomy,
			"worker": "", // AC-11: dropped before persistence
		},
	}
	if err := repo.UpsertWorkspaceRouting(ctx, "ws-1", in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.GetWorkspaceRouting(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.RoleTiers["qa"] != routing.TierEconomy {
		t.Errorf("qa tier = %q, want economy", got.RoleTiers["qa"])
	}
	if got.RoleTiers["devops"] != routing.TierEconomy {
		t.Errorf("devops tier = %q, want economy", got.RoleTiers["devops"])
	}
	if _, ok := got.RoleTiers["worker"]; ok {
		t.Errorf("empty-value worker entry survived persistence: %+v", got.RoleTiers)
	}
}

// AC-40 test #1: a literal empty-string role_tiers column (the state a
// pre-migration row lands in via ADD COLUMN ... DEFAULT ”) decodes to an
// empty map with no error. This is the precedent-trap test proving
// decodeRoleTiers's raw=="" special case was necessary — the sibling
// tier_per_reason column only needs COALESCE(..., '{}') because SQL NULL
// is its only "missing" state, but role_tiers can also be a literal ”
// coming from the migration's own DEFAULT clause.
func TestGetWorkspaceRouting_EmptyStringRoleTiersColumn(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO office_workspace_routing
			(workspace_id, enabled, default_tier, provider_order, provider_profiles, role_tiers, updated_at)
		VALUES (?, 1, 'balanced', '[]', '{}', '', datetime('now'))
	`, "ws-empty-roles"); err != nil {
		t.Fatalf("insert row with literal empty role_tiers: %v", err)
	}

	cfg, err := repo.GetWorkspaceRouting(ctx, "ws-empty-roles")
	if err != nil {
		t.Fatalf("GetWorkspaceRouting: %v", err)
	}
	if len(cfg.RoleTiers) != 0 {
		t.Errorf("RoleTiers = %+v, want empty map", cfg.RoleTiers)
	}
}

// AC-33: malformed (non-empty, non-JSON) role_tiers content is rejected
// rather than silently swallowed.
func TestGetWorkspaceRouting_MalformedRoleTiersColumn(t *testing.T) {
	repo, _ := newTestRepoWithDB(t)
	ctx := context.Background()
	if _, err := repo.ExecRaw(ctx, `
		INSERT INTO office_workspace_routing
			(workspace_id, enabled, default_tier, provider_order, provider_profiles, role_tiers, updated_at)
		VALUES (?, 1, 'balanced', '[]', '{}', 'not-json', datetime('now'))
	`, "ws-bad-roles"); err != nil {
		t.Fatalf("insert row with malformed role_tiers: %v", err)
	}

	_, err := repo.GetWorkspaceRouting(ctx, "ws-bad-roles")
	if err == nil {
		t.Fatal("expected error decoding malformed role_tiers")
	}
	if !strings.Contains(err.Error(), "role_tiers") {
		t.Errorf("error should name role_tiers, got %v", err)
	}
}

// defaultWorkspaceRouting (the never-configured-workspace path) also
// carries a non-nil, empty RoleTiers map.
func TestGetWorkspaceRouting_DefaultOnEmpty_RoleTiers(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	cfg, err := repo.GetWorkspaceRouting(ctx, "ws-never-configured")
	if err != nil {
		t.Fatalf("GetWorkspaceRouting: %v", err)
	}
	if len(cfg.RoleTiers) != 0 {
		t.Errorf("RoleTiers = %+v, want empty map", cfg.RoleTiers)
	}
}
