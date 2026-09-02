package scheduler

// Regression coverage for SchedulerService.queueCEOAgentError, the
// scheduler-package duplicate of service.queueCEOAgentError. It has no
// production caller today (see reactivity_children_completed_test.go's
// sibling comment on this package's dormant-copy surfaces), but left
// unguarded it is a trap for whichever future caller wires it up — see
// docs/specs/office/system-design/scheduler-01.md's escalation clause.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/models"
)

func createChildrenCompletedCEOAgent(t *testing.T, repo interface {
	CreateAgentInstance(ctx context.Context, agent *models.AgentInstance) error
}, id string) {
	t.Helper()
	if err := repo.CreateAgentInstance(context.Background(), &models.AgentInstance{
		ID:          id,
		WorkspaceID: "ws-1",
		Name:        id,
		Role:        models.AgentRoleCEO,
		Status:      models.AgentStatusIdle,
	}); err != nil {
		t.Fatalf("create ceo agent instance %s: %v", id, err)
	}
}

// TestSchedulerService_HandleRunFailure_CEOOwnFailure_DoesNotSelfEscalate
// proves a CEO-owned run exhausted on retries does not re-queue the same
// CEO for its own failure.
func TestSchedulerService_HandleRunFailure_CEOOwnFailure_DoesNotSelfEscalate(t *testing.T) {
	repo := newReactivityTestRepo(t)
	ss := newChildrenCompletedTestScheduler(t, repo)
	ctx := context.Background()

	createChildrenCompletedCEOAgent(t, repo, "ceo-1")

	if err := ss.QueueRun(ctx, "ceo-1", "agent_error", `{}`, ""); err != nil {
		t.Fatalf("queue ceo run: %v", err)
	}
	run, err := ss.ClaimNextRun(ctx)
	if err != nil || run == nil {
		t.Fatalf("claim ceo run: %v (run=%v)", err, run)
	}
	if run.AgentProfileID != "ceo-1" {
		t.Fatalf("claimed run belongs to %q, want ceo-1", run.AgentProfileID)
	}
	run.RetryCount = MaxRetryCount

	if err := ss.HandleRunFailure(ctx, run, errForTestScheduler("ceo boom")); err != nil {
		t.Fatalf("handle run failure: %v", err)
	}

	next, err := ss.ClaimNextRun(ctx)
	if err != nil {
		t.Fatalf("claim after failure: %v", err)
	}
	if next != nil {
		t.Fatalf("expected no self-escalation run queued, got agent=%q reason=%q",
			next.AgentProfileID, next.Reason)
	}
}

type errForTestScheduler string

func (e errForTestScheduler) Error() string { return string(e) }
