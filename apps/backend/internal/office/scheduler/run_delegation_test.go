package scheduler_test

// Covers AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6: once SetRunsService wires a
// shared runs/service.Service, SchedulerService.QueueRun/QueueRunCtx must
// delegate to it instead of always doing their own ss.repo.CreateRun. The
// delegated path resolves causation (workspace id, self-rooted causation
// id) that the legacy inline path never sets — those fields are the
// observable proof that delegation actually happened, not just that a run
// landed in the table.
//
// models.AgentInstance persists into the agent_profiles table (see
// insertAgentInstance in repository/sqlite/agents.go), so
// repo.CreateAgentInstance alone is enough to satisfy
// runs/service.resolveCausation's agent_profiles workspace lookup — no
// separate agent_profiles seeding helper is needed here.

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	officesqlite "github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/office/scheduler"
	"github.com/kandev/kandev/internal/office/service"
	runsservice "github.com/kandev/kandev/internal/runs/service"
)

// buildSchedulerForQueueRun builds a SchedulerService with a real
// office/service.Service bound to the same repo, so QueueRun's
// guardAgentStatus (ss.svc.GetAgentFromConfig) has something to read.
// buildScheduler (dispatch_routing_test.go) always passes a nil svc because
// none of its callers exercise QueueRun.
func buildSchedulerForQueueRun(t *testing.T, repo *officesqlite.Repository) *scheduler.SchedulerService {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	svc := service.NewService(service.ServiceOptions{Repo: repo, Logger: log})
	return scheduler.NewSchedulerService(repo, log, svc)
}

// onlyQueuedRun fetches the single run queued for workspaceID via the
// office repository's ListRuns (joins through agent_profiles.workspace_id,
// so it works whether or not the run row's own workspace_id column was
// stamped — exactly the thing these tests are distinguishing).
func onlyQueuedRun(t *testing.T, repo interface {
	ListRuns(ctx context.Context, workspaceID string) ([]*models.Run, error)
}, workspaceID string) *models.Run {
	t.Helper()
	reqs, err := repo.ListRuns(context.Background(), workspaceID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("want 1 run, got %d", len(reqs))
	}
	return reqs[0]
}

