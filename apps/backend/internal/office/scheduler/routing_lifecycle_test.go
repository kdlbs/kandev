package scheduler_test

import (
	"context"
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

// agentWithFallback builds a test agent profile with the given fallback settings.

func agentWithFallback(autoFallback bool, fallbackModel string) *settingsmodels.AgentProfile {
	agent := makeAgent()
	agent.AutoFallback = autoFallback
	agent.FallbackModel = fallbackModel
	return agent
}

const postStartFailureMessage = "missing API key: not authenticated"

// TestHandlePostStartFailure_AvailabilityFailureRequeuesViaWorkspaceRoute
// pins the office fallback owner: post-start availability failures requeue
// to the next candidate via the configured workspace route
// (routingerr.Decide(ContextOffice)), regardless of the agent profile's
// fallback settings. The session-start policy owns the model-fallback
// decision; the office router must not consult the execution profile
// (run.AgentProfileID vs execution.AgentProfileID can disagree).
func TestHandlePostStartFailure_AvailabilityFailureRequeuesViaWorkspaceRoute(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRoutedRun(t, repo)

	// A strict profile (no auto-fallback, no fallback model) must NOT gate
	// office post-start fallback: the run still requeues to the next
	// candidate, exactly as the provider-neutral ADR defines for office.
	handled, err := ss.HandlePostStartFailure(
		context.Background(), run, agentWithFallback(false, ""), postStartFailureMessage, nil)
	if err != nil {
		t.Fatalf("HandlePostStartFailure: %v", err)
	}
	if !handled {
		t.Fatal("office availability failure must be handled (requeue via workspace route)")
	}
	got, err := repo.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatalf("get run: %v", err)
	}
	if got.Status != "queued" {
		t.Errorf("run must be requeued for the next candidate, got status %q", got.Status)
	}
}

// TestHandlePostStartFailure_NonFallbackFailureEscalates verifies failures
// the classifier marks as not fallback-allowed (ambiguous runtime errors)
// keep the pre-existing escalation path: handled=false so the caller runs
// the legacy failure handling.
func TestHandlePostStartFailure_NonFallbackFailureEscalates(t *testing.T) {
	repo := newTestRepoSched(t)
	starter := newFakeTaskStarter()
	ss := buildScheduler(t, repo, starter)
	run := seedRoutedRun(t, repo)

	handled, err := ss.HandlePostStartFailure(
		context.Background(), run, agentWithFallback(true, ""), "unexpected glitch", nil)
	if err != nil {
		t.Fatalf("HandlePostStartFailure: %v", err)
	}
	if handled {
		t.Fatal("ambiguous runtime failure must not be handled by the routing path (escalate)")
	}
}
