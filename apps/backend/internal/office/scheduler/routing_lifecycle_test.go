package scheduler_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/scheduler"
)

// seedRoutedRun sets up a run that was launched via routing: a resolved
// route (provider claude-acp, model claude-sonnet) and an in-flight attempt
// row at seq 1 so HandlePostStartFailure can find the candidate. The
// returned run struct carries the routing columns in memory, mirroring how
// the scheduler receives a freshly-loaded run row.
func seedRoutedRun(t *testing.T, repo *officesqlite.Repository) *officemodels.Run {
	t.Helper()
	seedRoutingConfig(t, repo, []routing.ProviderID{"claude-acp"})
	run := seedRun(t, repo, `{"task_id":"t-1"}`)
	seq, err := repo.IncrementRouteAttemptSeq(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("increment seq: %v", err)
	}
	if err := repo.SetRunResolvedRoute(context.Background(), run.ID, "claude-profile", "claude-acp", "claude-sonnet"); err != nil {
		t.Fatalf("set resolved route: %v", err)
	}
	if err := repo.AppendRouteAttempt(context.Background(), &officemodels.RouteAttempt{
		RunID:              run.ID,
		Seq:                1,
		ExecutionProfileID: "claude-profile",
		ProviderID:         "claude-acp",
		Model:              "claude-sonnet",
		Tier:               "balanced",
		Outcome:            scheduler.RouteAttemptOutcomeLaunched,
		StartedAt:          time.Now().UTC(),
	}); err != nil {
		t.Fatalf("append attempt: %v", err)
	}
	run.CurrentRouteAttemptSeq = seq
	run.ResolvedExecutionProfileID = new("claude-profile")
	run.ResolvedProviderID = new("claude-acp")
	run.ResolvedModel = new("claude-sonnet")
	run.RequestedTier = new("balanced")
	return run
}

func agentWithFallback(autoFallback bool, fallbackModel string) *settingsmodels.AgentProfile {
	agent := makeAgent()
	agent.AutoFallback = autoFallback
	agent.FallbackModel = fallbackModel
	return agent
}

const postStartFailureMessage = "missing API key: not authenticated"

func TestHandlePostStartFailure_StrictModeEscalates(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRoutedRun(t, repo)

	handled, err := ss.HandlePostStartFailure(
		context.Background(), run, agentWithFallback(false, ""), postStartFailureMessage)
	if err != nil {
		t.Fatalf("HandlePostStartFailure: %v", err)
	}
	if handled {
		t.Fatal("strict mode must NOT handle the failure (escalate to terminal failure)")
	}
	// The run must not have been requeued.
	got, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "claimed" {
		t.Errorf("strict mode must leave the run claimed, got status %q", got.Status)
	}
	if got.FallbackModelOverride != nil {
		t.Errorf("strict mode must not set a fallback override, got %q", *got.FallbackModelOverride)
	}
}

func TestHandlePostStartFailure_AutoFallbackRequeues(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRoutedRun(t, repo)

	handled, err := ss.HandlePostStartFailure(
		context.Background(), run, agentWithFallback(true, ""), postStartFailureMessage)
	if err != nil {
		t.Fatalf("HandlePostStartFailure: %v", err)
	}
	if !handled {
		t.Fatal("auto-fallback mode must handle the failure (requeue to next candidate)")
	}
	got, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "queued" {
		t.Errorf("auto-fallback mode must requeue the run, got status %q", got.Status)
	}
	if got.FallbackModelOverride != nil {
		t.Errorf("auto-fallback mode must not set a fallback override, got %q", *got.FallbackModelOverride)
	}
}

func TestHandlePostStartFailure_FallbackModelSetsOverride(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRoutedRun(t, repo)

	handled, err := ss.HandlePostStartFailure(
		context.Background(), run, agentWithFallback(false, "gpt-5"), postStartFailureMessage)
	if err != nil {
		t.Fatalf("HandlePostStartFailure: %v", err)
	}
	if !handled {
		t.Fatal("fallback-model mode must handle the failure (one-shot retry)")
	}
	got, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "queued" {
		t.Errorf("fallback-model mode must requeue the run, got status %q", got.Status)
	}
	if got.FallbackModelOverride == nil || *got.FallbackModelOverride != "gpt-5" {
		t.Errorf("fallback-model mode must set override gpt-5, got %v", got.FallbackModelOverride)
	}
}