func TestQueueRun_DelegatesToRunsServiceWhenWired(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildSchedulerForQueueRun(t, repo)
	ctx := context.Background()

	settingsAgent := &models.AgentInstance{
		ID:                    testAgentID,
		WorkspaceID:           testWorkspaceID,
		Name:                  "delegation-agent",
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: 1,
	}
	if err := repo.CreateAgentInstance(ctx, settingsAgent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	ss.SetRunsService(runsSvc)

	if _, err := ss.QueueRun(ctx, testAgentID, scheduler.RunReasonTaskAssigned, `{}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	got := onlyQueuedRun(t, repo, testWorkspaceID)
	if got.WorkspaceID != testWorkspaceID {
		t.Errorf("workspace_id = %q, want %q (only the delegated causation path sets this)", got.WorkspaceID, testWorkspaceID)
	}
	if got.ChainCausationID != got.ID {
		t.Errorf("causation_id = %q, want self-rooted %q (only the delegated causation path sets this)", got.ChainCausationID, got.ID)
	}
	if got.PriorityClass != models.PriorityClassEvent {
		t.Errorf("priority_class = %d, want %d (PriorityClassEvent)", got.PriorityClass, models.PriorityClassEvent)
	}
}

// TestQueueRun_FailsClosedWhenNoRunsServiceWired covers
// AC-OFFICE-ENQUEUE-CONSOLIDATION-001.6: a delegating caller without the
// authoritative runs/service API available must fail its enqueue and
// surface the error, not fall back to an insert of its own — a fallback
// insert would bypass causation resolution and the causation-depth/
// self-trigger refusal gates entirely, which is exactly the ungated path
// this requirement removes. No run is queued as a result.
func TestQueueRun_FailsClosedWhenNoRunsServiceWired(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildSchedulerForQueueRun(t, repo)
	ctx := context.Background()

	settingsAgent := &models.AgentInstance{
		ID:                    testAgentID,
		WorkspaceID:           testWorkspaceID,
		Name:                  "no-delegation-agent",
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: 1,
	}
	if err := repo.CreateAgentInstance(ctx, settingsAgent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	if _, err := ss.QueueRun(ctx, testAgentID, scheduler.RunReasonTaskAssigned, `{}`, ""); err == nil {
		t.Fatal("expected QueueRun to fail closed with no runs service wired, got nil error")
	}

	reqs, err := repo.ListRuns(ctx, testWorkspaceID)
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(reqs) != 0 {
		t.Fatalf("want 0 runs queued, got %d (a failed enqueue must not leave a row behind)", len(reqs))
	}
}

// TestQueueRunCtx_DelegatesRealActorFromRunContext covers
// AC-OFFICE-RUN-CAUSATION-001.15: QueueRunCtx call sites (approval
// resolution, reactivity) already know the human or agent that caused the
// wake via RunContext.ActorType/ActorID, but that identity was previously
// discarded before it ever reached causation resolution — every wake was
// silently attributed to the system actor. This proves it now survives:
// the persisted row's actor fields, and (since ClassifyPriority treats a
// user actor specially) its priority class.
func TestQueueRunCtx_DelegatesRealActorFromRunContext(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildSchedulerForQueueRun(t, repo)
	ctx := context.Background()

	settingsAgent := &models.AgentInstance{
		ID:                    testAgentID,
		WorkspaceID:           testWorkspaceID,
		Name:                  "actor-agent",
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: 1,
	}
	if err := repo.CreateAgentInstance(ctx, settingsAgent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	ss.SetRunsService(runsSvc)

	_, err = ss.QueueRunCtx(ctx, testAgentID, scheduler.RunContext{
		Reason:    scheduler.RunReasonTaskComment,
		TaskID:    "task-1",
		ActorID:   "user-42",
		ActorType: "user",
	})
	if err != nil {
		t.Fatalf("queue run ctx: %v", err)
	}

	got := onlyQueuedRun(t, repo, testWorkspaceID)
	if got.ActorKind != models.ActorKindUser {
		t.Errorf("actor_kind = %q, want %q", got.ActorKind, models.ActorKindUser)
	}
	if got.ActorID != "user-42" {
		t.Errorf("actor_id = %q, want %q", got.ActorID, "user-42")
	}
	if got.PriorityClass != models.PriorityClassHuman {
		t.Errorf("priority_class = %d, want %d (PriorityClassHuman, since the actor is now a real user)", got.PriorityClass, models.PriorityClassHuman)
	}
}

// TestQueueRunCtx_AgentSelfTriggerAllowanceEnforced proves the
// AC-OFFICE-LAUNCH-SAFETY-004 self-trigger gate — dormant on this path
// before actor threading, since the gate only evaluates for
// actorKind == ActorKindAgent && actorID == the woken agent's own id —
// is now reachable through QueueRunCtx: an agent re-triggering itself
// beyond the default allowance (3, within the rolling window) is refused.
func TestQueueRunCtx_AgentSelfTriggerAllowanceEnforced(t *testing.T) {
	repo := newTestRepoSched(t)
	ss := buildSchedulerForQueueRun(t, repo)
	ctx := context.Background()

	settingsAgent := &models.AgentInstance{
		ID:                    testAgentID,
		WorkspaceID:           testWorkspaceID,
		Name:                  "self-trigger-agent",
		Role:                  models.AgentRoleWorker,
		Status:                models.AgentStatusIdle,
		MaxConcurrentSessions: 1,
	}
	if err := repo.CreateAgentInstance(ctx, settingsAgent); err != nil {
		t.Fatalf("create agent: %v", err)
	}

	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	runsSvc := runsservice.New(repo.RunsRepository(), nil, log, nil)
	ss.SetRunsService(runsSvc)

	queueSelfTriggered := func(taskID string) error {
		_, err := ss.QueueRunCtx(ctx, testAgentID, scheduler.RunContext{
			Reason:    scheduler.RunReasonTaskComment,
			TaskID:    taskID,
			ActorID:   testAgentID,
			ActorType: "agent",
		})
		return err
	}

	// runsservice.DefaultSelfTriggerAllowance is 3 (this scheduler wires a
	// runsSvc with no SetLaunchSafetyLimits override, so the default
	// applies): the first 3 distinct-task self-triggers must succeed.
	for i := 0; i < 3; i++ {
		if err := queueSelfTriggered(fmt.Sprintf("task-%d", i)); err != nil {
			t.Fatalf("self-trigger %d: unexpected refusal: %v", i, err)
		}
	}
	// The 4th within the rolling window must be refused.
	if err := queueSelfTriggered("task-3"); err == nil {
		t.Fatal("expected the 4th self-trigger within the allowance window to be refused, got nil error")
	}
}
