package scheduler_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
)

const (
	testWorkspaceID = "ws-1"
	testAgentID     = "agent-1"
)

type recordedStart struct {
	taskID  string
	agentID string
	launch  scheduler.LaunchContext
	route   scheduler.RouteOverride
}

type fakeTaskStarter struct {
	mu      sync.Mutex
	calls   []recordedStart
	failFor map[string]error
}

func newFakeTaskStarter() *fakeTaskStarter {
	return &fakeTaskStarter{failFor: map[string]error{}}
}

func (f *fakeTaskStarter) StartTask(
	context.Context, string, string, string, string, string, string, string, bool, []interface{},
) error {
	return fmt.Errorf("StartTask not implemented in fake")
}

func (f *fakeTaskStarter) StartTaskWithRoute(
	_ context.Context, taskID, agentID string,
	launch scheduler.LaunchContext, route scheduler.RouteOverride,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, recordedStart{taskID, agentID, launch, route})
	if err, ok := f.failFor[route.ProviderID]; ok {
		return err
	}
	return nil
}

func (f *fakeTaskStarter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeTaskStarter) lastCall() recordedStart {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.calls) == 0 {
		return recordedStart{}
	}
	return f.calls[len(f.calls)-1]
}

func newTestRepoSched(t *testing.T) *officesqlite.Repository {
	t.Helper()
	db, err := sqlx.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if _, _, err := settingsstore.Provide(db, db, nil); err != nil {
		t.Fatalf("settings store: %v", err)
	}
	repo, err := officesqlite.NewWithDB(db, db, nil)
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}
	return repo
}

type repoAdapter struct{ repo *officesqlite.Repository }

func (a *repoAdapter) GetWorkspaceRouting(ctx context.Context, ws string) (*routing.WorkspaceConfig, error) {
	return a.repo.GetWorkspaceRouting(ctx, ws)
}
func (a *repoAdapter) ListProviderHealth(ctx context.Context, ws string) ([]officemodels.ProviderHealth, error) {
	return a.repo.ListProviderHealth(ctx, ws)
}

func buildScheduler(t *testing.T, repo *officesqlite.Repository, starter scheduler.TaskStarter) *scheduler.SchedulerService {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	ss := scheduler.NewSchedulerService(repo, log, nil)
	ss.SetResolver(routing.NewResolver(&repoAdapter{repo: repo}, nil))
	ss.SetTaskStarter(starter)
	return ss
}

func seedRoutingConfig(t *testing.T, repo *officesqlite.Repository, providers []routing.ProviderID) {
	t.Helper()
	profiles := map[routing.ProviderID]routing.ProviderProfile{}
	for _, p := range providers {
		profiles[p] = routing.ProviderProfile{
			TierMap: routing.TierMap{Balanced: string(p) + "-bal"},
			Mode:    "default",
		}
	}
	cfg := &routing.WorkspaceConfig{
		Enabled:          true,
		DefaultTier:      routing.TierBalanced,
		ProviderOrder:    providers,
		ProviderProfiles: profiles,
	}
	if err := repo.UpsertWorkspaceRouting(context.Background(), testWorkspaceID, cfg); err != nil {
		t.Fatalf("upsert routing: %v", err)
	}
}

func seedRun(t *testing.T, repo *officesqlite.Repository, payload string) *officemodels.Run {
	t.Helper()
	run := &officemodels.Run{
		ID:             "run-" + t.Name(),
		AgentProfileID: testAgentID,
		Reason:         "task_assigned",
		Payload:        payload,
		Status:         "claimed",
		CoalescedCount: 1,
		RequestedAt:    time.Now().UTC(),
	}
	if err := repo.CreateRun(context.Background(), run); err != nil {
		t.Fatalf("create run: %v", err)
	}
	return run
}

func makeAgent() *officemodels.AgentInstance {
	return &settingsmodels.AgentProfile{
		ID:          testAgentID,
		Name:        "Coordinator",
		WorkspaceID: testWorkspaceID,
		Settings:    "{}",
	}
}

func TestDispatch_RoutingDisabled_FallsThrough(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if launched || parked {
		t.Fatalf("expected fall-through (launched=false, parked=false); got launched=%v parked=%v", launched, parked)
	}
	if starter.callCount() != 0 {
		t.Fatalf("starter should not be called when routing disabled")
	}
	attempts, _ := repo.ListRouteAttempts(context.Background(), run.ID)
	if len(attempts) != 0 {
		t.Fatalf("no attempts expected; got %d", len(attempts))
	}
}

