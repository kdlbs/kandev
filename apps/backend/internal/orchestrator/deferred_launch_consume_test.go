package orchestrator

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	deferredChainTaskID   = "task-chain-step"
	deferredPredecessorID = "task-predecessor"
	deferredStalePrompt   = "the brief written before any of the work happened"
)

// resolvedDependencyReader reports every dependent as unblocked. It models the
// instant a chain's last predecessor completes.
type resolvedDependencyReader struct {
	dependents []string
}

func (r *resolvedDependencyReader) DependencyGate(context.Context, string) (bool, string, error) {
	return false, "", nil
}

func (r *resolvedDependencyReader) ListDependentTaskIDs(context.Context, string) ([]string, error) {
	return r.dependents, nil
}

func (r *resolvedDependencyReader) ListPendingDependencyLaunches(context.Context) ([]taskservice.PendingDependencyLaunch, error) {
	return nil, nil
}

// launchCounter records every agent launch so a test can tell one session from
// two without polling the session table for a state it cannot force.
type launchCounter struct {
	mu    sync.Mutex
	calls int
	fired chan struct{}
}

func newLaunchCounter() *launchCounter {
	return &launchCounter{fired: make(chan struct{}, 8)}
}

func (l *launchCounter) record() {
	l.mu.Lock()
	l.calls++
	l.mu.Unlock()
	select {
	case l.fired <- struct{}{}:
	default:
	}
}

func (l *launchCounter) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.calls
}

// awaitLaunch waits for one more launch than alreadySeen, reporting whether it
// arrived. Both the positive control and the regression assertion use it, so
// neither can pass by measuring nothing.
func (l *launchCounter) awaitLaunch(alreadySeen int) bool {
	deadline := time.After(2 * time.Second)
	for {
		if l.count() > alreadySeen {
			return true
		}
		select {
		case <-l.fired:
		case <-deadline:
			return l.count() > alreadySeen
		}
	}
}

// seedChainStepTask creates a task carrying a start-when-unblocked deferred
// launch and no session — exactly what create_task_kandev with blocked_by and
// start_agent leaves behind.
func seedChainStepTask(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws1", Name: "Test", CreatedAt: now, UpdatedAt: now})
	_ = repo.CreateWorkflow(ctx, &models.Workflow{ID: "wf1", WorkspaceID: "ws1", Name: "WF", CreatedAt: now, UpdatedAt: now})

	require.NoError(t, repo.CreateTask(ctx, &models.Task{
		ID:          taskID,
		WorkspaceID: "ws1",
		WorkflowID:  "wf1",
		Title:       "Chain step",
		Description: "desc",
		State:       v1.TaskStateCreated,
		Metadata: map[string]interface{}{
			models.MetaKeyDeferredLaunch: map[string]interface{}{
				"intent":           "start",
				"agent_profile_id": "profile1",
				"prompt":           deferredStalePrompt,
				models.DeferredLaunchStartWhenUnblockedKey: true,
			},
		},
		CreatedAt: now,
		UpdatedAt: now,
	}))
}

// newDeferredLaunchTestService wires the smallest service that can carry a
// launch all the way to the agent manager.
func newDeferredLaunchTestService(
	t *testing.T, repo *sqliterepo.Repository, counter *launchCounter,
) *Service {
	t.Helper()
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[deferredChainTaskID] = &v1.Task{
		ID: deferredChainTaskID, WorkflowID: "wf1",
		Title: "Chain step", Description: "desc", State: v1.TaskStateCreated,
	}
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			counter.record()
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.SetTaskDependencyReader(&resolvedDependencyReader{dependents: []string{deferredChainTaskID}})
	return svc
}

func sessionCount(t *testing.T, repo *sqliterepo.Repository, taskID string) int {
	t.Helper()
	sessions, err := repo.ListTaskSessions(context.Background(), taskID)
	require.NoError(t, err)
	return len(sessions)
}

