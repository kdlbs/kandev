package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	taskservice "github.com/kandev/kandev/internal/task/service"
	usermodels "github.com/kandev/kandev/internal/user/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
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

type deferredRecentUseCall struct {
	profileID string
	userID    string
}

type deferredRecentUseRecorder struct {
	calls chan deferredRecentUseCall
}

func (r *deferredRecentUseRecorder) RecordAgentProfileRecentUse(
	ctx context.Context,
	_ usermodels.AgentProfileRecentUseContext,
	profileID string,
) (*usermodels.AgentProfileRecentUse, error) {
	identity, _ := authn.IdentityFromContext(ctx)
	r.calls <- deferredRecentUseCall{profileID: profileID, userID: identity.UserID}
	return nil, nil
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

func TestDeferredTaskCreateLaunchRecordsThePersistedCreatorAfterSuccess(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	launch := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	launch[models.DeferredLaunchUserIDKey] = "creator-1"
	launch[models.DeferredLaunchRecordRecentUseKey] = true
	require.NoError(t, repo.UpdateTask(ctx, task))

	recorder := &deferredRecentUseRecorder{calls: make(chan deferredRecentUseCall, 1)}
	svc := newDeferredLaunchTestService(t, repo, newLaunchCounter())
	svc.SetAgentProfileRecentUseRecorder(recorder)

	require.True(t, svc.launchDeferredTask(ctx, task, "test.deferred_launch", false))
	select {
	case call := <-recorder.calls:
		assert.Equal(t, "profile1", call.profileID)
		assert.Equal(t, "creator-1", call.userID)
	case <-time.After(2 * time.Second):
		t.Fatal("successful deferred task-create launch did not record profile recency")
	}
	awaitLaunchedSession(t, repo, deferredChainTaskID)
}

// A deferred launch without the selector attribution marker models an MCP
// create: the agent profile is operational launch input, not a UI selection.
// It must not reorder task_create history even if an old or forged record also
// contains a user ID.
func TestDeferredMCPLaunchDoesNotRecordRecencyWithoutSelectorAttribution(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	launch := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	launch[models.DeferredLaunchUserIDKey] = "mcp-user"
	require.NoError(t, repo.UpdateTask(ctx, task))

	recorder := &deferredRecentUseRecorder{calls: make(chan deferredRecentUseCall, 1)}
	svc := newDeferredLaunchTestService(t, repo, newLaunchCounter())
	svc.SetAgentProfileRecentUseRecorder(recorder)

	require.True(t, svc.launchDeferredTask(ctx, task, "test.deferred_mcp_launch", false))
	awaitLaunchedSession(t, repo, deferredChainTaskID)
	select {
	case call := <-recorder.calls:
		t.Fatalf("MCP deferred launch recorded profile %q for user %q", call.profileID, call.userID)
	case <-time.After(100 * time.Millisecond):
	}
}

func TestFailedDeferredTaskCreateLaunchDoesNotRecordRecency(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	launch := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	launch[models.DeferredLaunchUserIDKey] = "creator-1"
	launch[models.DeferredLaunchRecordRecentUseKey] = true
	require.NoError(t, repo.UpdateTask(ctx, task))

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[deferredChainTaskID] = &v1.Task{
		ID: deferredChainTaskID, WorkflowID: "wf1", Title: "Chain step", Description: "desc", State: v1.TaskStateCreated,
	}
	launchReturned := make(chan struct{})
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			close(launchReturned)
			return nil, errors.New("executor refused the deferred launch")
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	recorder := &deferredRecentUseRecorder{calls: make(chan deferredRecentUseCall, 1)}
	svc.SetAgentProfileRecentUseRecorder(recorder)

	require.True(t, svc.launchDeferredTask(ctx, task, "test.deferred_launch", false))
	select {
	case <-launchReturned:
	case <-time.After(2 * time.Second):
		t.Fatal("deferred launch did not reach the failing agent boundary")
	}
	select {
	case call := <-recorder.calls:
		t.Fatalf("failed deferred launch recorded profile %q", call.profileID)
	case <-time.After(100 * time.Millisecond):
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

// The interleaving the ordering-based tests cannot see: the gate opens WHILE a
// direct start is mid-launch.
//
// Consuming the intent after a successful launch left that whole window open,
// and a launch is slow — worktree creation, a container health check. A gate
// firing inside it loads the task, still sees the intent, claims it, and starts
// its own session alongside the one being launched. Same double session, reached
// by interleaving rather than by ordering, so
// TestManualStartConsumesTheDeferredLaunchSoTheGateCannotRefire passes right
// through it.
//
// Deterministic, not timing-based: the gate is fired from inside the agent
// manager's launch callback, which is exactly the middle of the launch.
func TestGateFiringMidLaunchCannotStartASecondSession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	counter := newLaunchCounter()

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[deferredChainTaskID] = &v1.Task{
		ID: deferredChainTaskID, WorkflowID: "wf1",
		Title: "Chain step", Description: "desc", State: v1.TaskStateCreated,
	}
	var svc *Service
	gateFired := make(chan struct{})
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			counter.record()
			// Mid-launch: the last predecessor completes and the gate opens.
			// Fired once — the gate's own launch would re-enter this callback.
			select {
			case <-gateFired:
			default:
				close(gateFired)
				svc.handleTaskDependenciesForTerminalState(ctx, deferredPredecessorID, v1.TaskStateCompleted)
			}
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-1"}, nil
		},
	}
	svc = createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)
	svc.SetTaskDependencyReader(&resolvedDependencyReader{dependents: []string{deferredChainTaskID}})

	_, err := svc.StartTask(ctx, deferredChainTaskID, "profile1", "", "", "",
		"do the work with what we know now", "", false, false, nil)
	require.NoError(t, err)
	<-gateFired

	assert.False(t, counter.awaitLaunch(1),
		"a gate opening mid-launch must not launch a second session")
	assert.Equal(t, 1, sessionCount(t, repo, deferredChainTaskID),
		"the task must end with exactly one session")
}