func TestDispatch_ForwardsLaunchContextToStarter(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	launch := scheduler.LaunchContext{
		Prompt:    "office-built-prompt-body",
		ProfileID: "profile-xyz",
		Env:       map[string]string{"KANDEV_API_URL": "http://x"},
	}
	launched, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), launch)
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true")
	}
	got := starter.lastCall()
	if got.launch.Prompt != "office-built-prompt-body" {
		t.Errorf("prompt not propagated: %q", got.launch.Prompt)
	}
	if got.launch.ProfileID != "profile-xyz" {
		t.Errorf("profile_id not propagated: %q", got.launch.ProfileID)
	}
	if got.launch.Env["KANDEV_API_URL"] != "http://x" {
		t.Errorf("env not propagated: %v", got.launch.Env)
	}
}

func TestDispatch_FirstProviderSucceeds(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched || parked {
		t.Fatalf("expected launched=true; got launched=%v parked=%v", launched, parked)
	}
	if starter.callCount() != 1 {
		t.Fatalf("expected 1 starter call; got %d", starter.callCount())
	}
	if got := starter.lastCall().route.ProviderID; got != "claude-acp" {
		t.Errorf("expected first provider claude-acp; got %s", got)
	}
	attempts, _ := repo.ListRouteAttempts(context.Background(), run.ID)
	if len(attempts) != 1 {
		t.Fatalf("expected 1 attempt; got %d", len(attempts))
	}
	if attempts[0].Outcome != scheduler.RouteAttemptOutcomeLaunched {
		t.Errorf("outcome = %q, want launched", attempts[0].Outcome)
	}
	updated, _ := repo.GetRun(context.Background(), run.ID)
	if updated.ResolvedProviderID == nil || *updated.ResolvedProviderID != "claude-acp" {
		t.Errorf("resolved provider not persisted")
	}
}

// TestDispatch_WakeReasonTierPolicy verifies the run's wake reason
// flows through to the resolver and pins the candidate to the
// workspace-configured tier for that reason.
func TestDispatch_WakeReasonTierPolicy(t *testing.T) {
	repo := newTestRepoSched(t)
	// Workspace defaults to balanced; heartbeat is pinned to economy.
	profiles := map[routing.ProviderID]routing.ProviderProfile{
		"claude-acp": {
			TierMap: routing.TierMap{Balanced: "sonnet", Economy: "haiku"},
			Mode:    "default",
		},
	}
	cfg := &routing.WorkspaceConfig{
		Enabled:          true,
		DefaultTier:      routing.TierBalanced,
		ProviderOrder:    []routing.ProviderID{"claude-acp"},
		ProviderProfiles: profiles,
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
	launched, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true")
	}
	got := starter.lastCall().route
	if got.Tier != string(routing.TierEconomy) {
		t.Errorf("tier = %q, want economy", got.Tier)
	}
	if got.Model != "haiku" {
		t.Errorf("model = %q, want haiku", got.Model)
	}
}

// TestDispatch_FailedAttemptKeepsProviderModel asserts that when the
// dispatcher records a failed launch attempt, the stored row retains
// the provider/model/tier from the start of the attempt. Previously
// finishAttempt published a partial row that the frontend reducer
// replaced over the full row, wiping provider+model in the UI.
func TestDispatch_FailedAttemptKeepsProviderModel(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	starter.failFor["claude-acp"] = fmt.Errorf("anthropic_quota_exceeded")
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	_, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	attempts, _ := repo.ListRouteAttempts(context.Background(), run.ID)
	if len(attempts) != 2 {
		t.Fatalf("expected 2 attempts; got %d", len(attempts))
	}
	failed := attempts[0]
	if failed.ProviderID != "claude-acp" {
		t.Errorf("failed attempt provider lost: %q", failed.ProviderID)
	}
	if failed.Model == "" {
		t.Errorf("failed attempt model lost: %q", failed.Model)
	}
	if failed.Tier == "" {
		t.Errorf("failed attempt tier lost: %q", failed.Tier)
	}
	if failed.StartedAt.IsZero() {
		t.Errorf("failed attempt started_at lost")
	}
	if failed.Outcome != scheduler.RouteAttemptOutcomeFailedProviderUnavail {
		t.Errorf("failed outcome = %q", failed.Outcome)
	}
}

func TestDispatch_FirstQuotaSecondSucceeds(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	starter.failFor["claude-acp"] = fmt.Errorf("anthropic_quota_exceeded: please try again")
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	launched, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true after fallback")
	}
	if starter.callCount() != 2 {
		t.Fatalf("expected 2 starter calls; got %d", starter.callCount())
	}
	attempts, _ := repo.ListRouteAttempts(context.Background(), run.ID)
	if len(attempts) != 2 {
		t.Fatalf("expected 2 attempts; got %d", len(attempts))
	}
	if attempts[0].Outcome != scheduler.RouteAttemptOutcomeFailedProviderUnavail {
		t.Errorf("first outcome = %q, want failed_provider_unavailable", attempts[0].Outcome)
	}
	if attempts[0].ErrorCode == "" {
		t.Error("first attempt missing classifier error code")
	}
	if attempts[1].Outcome != scheduler.RouteAttemptOutcomeLaunched {
		t.Errorf("second outcome = %q, want launched", attempts[1].Outcome)
	}
}