// awaitLaunchedSession joins the gate's detached launch goroutine on an
// observable side effect — the RUNNING state written once the agent process
// has started. Without it the test closes its database while the goroutine is
// still writing to it.
func awaitLaunchedSession(t *testing.T, repo *sqliterepo.Repository, taskID string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		sessions, err := repo.ListTaskSessions(context.Background(), taskID)
		if err == nil {
			for _, session := range sessions {
				if session.State == models.TaskSessionStateRunning {
					return
				}
			}
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("no session reached RUNNING within the deadline")
}

// The positive control. It is here so the regression assertion below cannot
// pass by observing nothing: this proves the harness DOES see a gate-driven
// launch when the intent is still intact, so "no second launch" in the other
// test means the intent was consumed, not that the plumbing was inert.
func TestDependencyResolutionLaunchesAnIntactIntent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	counter := newLaunchCounter()
	svc := newDeferredLaunchTestService(t, repo, counter)

	svc.handleTaskDependenciesForTerminalState(ctx, deferredPredecessorID, v1.TaskStateCompleted)

	require.True(t, counter.awaitLaunch(0), "an unconsumed start-when-unblocked intent must launch on resolution")
	awaitLaunchedSession(t, repo, deferredChainTaskID)
	assert.Equal(t, 1, sessionCount(t, repo, deferredChainTaskID))

	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	_, still := task.Metadata[models.MetaKeyDeferredLaunch]
	assert.False(t, still, "the gate must consume the intent it fired on")
}

// The regression. Observed twice on 2026-08-13: a task created with blocked_by
// was started manually, and hours later the dependency gate fired anyway and
// spawned a SECOND session replaying the original, long-stale prompt.
//
// Starting the task by any means consumes the intent, so the gate finds nothing
// to fire and the task keeps exactly the one session its start created.
func TestManualStartConsumesTheDeferredLaunchSoTheGateCannotRefire(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	counter := newLaunchCounter()
	svc := newDeferredLaunchTestService(t, repo, counter)

	// The manual start — the user pressing Start, or message_task_kandev
	// launching the agent with its own prompt.
	_, err := svc.StartTask(ctx, deferredChainTaskID, "profile1", "", "", "",
		"do the work with what we know now", "", false, false, nil)
	require.NoError(t, err)
	require.True(t, counter.awaitLaunch(0), "the manual start must launch")
	require.Equal(t, 1, sessionCount(t, repo, deferredChainTaskID))

	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	_, still := task.Metadata[models.MetaKeyDeferredLaunch]
	// Deliberately non-fatal: when this regresses the test must go on to
	// demonstrate the actual damage — the second session — rather than stopping
	// at the metadata that causes it.
	assert.False(t, still, "a started task must not keep a pending launch intent")

	// Hours later: the last predecessor completes and the gate opens.
	svc.handleTaskDependenciesForTerminalState(ctx, deferredPredecessorID, v1.TaskStateCompleted)

	assert.False(t, counter.awaitLaunch(1),
		"the gate must not launch a second session on a task that already started")
	assert.Equal(t, 1, sessionCount(t, repo, deferredChainTaskID),
		"the task must end with exactly the one session its manual start created")
}

// StartCreatedSession is the other agent-start chokepoint: it launches the
// agent on a session that was only prepared. message_task_kandev on a
// not-yet-started session lands here, which is how the production tasks were
// started, so it must consume the intent too.
func TestStartCreatedSessionConsumesTheDeferredLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "prepared-session", TaskID: deferredChainTaskID,
		AgentProfileID: "profile1", State: models.TaskSessionStateCreated,
		IsPrimary: true, StartedAt: now, UpdatedAt: now,
	}))
	counter := newLaunchCounter()
	svc := newDeferredLaunchTestService(t, repo, counter)

	_, err := svc.StartCreatedSession(ctx, deferredChainTaskID, "prepared-session",
		"profile1", "here is what actually matters", false, false, false, nil, nil)
	require.NoError(t, err)

	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	_, still := task.Metadata[models.MetaKeyDeferredLaunch]
	assert.False(t, still, "starting a prepared session must consume the pending launch intent")
}

// A start that FAILS leaves the intent alone: the task never ran, so the gate
// is still the thing that has to run it.
func TestFailedStartKeepsTheDeferredLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	counter := newLaunchCounter()
	svc := newDeferredLaunchTestService(t, repo, counter)

	// No agent profile anywhere: StartCreatedSession rejects before launching.
	now := time.Now().UTC()
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "prepared-session", TaskID: deferredChainTaskID,
		State: models.TaskSessionStateCreated, IsPrimary: true,
		StartedAt: now, UpdatedAt: now,
	}))

	_, err := svc.StartCreatedSession(ctx, deferredChainTaskID, "prepared-session",
		"", "prompt", false, false, false, nil, nil)
	require.Error(t, err)

	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	launch, still := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, still, "a failed start must not consume the launch intent")
	assert.Equal(t, deferredStalePrompt, launch["prompt"])
}
