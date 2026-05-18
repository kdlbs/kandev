package sqlite_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/routing"
)

func TestGetWorkspaceRouting_DefaultOnEmpty(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	cfg, err := repo.GetWorkspaceRouting(ctx, "ws-1")
	if err != nil {
		t.Fatalf("GetWorkspaceRouting: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil default config")
	}
	if cfg.Enabled {
		t.Error("expected Enabled=false by default")
	}
	if cfg.DefaultTier != routing.TierBalanced {
		t.Errorf("DefaultTier = %q, want balanced", cfg.DefaultTier)
	}
	if len(cfg.ProviderOrder) != 0 {
		t.Errorf("ProviderOrder = %v, want empty", cfg.ProviderOrder)
	}
	if len(cfg.ProviderProfiles) != 0 {
		t.Errorf("ProviderProfiles = %v, want empty", cfg.ProviderProfiles)
	}
}

func TestUpsertAndGetWorkspaceRouting_RoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	in := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp", "codex-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {
				TierMap: routing.TierMap{Frontier: "opus", Balanced: "sonnet"},
				Mode:    "default",
				Flags:   []string{"--quiet"},
				Env:     map[string]string{"FOO": "bar"},
			},
			"codex-acp": {
				TierMap: routing.TierMap{Balanced: "gpt-5.4"},
			},
		},
	}
	if err := repo.UpsertWorkspaceRouting(ctx, "ws-1", in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.GetWorkspaceRouting(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if !got.Enabled {
		t.Error("Enabled lost on round-trip")
	}
	if got.DefaultTier != routing.TierBalanced {
		t.Errorf("DefaultTier = %q", got.DefaultTier)
	}
	if len(got.ProviderOrder) != 2 || got.ProviderOrder[0] != "claude-acp" {
		t.Errorf("ProviderOrder = %v", got.ProviderOrder)
	}
	claude := got.ProviderProfiles["claude-acp"]
	if claude.TierMap.Frontier != "opus" || claude.TierMap.Balanced != "sonnet" {
		t.Errorf("claude tier map: %+v", claude.TierMap)
	}
	if claude.Mode != "default" {
		t.Errorf("claude profile lost fields: %+v", claude)
	}
	if claude.Env["FOO"] != "bar" {
		t.Errorf("claude env lost: %+v", claude.Env)
	}
	if len(claude.Flags) != 1 || claude.Flags[0] != "--quiet" {
		t.Errorf("claude flags = %v", claude.Flags)
	}
}

func TestUpsertWorkspaceRouting_Idempotent(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()

	first := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierFrontier,
		ProviderOrder: []routing.ProviderID{"claude-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Frontier: "opus"}},
		},
	}
	if err := repo.UpsertWorkspaceRouting(ctx, "ws-1", first); err != nil {
		t.Fatalf("first upsert: %v", err)
	}

	second := &routing.WorkspaceConfig{
		Enabled:       false,
		DefaultTier:   routing.TierEconomy,
		ProviderOrder: []routing.ProviderID{"codex-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"codex-acp": {TierMap: routing.TierMap{Economy: "gpt-5.3-mini"}},
		},
	}
	if err := repo.UpsertWorkspaceRouting(ctx, "ws-1", second); err != nil {
		t.Fatalf("second upsert: %v", err)
	}

	got, err := repo.GetWorkspaceRouting(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Enabled {
		t.Error("expected Enabled=false after overwrite")
	}
	if got.DefaultTier != routing.TierEconomy {
		t.Errorf("DefaultTier = %q, want economy", got.DefaultTier)
	}
	if len(got.ProviderOrder) != 1 || got.ProviderOrder[0] != "codex-acp" {
		t.Errorf("order = %v, want [codex-acp]", got.ProviderOrder)
	}
	if _, ok := got.ProviderProfiles["claude-acp"]; ok {
		t.Error("stale claude profile not removed by upsert")
	}
}

// TestUpsertWorkspaceRouting_TierPerReasonRoundTrip pins the JSON
// round-trip for the wake-reason tier policy column.
func TestUpsertWorkspaceRouting_TierPerReasonRoundTrip(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	in := &routing.WorkspaceConfig{
		Enabled:       true,
		DefaultTier:   routing.TierBalanced,
		ProviderOrder: []routing.ProviderID{"claude-acp"},
		ProviderProfiles: map[routing.ProviderID]routing.ProviderProfile{
			"claude-acp": {TierMap: routing.TierMap{Balanced: "sonnet", Economy: "haiku"}},
		},
		TierPerReason: routing.TierPerReason{
			routing.WakeReasonHeartbeat:      routing.TierEconomy,
			routing.WakeReasonRoutineTrigger: routing.TierEconomy,
		},
	}
	if err := repo.UpsertWorkspaceRouting(ctx, "ws-1", in); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := repo.GetWorkspaceRouting(ctx, "ws-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.TierPerReason[routing.WakeReasonHeartbeat] != routing.TierEconomy {
		t.Errorf("heartbeat tier = %q", got.TierPerReason[routing.WakeReasonHeartbeat])
	}
	if got.TierPerReason[routing.WakeReasonRoutineTrigger] != routing.TierEconomy {
		t.Errorf("routine_trigger tier = %q", got.TierPerReason[routing.WakeReasonRoutineTrigger])
	}
}

func TestUpsertWorkspaceRouting_NilRejected(t *testing.T) {
	repo := newTestRepo(t)
	ctx := context.Background()
	if err := repo.UpsertWorkspaceRouting(ctx, "ws-1", nil); err == nil {
		t.Fatal("expected error on nil config")
	}
}