func TestDispatch_AllAutoRetryableParksRun(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	starter.failFor["claude-acp"] = fmt.Errorf("rate_limit_exceeded")
	starter.failFor["codex-acp"] = fmt.Errorf("rate_limit_exceeded")
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	launched, parked, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if launched {
		t.Fatalf("expected launched=false")
	}
	if !parked {
		t.Fatalf("expected parked=true")
	}
	updated, _ := repo.GetRun(context.Background(), run.ID)
	if updated.RoutingBlockedStatus == nil ||
		*updated.RoutingBlockedStatus != routing.StatusWaitingForCapacity {
		t.Errorf("expected waiting_for_provider_capacity; got %+v", updated.RoutingBlockedStatus)
	}
}

func TestDispatch_ExcludesProvidersFromPriorAttempts(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	// Seed a prior failed attempt for claude-acp so the dispatcher should
	// skip it via ExcludeProviders.
	if _, err := repo.IncrementRouteAttemptSeq(context.Background(), run.ID); err != nil {
		t.Fatalf("seed seq bump: %v", err)
	}
	if err := repo.AppendRouteAttempt(context.Background(), &officemodels.RouteAttempt{
		RunID:      run.ID,
		Seq:        1,
		ProviderID: "claude-acp",
		Model:      "claude-acp-bal",
		Tier:       "balanced",
		Outcome:    scheduler.RouteAttemptOutcomeFailedProviderUnavail,
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed prior attempt: %v", err)
	}
	launched, _, err := ss.DispatchWithRouting(context.Background(), run, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true via codex fallback")
	}
	if got := starter.lastCall().route.ProviderID; got != "codex-acp" {
		t.Errorf("expected codex-acp (claude excluded); got %s", got)
	}
}

func TestDispatch_AllUserActionParksBlocked(t *testing.T) {
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
	updated, _ := repo.GetRun(context.Background(), run.ID)
	if updated.RoutingBlockedStatus == nil {
		t.Fatalf("expected RoutingBlockedStatus set")
	}
	if *updated.RoutingBlockedStatus != routing.StatusBlockedActionRequired {
		t.Errorf("expected blocked_provider_action_required; got %q",
			*updated.RoutingBlockedStatus)
	}
}

// TestDispatch_CapsAttemptsAndParks pins the safety cap that prevents
// a pathological cycle (provider flapping, post-start fallback racing
// with re-park) from appending route_attempt rows forever. Seeds
// MaxAttemptsPerRun prior failed attempts and asserts the dispatcher
// stops, records a terminal skipped_max_attempts row, and parks the
// run as blocked_provider_action_required.
func TestDispatch_CapsAttemptsAndParks(t *testing.T) {
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
	if launched {
		t.Fatalf("expected launched=false at cap")
	}
	if !parked {
		t.Fatalf("expected parked=true at cap")
	}
	if starter.callCount() != 0 {
		t.Fatalf("starter must not be called at cap; got %d calls", starter.callCount())
	}

	attempts, _ := repo.ListRouteAttempts(context.Background(), run.ID)
	if len(attempts) != scheduler.MaxAttemptsPerRun+1 {
		t.Fatalf("expected %d attempts (seeded + cap row); got %d",
			scheduler.MaxAttemptsPerRun+1, len(attempts))
	}
	final := attempts[len(attempts)-1]
	if final.Outcome != scheduler.RouteAttemptOutcomeMaxAttempts {
		t.Errorf("final outcome = %q, want skipped_max_attempts", final.Outcome)
	}
	if final.ErrorCode != "max_attempts_exceeded" {
		t.Errorf("final error_code = %q, want max_attempts_exceeded", final.ErrorCode)
	}

	updated, _ := repo.GetRun(context.Background(), run.ID)
	if updated.RoutingBlockedStatus == nil {
		t.Fatalf("expected routing_blocked_status set")
	}
	if *updated.RoutingBlockedStatus != routing.StatusBlockedActionRequired {
		t.Errorf("expected blocked_provider_action_required; got %q",
			*updated.RoutingBlockedStatus)
	}
	if updated.EarliestRetryAt != nil {
		t.Errorf("expected earliest_retry_at nil (no auto-retry at cap); got %v",
			*updated.EarliestRetryAt)
	}
}

// TestDispatch_LiftedRunIgnoresPriorCycleExclusions pins the fix for
// the "park → lift → re-park" loop. After all providers failed in cycle
// 1, the run is parked. When the scheduler lifts it (auto-wake / manual
// retry), route_cycle_baseline_seq is bumped to the current attempt
// count. The next dispatch must NOT inherit the cycle-1 exclusion list
// — every provider in the order is eligible again.
func TestDispatch_LiftedRunIgnoresPriorCycleExclusions(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp", "opencode-acp"})
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)

	// Seed three prior failed attempts (all providers exhausted in cycle 1).
	for _, p := range []string{"claude-acp", "codex-acp", "opencode-acp"} {
		seq, err := repo.IncrementRouteAttemptSeq(context.Background(), run.ID)
		if err != nil {
			t.Fatalf("seed seq bump: %v", err)
		}
		if err := repo.AppendRouteAttempt(context.Background(), &officemodels.RouteAttempt{
			RunID:      run.ID,
			Seq:        seq,
			ProviderID: p,
			Model:      p + "-bal",
			Tier:       "balanced",
			Outcome:    scheduler.RouteAttemptOutcomeFailedProviderUnavail,
			StartedAt:  time.Now().UTC(),
		}); err != nil {
			t.Fatalf("seed prior attempt: %v", err)
		}
	}
	// Simulate the lift: bump baseline so the next dispatch ignores
	// every prior attempt.
	if err := repo.BumpRouteCycleBaseline(context.Background(), run.ID); err != nil {
		t.Fatalf("bump baseline: %v", err)
	}
	// Reload run to pick up baseline + seq.
	fresh, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if fresh.RouteCycleBaselineSeq != 3 {
		t.Fatalf("expected baseline=3; got %d", fresh.RouteCycleBaselineSeq)
	}

	launched, parked, err := ss.DispatchWithRouting(context.Background(),
		fresh, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true after lift; got launched=%v parked=%v",
			launched, parked)
	}
	// First eligible candidate must be claude-acp again — the baseline
	// erased the cycle-1 exclusion list.
	if got := starter.lastCall().route.ProviderID; got != "claude-acp" {
		t.Errorf("expected claude-acp (cycle 2 fresh start); got %s", got)
	}
}