// A start that fails must PUT THE INTENT BACK, now that the claim is taken
// before the launch rather than after it. Without the release the task silently
// never runs, which is a worse failure than the extra session the claim
// prevents.
func TestFailedLaunchReleasesTheClaimedIntent(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)

	taskRepo := newMockTaskRepo()
	taskRepo.tasks[deferredChainTaskID] = &v1.Task{
		ID: deferredChainTaskID, WorkflowID: "wf1",
		Title: "Chain step", Description: "desc", State: v1.TaskStateCreated,
	}
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: repo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			return nil, errors.New("executor refused the launch")
		},
	}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, agentMgr)

	_, err := svc.StartTask(ctx, deferredChainTaskID, "profile1", "", "", "",
		"do the work", "", false, false, nil)
	require.Error(t, err)

	task, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	launch, still := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	require.True(t, still, "a failed launch must put the reserved intent back")
	assert.Equal(t, deferredStalePrompt, launch["prompt"], "the restored intent must be the one that was claimed")
	assert.Equal(t, "profile1", launch["agent_profile_id"])
	if flag, _ := launch[models.DeferredLaunchStartWhenUnblockedKey].(bool); !flag {
		t.Fatal("the restored intent must keep its start-when-unblocked flag or the gate stops recognising it")
	}
}

func TestQueuePromotionOfBlockedWIPOverflowDoesNotLaunch(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedSession(t, repo, "wip-occupant", "wip-occupant-session", "step-limited")
	seedChainStepTask(t, repo, deferredChainTaskID)
	createdTask, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	createdLaunch := createdTask.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	createdLaunch[models.DeferredLaunchUserIDKey] = "creator-1"
	createdLaunch[models.DeferredLaunchRecordRecentUseKey] = true
	require.NoError(t, repo.UpdateTask(ctx, createdTask))

	counter := newLaunchCounter()
	steps := newMockStepGetter()
	steps.steps["step-source"] = &wfmodels.WorkflowStep{
		ID: "step-source", WorkflowID: "wf1", Name: "Source", Position: 0,
	}
	steps.steps["step-limited"] = &wfmodels.WorkflowStep{
		ID: "step-limited", WorkflowID: "wf1", Name: "Limited", Position: 1, WIPLimit: 1,
	}
	steps.steps["step-next"] = &wfmodels.WorkflowStep{
		ID: "step-next", WorkflowID: "wf1", Name: "Next", Position: 2,
	}

	svc := newDeferredLaunchTestService(t, repo, counter)
	svc.SetWorkflowStepGetter(steps)
	recorder := &deferredRecentUseRecorder{calls: make(chan deferredRecentUseCall, 1)}
	svc.SetAgentProfileRecentUseRecorder(recorder)
	// Model an unresolved blocked_by edge. Promotion is deliberately allowed;
	// only the deferred agent launch must remain gated.
	dependencies := &launchDependencyReader{blocked: true}
	svc.SetTaskDependencyReader(dependencies)
	store := newWorkflowStore(repo, steps, svc.agentManager, noopPublisher, testLogger(),
		func(eventCtx context.Context, task *models.Task) {
			svc.handleTaskQueuePromoted(eventCtx, watcher.TaskEventData{TaskID: task.ID})
		})

	// First overflow the limited step while its only WIP slot is occupied.
	require.NoError(t, store.ApplyTransition(
		ctx, deferredChainTaskID, "", "step-source", "step-limited", "manual_move",
	))
	queued, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	require.False(t, queued.WIPAdmitted)
	require.Equal(t, "step-limited", queued.QueuedForStepID)

	// Vacating the slot promotes the same-step queue entry and synchronously
	// delivers task.queue_promoted to the handler under test.
	require.NoError(t, store.ApplyTransition(
		ctx, "wip-occupant", "wip-occupant-session", "step-limited", "step-next", "on_turn_complete",
	))
	promoted, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	require.True(t, promoted.WIPAdmitted)
	require.Empty(t, promoted.QueuedForStepID)
	_, promotionPending := promoted.Metadata[models.MetaKeyQueuePromotionPending]
	require.True(t, promotionPending, "blocked promotion must retain its retry token")
	_, launchPending := promoted.Metadata[models.MetaKeyDeferredLaunch]
	require.True(t, launchPending, "blocked promotion must retain its deferred launch intent")

	select {
	case <-counter.fired:
		t.Fatal("dependency-blocked WIP promotion launched an agent")
	case <-time.After(100 * time.Millisecond):
	}
	require.Equal(t, 0, sessionCount(t, repo, deferredChainTaskID))

	// Dependency resolution enters the normal auto-start chokepoint. It must
	// resume the still-pending promotion lifecycle rather than leave the token
	// stranded after launching directly.
	dependencies.blocked = false
	svc.autoStartTaskForStep(ctx, deferredChainTaskID, "step-limited", "task.dependencies_resolved", 0)
	require.True(t, counter.awaitLaunch(0), "resolved dependency did not launch the promoted task")
	awaitLaunchedSession(t, repo, deferredChainTaskID)
	started, err := repo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	_, promotionPending = started.Metadata[models.MetaKeyQueuePromotionPending]
	require.False(t, promotionPending, "resolved promotion left its lifecycle token behind")
	select {
	case call := <-recorder.calls:
		assert.Equal(t, "profile1", call.profileID)
		assert.Equal(t, "creator-1", call.userID)
	case <-time.After(2 * time.Second):
		t.Fatal("WIP-promoted task-create launch did not record profile recency")
	}
}

