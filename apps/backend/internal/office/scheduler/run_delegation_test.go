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

	if err := ss.QueueRun(ctx, testAgentID, scheduler.RunReasonTaskAssigned, `{}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	got := onlyQueuedRun(t, repo, testWorkspaceID)
	if got.WorkspaceID != testWorkspaceID {
		t.Errorf("workspace_id = %q, want %q (only the delegated causation path sets this)", got.WorkspaceID, testWorkspaceID)
	}
	if got.CausationID != got.ID {
		t.Errorf("causation_id = %q, want self-rooted %q (only the delegated causation path sets this)", got.CausationID, got.ID)
	}
	if got.PriorityClass != models.PriorityClassEvent {
		t.Errorf("priority_class = %d, want %d (PriorityClassEvent)", got.PriorityClass, models.PriorityClassEvent)
	}
}

func TestQueueRun_UsesLegacyInlinePathWhenNoRunsServiceWired(t *testing.T) {
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

	if err := ss.QueueRun(ctx, testAgentID, scheduler.RunReasonTaskAssigned, `{}`, ""); err != nil {
		t.Fatalf("queue run: %v", err)
	}

	got := onlyQueuedRun(t, repo, testWorkspaceID)
	// The legacy inline path never resolves causation, so these stay at
	// their Go zero values even though the agent has a workspace.
	if got.WorkspaceID != "" {
		t.Errorf("workspace_id = %q, want empty (legacy path does not resolve causation)", got.WorkspaceID)
	}
	if got.CausationID != "" {
		t.Errorf("causation_id = %q, want empty (legacy path does not resolve causation)", got.CausationID)
	}
	if got.PriorityClass != models.PriorityClassEvent {
		t.Errorf("priority_class = %d, want %d (PriorityClassEvent, via the PriorityClass fix)", got.PriorityClass, models.PriorityClassEvent)
	}
}