// TestDispatch_PostStartFallbackKeepsExclusions pins the within-cycle
// invariant: baseline=0 means every failed attempt is part of the
// current cycle and must stay excluded. A run with one prior failed
// claude attempt should fall back to codex on the next dispatch.
func TestDispatch_PostStartFallbackKeepsExclusions(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp", "codex-acp"})
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)

	// One prior failed attempt for claude-acp, baseline left at 0 (the
	// default) — this is the post-start-fallback shape.
	if _, err := repo.IncrementRouteAttemptSeq(context.Background(), run.ID); err != nil {
		t.Fatalf("seed seq bump: %v", err)
	}
	if err := repo.AppendRouteAttempt(context.Background(), &officemodels.RouteAttempt{
		RunID:      run.ID,
		Seq:        1,
		ProviderID: "claude-acp",
		Model:      "claude-acp-bal",
		Tier:       "balanced",
		Outcome:    scheduler.RouteAttemptOutcomeFailedProviderUnavail,
		StartedAt:  time.Now().UTC(),
	}); err != nil {
		t.Fatalf("seed prior attempt: %v", err)
	}
	fresh, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if fresh.RouteCycleBaselineSeq != 0 {
		t.Fatalf("expected baseline=0; got %d", fresh.RouteCycleBaselineSeq)
	}

	launched, _, err := ss.DispatchWithRouting(context.Background(),
		fresh, makeAgent(), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched {
		t.Fatalf("expected launched=true via codex (claude still excluded)")
	}
	if got := starter.lastCall().route.ProviderID; got != "codex-acp" {
		t.Errorf("expected codex-acp (claude excluded within-cycle); got %s", got)
	}
}

func TestLiftParkedRuns_ClearsExpired(t *testing.T) {
	repo := newTestRepoSched(t)
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	past := time.Now().UTC().Add(-1 * time.Minute)
	if err := repo.ParkRunForProviderCapacity(context.Background(),
		run.ID, routing.StatusWaitingForCapacity, past); err != nil {
		t.Fatalf("park: %v", err)
	}
	lifted, err := ss.LiftParkedRuns(context.Background(), time.Now().UTC())
	if err != nil {
		t.Fatalf("lift: %v", err)
	}
	if lifted != 1 {
		t.Fatalf("expected 1 lifted; got %d", lifted)
	}
	updated, _ := repo.GetRun(context.Background(), run.ID)
	if updated.RoutingBlockedStatus != nil {
		t.Errorf("expected RoutingBlockedStatus cleared; got %v",
			*updated.RoutingBlockedStatus)
	}
}