func TestFailedPromotedDeferredLaunchRestoresTokenAndRetries(t *testing.T) {
	ctx := context.Background()
	baseRepo := setupTestRepo(t)
	seedChainStepTask(t, baseRepo, deferredChainTaskID)
	require.NoError(t, baseRepo.SetTaskMetadataKey(
		ctx, deferredChainTaskID, models.MetaKeyQueuePromotionPending, true,
	))
	task, err := baseRepo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	launch := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	launch[models.DeferredLaunchUserIDKey] = "creator-1"
	launch[models.DeferredLaunchRecordRecentUseKey] = true
	task.WorkflowStepID = "step-draft"
	task.WIPAdmitted = true
	require.NoError(t, baseRepo.UpdateTask(ctx, task))

	steps := newMockStepGetter()
	steps.steps["step-draft"] = &wfmodels.WorkflowStep{
		ID: "step-draft", WorkflowID: "wf1", Name: "Draft", Position: 0,
	}
	steps.steps["step-review"] = &wfmodels.WorkflowStep{
		ID: "step-review", WorkflowID: "wf1", Name: "Review", Position: 1,
	}
	// Fail the scheduler's task projection lookup on the first attempt. This
	// injects a transient LaunchSession failure before any session or workspace
	// is created, making a successful retry safe and deterministic.
	taskRepo := newMockTaskRepo()
	taskRepo.getTaskErr = errors.New("transient scheduler projection failure")
	var agentLaunches atomic.Int32
	launchAttempted := make(chan struct{}, 1)
	agentMgr := &mockAgentManager{
		repoForExecutionLookup: baseRepo,
		launchAgentFunc: func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
			agentLaunches.Add(1)
			launchAttempted <- struct{}{}
			return &executor.LaunchAgentResponse{AgentExecutionID: "exec-retry"}, nil
		},
	}
	svc := createTestServiceWithScheduler(baseRepo, steps, taskRepo, agentMgr)
	svc.SetTaskDependencyReader(&resolvedDependencyReader{})
	recorder := &deferredRecentUseRecorder{calls: make(chan deferredRecentUseCall, 1)}
	svc.SetAgentProfileRecentUseRecorder(recorder)

	svc.handleTaskQueuePromoted(ctx, watcher.TaskEventData{TaskID: deferredChainTaskID})

	// The failure is handled in the detached launch goroutine. Wait until both
	// durable retry inputs have been restored before running startup recovery.
	deadline := time.Now().Add(2 * time.Second)
	for {
		stored, loadErr := baseRepo.GetTask(ctx, deferredChainTaskID)
		require.NoError(t, loadErr)
		_, deferredRestored := stored.Metadata[models.MetaKeyDeferredLaunch]
		_, promotionRestored := stored.Metadata[models.MetaKeyQueuePromotionPending]
		if deferredRestored && promotionRestored {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("launch retry metadata was not restored: %#v", stored.Metadata)
		}
		time.Sleep(5 * time.Millisecond)
	}
	require.Equal(t, 0, sessionCount(t, baseRepo, deferredChainTaskID))
	select {
	case call := <-recorder.calls:
		t.Fatalf("failed promoted launch recorded profile %q", call.profileID)
	case <-time.After(100 * time.Millisecond):
	}

	// Repair the transient scheduler projection failure, then exercise the same
	// startup sweep that retries durable queue-promotion lifecycle tokens.
	taskRepo.mu.Lock()
	taskRepo.getTaskErr = nil
	taskRepo.tasks[deferredChainTaskID] = &v1.Task{
		ID: deferredChainTaskID, WorkflowID: "wf1",
		Title: "Chain step", Description: "desc", State: v1.TaskStateCreated,
	}
	taskRepo.mu.Unlock()

	svc.reconcileTaskLifecycleTokens(ctx)
	select {
	case <-launchAttempted:
	case <-time.After(2 * time.Second):
		t.Fatal("startup lifecycle recovery did not retry the promoted launch")
	}
	awaitLaunchedSession(t, baseRepo, deferredChainTaskID)

	stored, err := baseRepo.GetTask(ctx, deferredChainTaskID)
	require.NoError(t, err)
	_, deferredPending := stored.Metadata[models.MetaKeyDeferredLaunch]
	assert.False(t, deferredPending)
	_, promotionPending := stored.Metadata[models.MetaKeyQueuePromotionPending]
	assert.False(t, promotionPending)
	assert.Equal(t, int32(1), agentLaunches.Load())
	select {
	case call := <-recorder.calls:
		assert.Equal(t, "profile1", call.profileID)
		assert.Equal(t, "creator-1", call.userID)
	case <-time.After(2 * time.Second):
		t.Fatal("retried promoted task-create launch did not record profile recency")
	}
}