func TestDispatch_ForcedFallbackLaunchesOnlyFallbackModel(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRoutedRun(t, repo)
	if err := repo.SetRunFallbackModelOverride(context.Background(), run.ID, "gpt-5"); err != nil {
		t.Fatalf("set override: %v", err)
	}
	run.FallbackModelOverride = new("gpt-5")

	launched, parked, err := ss.DispatchWithRouting(
		context.Background(), run, agentWithFallback(false, "gpt-5"), scheduler.LaunchContext{})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if !launched || parked {
		t.Fatalf("expected launched=true parked=false, got launched=%v parked=%v", launched, parked)
	}
	if starter.callCount() != 1 {
		t.Fatalf("expected exactly 1 launch, got %d", starter.callCount())
	}
	call := starter.lastCall()
	if call.route.ProviderID != "claude-acp" {
		t.Errorf("forced candidate must reuse the resolved provider, got %q", call.route.ProviderID)
	}
	if call.route.Model != "gpt-5" {
		t.Errorf("forced candidate must use the fallback model, got %q", call.route.Model)
	}
	// The override is consumed.
	got, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.FallbackModelOverride != nil {
		t.Errorf("override must be cleared after one attempt, got %q", *got.FallbackModelOverride)
	}
	if got.ResolvedModel == nil || *got.ResolvedModel != "gpt-5" {
		t.Errorf("resolved model must record the fallback, got %v", got.ResolvedModel)
	}
}

func TestDispatch_ForcedFallbackFailureIsTerminal(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	starter.failFor["claude-acp"] = fmt.Errorf("unknown model claude-sonnet")
	ss := buildScheduler(t, repo, starter)
	run := seedRoutedRun(t, repo)
	if err := repo.SetRunFallbackModelOverride(context.Background(), run.ID, "gpt-5"); err != nil {
		t.Fatalf("set override: %v", err)
	}
	run.FallbackModelOverride = new("gpt-5")

	launched, parked, err := ss.DispatchWithRouting(
		context.Background(), run, agentWithFallback(false, "gpt-5"), scheduler.LaunchContext{})
	if err == nil {
		t.Fatal("forced fallback failure must return an error (terminal)")
	}
	if launched || parked {
		t.Fatalf("expected launched=false parked=false, got launched=%v parked=%v", launched, parked)
	}
	if !strings.Contains(err.Error(), "gpt-5") {
		t.Errorf("error must name the fallback model, got %q", err.Error())
	}
	// No further retries: the override was consumed.
	got, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.FallbackModelOverride != nil {
		t.Errorf("override must be cleared after the one attempt, got %q", *got.FallbackModelOverride)
	}
}

// TestHandlePostStartFailure_NoSecondFallbackAttempt covers the guard that
// prevents a second post-start fallback: after the forced fallback launches
// successfully, a later streaming failure on that fallback model must
// escalate to the terminal failure path instead of re-dispatching the same
// fallback model (the one-shot override was already consumed).
func TestHandlePostStartFailure_NoSecondFallbackAttempt(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRoutedRun(t, repo)
	if err := repo.SetRunFallbackModelOverride(context.Background(), run.ID, "gpt-5"); err != nil {
		t.Fatalf("set override: %v", err)
	}
	run.FallbackModelOverride = new("gpt-5")

	launched, parked, err := ss.DispatchWithRouting(
		context.Background(), run, agentWithFallback(false, "gpt-5"), scheduler.LaunchContext{})
	if err != nil || !launched || parked {
		t.Fatalf("forced fallback dispatch: launched=%v parked=%v err=%v", launched, parked, err)
	}

	// The fallback attempt (seq 2, model gpt-5) is now in flight. A streaming
	// failure on that model must escalate — no new override, no requeue.
	afterDispatch, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run after dispatch: %v", err)
	}
	handled, err := ss.HandlePostStartFailure(
		context.Background(), afterDispatch, agentWithFallback(false, "gpt-5"), postStartFailureMessage)
	if err != nil {
		t.Fatalf("HandlePostStartFailure: %v", err)
	}
	if handled {
		t.Fatal("failure on the fallback model itself must NOT be handled again (escalate)")
	}
	got, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "claimed" {
		t.Errorf("must leave the run claimed (no requeue), got status %q", got.Status)
	}
	if got.FallbackModelOverride != nil {
		t.Errorf("must not set a new fallback override, got %q", *got.FallbackModelOverride)
	}
}
