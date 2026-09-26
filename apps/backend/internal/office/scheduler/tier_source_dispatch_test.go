package scheduler_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
)

// makeAgentWithRole mirrors makeAgent but sets the organisational Role so
// role_tiers precedence tests can exercise the resolver's role level.
func makeAgentWithRole(role settingsmodels.AgentRole) *officemodels.AgentInstance {
	agent := makeAgent()
	agent.Role = role
	return agent
}

// AC-20d/AC-20e: a launch that resolves at the workspace-default level
// persists tier_source="workspace" on the recorded attempt via
// recordAttemptStart.
func TestDispatch_TierSource_WorkspaceDefault(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	ss := buildScheduler(t, repo, newFakeTaskStarter())
	run := seedRun(t, repo, `{"task_id":"t-1"}`)

	launched, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true")
	}
	attempts, err := repo.ListRouteAttempts(context.Background(), run.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v err=%v, want one attempt", attempts, err)
	}
	if attempts[0].TierSource != "workspace" {
		t.Errorf("TierSource = %q, want workspace", attempts[0].TierSource)
	}
}

// The new role level: an agent whose role has a role_tiers entry
// launches at that tier, and the attempt records tier_source="role".
func TestDispatch_TierSource_RoleTier(t *testing.T) {
	repo := newTestRepoSched(t)
	profiles := map[routing.ProviderID]routing.ProviderProfile{
		"claude-acp": {
			TierMap: routing.TierMap{Balanced: "sonnet", Economy: "haiku"},
			ExecutionProfileIDs: routing.ExecutionProfileIDs{
				Balanced: "claude-sonnet-profile",
				Economy:  "claude-haiku-profile",
			},
			Mode: "default",
		},
	}
	cfg := &routing.WorkspaceConfig{
		Enabled:          true,
		DefaultTier:      routing.TierBalanced,
		ProviderOrder:    []routing.ProviderID{"claude-acp"},
		ProviderProfiles: profiles,
		RoleTiers:        routing.RoleTierMap{"qa": routing.TierEconomy},
	}
	if err := repo.UpsertWorkspaceRouting(context.Background(), testWorkspaceID, cfg); err != nil {
		t.Fatalf("upsert routing: %v", err)
	}
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)

	launched, _, err := ss.DispatchWithRouting(
		context.Background(), run, makeAgentWithRole(settingsmodels.AgentRoleQA), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true")
	}
	got := starter.lastCall().route
	if got.Tier != string(routing.TierEconomy) || got.Model != "haiku" {
		t.Errorf("route = %+v, want economy/haiku", got)
	}
	attempts, err := repo.ListRouteAttempts(context.Background(), run.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v err=%v, want one attempt", attempts, err)
	}
	if attempts[0].TierSource != "role" {
		t.Errorf("TierSource = %q, want role", attempts[0].TierSource)
	}
}

// The top level: a matching wake-reason policy resolves the tier and the
// attempt records tier_source="wake_reason", outranking a role entry.
func TestDispatch_TierSource_WakeReasonBeatsRole(t *testing.T) {
	repo := newTestRepoSched(t)
	profiles := map[routing.ProviderID]routing.ProviderProfile{
		"claude-acp": {
			TierMap: routing.TierMap{Frontier: "opus", Balanced: "sonnet", Economy: "haiku"},
			ExecutionProfileIDs: routing.ExecutionProfileIDs{
				Frontier: "claude-opus-profile",
				Balanced: "claude-sonnet-profile",
				Economy:  "claude-haiku-profile",
			},
			Mode: "default",
		},
	}
	cfg := &routing.WorkspaceConfig{
		Enabled:          true,
		DefaultTier:      routing.TierBalanced,
		ProviderOrder:    []routing.ProviderID{"claude-acp"},
		ProviderProfiles: profiles,
		RoleTiers:        routing.RoleTierMap{"qa": routing.TierFrontier},
		TierPerReason: routing.TierPerReason{
			routing.WakeReasonHeartbeat: routing.TierEconomy,
		},
	}
	if err := repo.UpsertWorkspaceRouting(context.Background(), testWorkspaceID, cfg); err != nil {
		t.Fatalf("upsert routing: %v", err)
	}
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	run.Reason = routing.WakeReasonHeartbeat

	launched, _, err := ss.DispatchWithRouting(
		context.Background(), run, makeAgentWithRole(settingsmodels.AgentRoleQA), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true")
	}
	got := starter.lastCall().route
	if got.Tier != string(routing.TierEconomy) || got.Model != "haiku" {
		t.Errorf("route = %+v, want economy/haiku (wake reason beats role)", got)
	}
	attempts, err := repo.ListRouteAttempts(context.Background(), run.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v err=%v, want one attempt", attempts, err)
	}
	if attempts[0].TierSource != "wake_reason" {
		t.Errorf("TierSource = %q, want wake_reason", attempts[0].TierSource)
	}
}