// intentObserver records the dangerous state: does the task still carry a
// claimable deferred launch intent at a moment when this start already owns a
// session row?
//
// The observation point is the session reload startTask performs between
// preparing the session and launching it. Session creation itself runs through
// the executor's own repo handle, not this one, so the first orchestrator-owned
// read of the new session is the earliest seam available here. (The title claim
// would be tidier but only reaches the repository for tasks awaiting a
// generated title, so it never fires for this fixture — which is why the test
// asserts it observed something at all.)
type intentObserver struct {
	*sqliterepo.Repository
	sessionExisted *bool
	intentPresent  *bool
}

func (r *intentObserver) GetTaskSession(ctx context.Context, sessionID string) (*models.TaskSession, error) {
	session, err := r.Repository.GetTaskSession(ctx, sessionID)
	if r.intentPresent == nil && err == nil && session != nil {
		if task, taskErr := r.GetTask(ctx, session.TaskID); taskErr == nil && task != nil {
			_, present := task.Metadata[models.MetaKeyDeferredLaunch]
			r.intentPresent = &present
			existed := true
			r.sessionExisted = &existed
		}
	}
	return session, err
}

// The window between "this start created a session" and "this start launches".
//
// Reserving the intent immediately before LaunchPreparedSession left roughly a
// hundred lines and half a dozen DB round trips — session preparation, workflow
// session config, passthrough resolution, prompt building, the title claim — in
// which the session row already existed and the intent was still claimable. A
// gate opening there takes it and launches its own session next to the one
// about to start: the same two-session outcome by a third route.
//
// Asserting the invariant directly rather than racing a real gate through it:
// once this start owns a session, the intent must already be gone, so there is
// nothing left for a gate to claim whenever it fires. Driving an actual
// concurrent gate through this window deadlocks on SQLite's single writer, and
// a test that hangs is worse than one that states the property.
//
// Found by re-reading the fix, not by a reviewer: the mid-launch test cannot
// see this window, because by then the claim has been taken either way.
func TestTheLaunchIntentIsClaimedBeforeTheStartOwnsASession(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedChainStepTask(t, repo, deferredChainTaskID)
	counter := newLaunchCounter()
	svc := newDeferredLaunchTestService(t, repo, counter)

	observer := &intentObserver{Repository: repo}
	svc.repo = observer

	_, err := svc.StartTask(ctx, deferredChainTaskID, "profile1", "", "", "",
		"do the work with what we know now", "", false, false, nil)
	require.NoError(t, err)

	require.NotNil(t, observer.intentPresent, "the observation point never ran; the test measured nothing")
	require.NotNil(t, observer.sessionExisted)
	require.True(t, *observer.sessionExisted,
		"fixture must observe a point where this start already owns a session")
	assert.False(t, *observer.intentPresent,
		"the intent must already be claimed once this start owns a session, or a gate "+
			"firing in between launches a second one alongside it")
}
