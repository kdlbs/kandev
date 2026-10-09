package scheduler_test

// Covers the causing-run identity gap in reactivity-triggered wakes:
// QueueRunCtx/queueRunAsActor threaded ActorKind/ActorID into
// runsservice.QueueRunRequest but never CausingRunID, so every
// reactivity wake resolved as its own root cause (causation depth 0)
// regardless of a genuine live causing run behind the acting agent.

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	runsservice "github.com/kandev/kandev/internal/runs/service"

	"github.com/kandev/kandev/internal/office/scheduler"
)

func findRunByAgent(t *testing.T, repo interface {
	ListRuns(ctx context.Context, workspaceID string) ([]*models.Run, error)
}, workspaceID, agentProfileID string) *models.Run {
	t.Helper()
	reqs, err := repo.ListRuns(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	for _, r := range reqs {
		if r.AgentProfileID == agentProfileID {
			return r
		}
	}
	t.Fatalf("no run found for agent %q", agentProfileID)
	return nil
}

// TestQueueRunCtx_AgentActorInheritsCausationFromItsLiveClaimedRun pins
// the fix: a reactivity wake whose actor is an agent profile with a live
// claimed run must inherit that run's causation chain (parent id and
// depth+1), not resolve as a fresh root cause.
func TestQueueRunCtx_AgentActorInheritsCausationFromItsLiveClaimedRun(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildSchedulerForQueueRun(t, repo)
	ctx := context.Background()

	actorAgentID := "actor-agent"
	for _, id := range []string{testAgentID, actorAgentID} {
		agent := &models.AgentInstance{
			ID: id, WorkspaceID: testWorkspaceID, Name: id,
			Role: models.AgentRoleWorker, Status: models.AgentStatusIdle, MaxConcurrentSessions: 1,
		}
		if err := repo.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent %s: %v", id, err)
		}
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	ss.SetRunsService(runsSvc)

	// Seed the actor's own live, claimed run: a root cause at depth 0.
	if _, err := ss.QueueRun(ctx, actorAgentID, scheduler.RunReasonTaskAssigned, `{}`, ""); err != nil {
		t.Fatalf("queue actor's own run: %v", err)
	}
	actorRun, err := repo.RunsRepository().ClaimRun(ctx, actorAgentID)
	if err != nil {
		t.Fatalf("claim actor's run: %v", err)
	}

	// A comment from the actor agent wakes testAgentID.
	if _, err := ss.QueueRunCtx(ctx, testAgentID, scheduler.RunContext{
		Reason:    scheduler.RunReasonTaskComment,
		TaskID:    "task-1",
		ActorID:   actorAgentID,
		ActorType: "agent",
	}); err != nil {
		t.Fatalf("queue run ctx: %v", err)
	}

	woken := findRunByAgent(t, repo, testWorkspaceID, testAgentID)
	if woken.ParentRunID != actorRun.ID {
		t.Errorf("parent_run_id = %q, want the actor's live claimed run %q", woken.ParentRunID, actorRun.ID)
	}
	if woken.CausationDepth != actorRun.CausationDepth+1 {
		t.Errorf("causation_depth = %d, want %d (actor's depth + 1)", woken.CausationDepth, actorRun.CausationDepth+1)
	}
	if woken.ChainCausationID != actorRun.ChainCausationID {
		t.Errorf("causation_id = %q, want the actor's chain id %q", woken.ChainCausationID, actorRun.ChainCausationID)
	}
}

// TestQueueRunCtx_AgentActorWithNoLiveClaimedRunResolvesAsRoot pins the
// no-op half: an agent actor with no live claimed run (already finished,
// or never claimed) must resolve exactly as before this fix — its own
// root cause — rather than erroring or refusing.
func TestQueueRunCtx_AgentActorWithNoLiveClaimedRunResolvesAsRoot(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildSchedulerForQueueRun(t, repo)
	ctx := context.Background()

	actorAgentID := "actor-agent-no-claim"
	for _, id := range []string{testAgentID, actorAgentID} {
		agent := &models.AgentInstance{
			ID: id, WorkspaceID: testWorkspaceID, Name: id,
			Role: models.AgentRoleWorker, Status: models.AgentStatusIdle, MaxConcurrentSessions: 1,
		}
		if err := repo.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent %s: %v", id, err)
		}
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	ss.SetRunsService(runsSvc)

	if _, err := ss.QueueRunCtx(ctx, testAgentID, scheduler.RunContext{
		Reason:    scheduler.RunReasonTaskComment,
		TaskID:    "task-1",
		ActorID:   actorAgentID,
		ActorType: "agent",
	}); err != nil {
		t.Fatalf("queue run ctx: %v", err)
	}

	woken := findRunByAgent(t, repo, testWorkspaceID, testAgentID)
	if woken.ParentRunID != "" {
		t.Errorf("parent_run_id = %q, want empty (actor has no live claimed run)", woken.ParentRunID)
	}
	if woken.CausationDepth != 0 {
		t.Errorf("causation_depth = %d, want 0", woken.CausationDepth)
	}
	if woken.ChainCausationID != woken.ID {
		t.Errorf("causation_id = %q, want self-rooted %q", woken.ChainCausationID, woken.ID)
	}
}

// TestQueueRunCtx_AmbiguousMultiClaimActorInheritsDeepestCausationDepth pins
// the fix for the ambiguous-multi-claim causation-attribution gap: an actor
// agent holding more than one live claimed run at once (max_concurrent_sessions
// above 1) must not let the wake resolve as a fresh root cause just because
// the causing claim can't be identified — that would let an agent already
// deep in a causation chain launder a self-triggered wake back past the
// depth ceiling. The wake instead inherits the deepest of the actor's
// ambiguous claims, so the computed depth is never lower than the true
// causing claim's depth would have produced, whichever claim it really was.
func TestQueueRunCtx_AmbiguousMultiClaimActorInheritsDeepestCausationDepth(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildSchedulerForQueueRun(t, repo)
	ctx := context.Background()

	actorAgentID := "actor-agent-ambiguous"
	for _, id := range []string{testAgentID, actorAgentID} {
		agent := &models.AgentInstance{
			ID: id, WorkspaceID: testWorkspaceID, Name: id,
			Role: models.AgentRoleWorker, Status: models.AgentStatusIdle, MaxConcurrentSessions: 2,
		}
		if err := repo.CreateAgentInstance(ctx, agent); err != nil {
			t.Fatalf("create agent %s: %v", id, err)
		}
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	ss.SetRunsService(runsSvc)

	shallow := &models.Run{
		ID: "shallow-claim", AgentProfileID: actorAgentID, Reason: "task_assigned",
		Payload: `{}`, CausationDepth: 1,
	}
	if err := repo.RunsRepository().CreateRun(ctx, shallow); err != nil {
		t.Fatalf("create shallow claim: %v", err)
	}
	if _, err := repo.RunsRepository().ClaimRun(ctx, actorAgentID); err != nil {
		t.Fatalf("claim shallow: %v", err)
	}
	deep := &models.Run{
		ID: "deep-claim", AgentProfileID: actorAgentID, Reason: "task_assigned",
		Payload: `{}`, CausationDepth: 4,
	}
	if err := repo.RunsRepository().CreateRun(ctx, deep); err != nil {
		t.Fatalf("create deep claim: %v", err)
	}
	if _, err := repo.RunsRepository().ClaimRun(ctx, actorAgentID); err != nil {
		t.Fatalf("claim deep: %v", err)
	}

	if _, err := ss.QueueRunCtx(ctx, testAgentID, scheduler.RunContext{
		Reason:    scheduler.RunReasonTaskComment,
		TaskID:    "task-1",
		ActorID:   actorAgentID,
		ActorType: "agent",
	}); err != nil {
		t.Fatalf("queue run ctx: %v", err)
	}

	woken := findRunByAgent(t, repo, testWorkspaceID, testAgentID)
	if woken.ParentRunID != deep.ID {
		t.Errorf("parent_run_id = %q, want the deepest ambiguous claim %q", woken.ParentRunID, deep.ID)
	}
	if woken.CausationDepth != deep.CausationDepth+1 {
		t.Errorf("causation_depth = %d, want %d (deepest claim's depth + 1)", woken.CausationDepth, deep.CausationDepth+1)
	}
}

// TestQueueRunCtx_CausingRunLookupErrorPropagatesRatherThanSilentlyRooting
// pins that a genuine repository error resolving the actor's live claimed
// run (anything other than "no such row") fails the queue attempt instead
// of silently treating it like "no live claimed run" and minting the new
// run as a fresh causation root — which would let a transient lookup
// failure evade the causation-depth ceiling it exists to enforce.
func TestQueueRunCtx_CausingRunLookupErrorPropagatesRatherThanSilentlyRooting(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildSchedulerForQueueRun(t, repo)
	ctx := context.Background()

	agent := &models.AgentInstance{
		ID: testAgentID, WorkspaceID: testWorkspaceID, Name: testAgentID,
		Role: models.AgentRoleWorker, Status: models.AgentStatusIdle, MaxConcurrentSessions: 1,
	}
	if err := repo.CreateAgentInstance(ctx, agent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	ss.SetRunsService(runsSvc)

	// Drop the runs table so GetClaimedRunForCausationAttribution's SELECT
	// fails with a real error rather than sql.ErrNoRows.
	if _, err := repo.RunsRepository().Writer().Exec(`DROP TABLE runs`); err != nil {
		t.Fatalf("drop runs table: %v", err)
	}

	if _, err := ss.QueueRunCtx(ctx, testAgentID, scheduler.RunContext{
		Reason:    scheduler.RunReasonTaskComment,
		TaskID:    "task-1",
		ActorID:   "actor-agent",
		ActorType: "agent",
	}); err == nil {
		t.Fatal("expected queue run ctx to fail when the causing-run lookup errors, not silently root the new run")
	}
}