// parkRunBlocked's skip-attempt rows also carry tier_source, propagated
// from the resolution's block-reason path rather than a launch attempt.
func TestDispatch_TierSource_ParkedBlockedRecordsSource(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := newFakeTaskStarter()
	starter.failFor["claude-acp"] = fmt.Errorf("missing API key: not authenticated")
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)

	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if launched || !parked {
		t.Fatalf("expected parked; got launched=%v parked=%v", launched, parked)
	}
	attempts, err := repo.ListRouteAttempts(context.Background(), run.ID)
	if err != nil || len(attempts) != 1 {
		t.Fatalf("attempts = %v err=%v, want one attempt", attempts, err)
	}
	if attempts[0].TierSource != "workspace" {
		t.Errorf("TierSource = %q, want workspace", attempts[0].TierSource)
	}
}

// AC-20f/AC-20g: the terminal skipped_max_attempts row parkRunMaxAttempts
// records carries no Tier today, and correspondingly no TierSource — the
// cap fires before the resolver ever runs for this dispatch call.
func TestDispatch_TierSource_MaxAttemptsRowLeavesSourceEmpty(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)

	for i := 0; i < scheduler.MaxAttemptsPerRun; i++ {
		seq, err := repo.IncrementRouteAttemptSeq(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("seed seq bump: %v", err)
		}
		if err := repo.AppendRouteAttempt(context.Background(), &officemodels.RouteAttempt{
			RunID:      run.ID,
			Seq:        seq,
			ProviderID: "claude-acp",
			Model:      "claude-acp-bal",
			Tier:       "balanced",
			TierSource: "workspace",
			Outcome:    scheduler.RouteAttemptOutcomeFailedProviderUnavail,
			StartedAt:  time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed prior attempt: %v", err)
		}
	}

	launched, parked, err := ss.DispatchWithRouting(context.Background(),
		run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if launched || !parked {
		t.Fatalf("expected parked at cap; got launched=%v parked=%v", launched, parked)
	}
	attempts, err := repo.ListRouteAttempts(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	last := attempts[len(attempts)-1]
	if last.Outcome != scheduler.RouteAttemptOutcomeMaxAttempts {
		t.Fatalf("last outcome = %q, want skipped_max_attempts", last.Outcome)
	}
	if last.TierSource != "" {
		t.Errorf("TierSource = %q, want empty on the max-attempts terminal row", last.TierSource)
	}
}

// The route_attempt_appended WS event carries the same TierSource the
// persisted attempt row does — the payload is the raw models.RouteAttempt,
// not a re-derived value, so this pins the wiring rather than the value.
func TestDispatch_TierSource_PublishedOnRouteAttemptAppended(t *testing.T) {
	repo := newTestRepoSched(t)
	profiles := map[routing.ProviderID]routing.ProviderProfile{
		"claude-acp": {
			TierMap: routing.TierMap{Balanced: "sonnet", Economy: "haiku"},
			ExecutionProfileIDs: routing.ExecutionProfileIDs{
				Balanced: "claude-sonnet-profile",
				Economy:  "claude-haiku-profile",
			},
			Mode: "default",
		},
	}
	cfg := &routing.WorkspaceConfig{
		Enabled:          true,
		DefaultTier:      routing.TierBalanced,
		ProviderOrder:    []routing.ProviderID{"claude-acp"},
		ProviderProfiles: profiles,
		RoleTiers:        routing.RoleTierMap{"qa": routing.TierEconomy},
	}
	if err := repo.UpsertWorkspaceRouting(context.Background(), testWorkspaceID, cfg); err != nil {
		t.Fatalf("upsert routing: %v", err)
	}
	ss := buildScheduler(t, repo, newFakeTaskStarter())

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	eb := bus.NewMemoryEventBus(log)
	ss.SetEventBus(eb)

	var captured *officemodels.RouteAttempt
	if _, err := eb.Subscribe(events.OfficeRouteAttemptAppended, func(_ context.Context, ev *bus.Event) error {
		data, ok := ev.Data.(map[string]interface{})
		if !ok {
			t.Fatalf("event data type = %T, want map[string]interface{}", ev.Data)
		}
		attempt, ok := data["attempt"].(officemodels.RouteAttempt)
		if !ok {
			t.Fatalf("attempt field type = %T, want officemodels.RouteAttempt", data["attempt"])
		}
		captured = &attempt
		return nil
	}); err != nil {
		t.Fatalf("subscribe: %v", err)
	}

	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	launched, _, err := ss.DispatchWithRouting(
		context.Background(), run, makeAgentWithRole(settingsmodels.AgentRoleQA), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true")
	}
	if captured == nil {
		t.Fatal("route_attempt_appended was not published")
	}
	if captured.TierSource != "role" {
		t.Errorf("published attempt TierSource = %q, want role", captured.TierSource)
	}
}
