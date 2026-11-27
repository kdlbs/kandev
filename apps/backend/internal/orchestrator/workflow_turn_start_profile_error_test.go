package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	sqliterepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type failTurnStartWorkflowRouteRepo struct {
	*sqliterepo.Repository
	err error
}

func (r failTurnStartWorkflowRouteRepo) SetTaskMetadataKey(
	ctx context.Context,
	taskID, key string,
	value interface{},
) error {
	if key == models.MetaKeyWorkflowSessionRoute {
		return r.err
	}
	return r.Repository.SetTaskMetadataKey(ctx, taskID, key, value)
}

func TestTurnStartProfilePreparationFailurePreservesResume(t *testing.T) {
	for _, useEngine := range []bool{false, true} {
		name := "legacy"
		if useEngine {
			name = "engine"
		}
		t.Run(name, func(t *testing.T) {
			testTurnStartProfilePreparationFailurePreservesResume(t, useEngine)
		})
	}
}

func testTurnStartProfilePreparationFailurePreservesResume(t *testing.T, useEngine bool) {
	t.Helper()
	ctx := context.Background()
	fixture, session, routeErr := newTurnStartProfileFailureFixture(t, useEngine)

	eventBus := bus.NewMemoryEventBus(testLogger())
	t.Cleanup(eventBus.Close)
	fixture.svc.eventBus = eventBus
	falseWaitingEvents := watchFalseTurnStartWaitingEvents(t, eventBus, session.ID)
	releaseIssuer, resumeDone := holdResumeAtCredentialBoundary(t, fixture, session)

	claimed, err := fixture.repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, claimed.State)
	attemptID := models.StringFromAny(claimed.Metadata[models.SessionMetaKeyAgentStartAttemptID])
	require.NotEmpty(t, attemptID, "resume must own a startup attempt before turn-start evaluation")
	startupErrorMessage := claimed.ErrorMessage

	transitionsBefore := countTurnStartWorkflowTransitions(t, fixture)
	_, turnStartErr := fixture.svc.processOnTurnStartAdmission(ctx, "t1", session.ID, true)
	require.ErrorIs(t, turnStartErr, routeErr, "strict evaluation must surface the underlying profile preparation error")
	assertTurnStartProfileFailurePreserved(
		t, fixture, session, attemptID, startupErrorMessage, transitionsBefore, falseWaitingEvents,
	)

	releaseIssuer()
	select {
	case resumeErr := <-resumeDone:
		require.NoError(t, resumeErr, "credential snapshot persistence must keep the resume attempt valid")
	case <-time.After(10 * time.Second):
		require.FailNow(t, "resume did not finish after releasing the credential boundary")
	}
	resumed, err := fixture.repo.GetTaskSession(ctx, session.ID)
	require.NoError(t, err)
	require.Equal(t, attemptID, models.StringFromAny(resumed.Metadata[models.SessionMetaKeyAgentStartAttemptID]))
	require.Equal(t, startupErrorMessage, resumed.ErrorMessage)
}

func newTurnStartProfileFailureFixture(
	t *testing.T,
	useEngine bool,
) (*profileSwitchFixture, *models.TaskSession, error) {
	t.Helper()
	ctx := context.Background()
	fixture := newProfileSwitchFixture(
		t,
		models.WorkflowProfileSessionStartPolicyReuse,
		models.WorkflowProfileSessionEndPolicyPark,
	)
	session, err := fixture.repo.GetTaskSession(ctx, fixture.current.ID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	session.RepositoryID = "route-failure-repository"
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, session))
	require.NoError(t, fixture.repo.CreateRepository(ctx, &models.Repository{
		ID: "route-failure-repository", WorkspaceID: "ws1", Name: "widgets",
		SourceType: "local", LocalPath: t.TempDir(), Provider: "github",
		ProviderOwner: "acme", ProviderName: "widgets", DefaultBranch: "main",
		RemoteURL: "https://github.com/acme/widgets.git",
	}))
	require.NoError(t, fixture.repo.CreateTaskRepository(ctx, &models.TaskRepository{
		ID: "route-failure-task-repository", TaskID: "t1", RepositoryID: "route-failure-repository",
	}))
	fixture.stepGetter.steps["step-a"] = &wfmodels.WorkflowStep{
		ID: "step-a", WorkflowID: "wf1", Name: "Implement", Position: 0,
		AgentProfileID: "profile-a",
		Events:         wfmodels.StepEvents{OnTurnStart: []wfmodels.OnTurnStartAction{{Type: wfmodels.OnTurnStartMoveToNext}}},
	}
	fixture.stepGetter.steps["step-b"] = &wfmodels.WorkflowStep{
		ID: "step-b", WorkflowID: "wf1", Name: "Review", Position: 1, AgentProfileID: "profile-a",
	}
	routeErr := errors.New("injected workflow route persistence failure")
	fixture.svc.repo = failTurnStartWorkflowRouteRepo{Repository: fixture.repo, err: routeErr}
	if useEngine {
		fixture.svc.SetWorkflowStepGetter(fixture.stepGetter)
	} else {
		fixture.svc.workflowEngine = nil
	}
	return fixture, session, routeErr
}

func holdResumeAtCredentialBoundary(
	t *testing.T,
	fixture *profileSwitchFixture,
	session *models.TaskSession,
) (func(), <-chan error) {
	t.Helper()
	ctx := context.Background()
	issuer := &turnStartBarrierCredentialIssuer{entered: make(chan struct{}), release: make(chan struct{})}
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(issuer.release) }) }
	fixture.svc.executor.SetGitHubCredentialBroker(issuer, "https://kandev.example/api/v1/github/credentials/resolve")
	resumeDone := make(chan error, 1)
	resumeFinished := make(chan struct{})
	go func() {
		defer close(resumeFinished)
		_, resumeErr := fixture.svc.executor.ResumeSession(ctx, session, true)
		resumeDone <- resumeErr
	}()
	t.Cleanup(func() {
		release()
		select {
		case <-resumeFinished:
		case <-time.After(10 * time.Second):
			t.Error("resume did not finish during test cleanup")
		}
	})
	select {
	case <-issuer.entered:
	case <-time.After(10 * time.Second):
		t.Fatal("resume did not reach the credential boundary")
	}
	return release, resumeDone
}

func watchFalseTurnStartWaitingEvents(
	t *testing.T,
	eventBus *bus.MemoryEventBus,
	sessionID string,
) *atomic.Int32 {
	t.Helper()
	var falseWaitingEvents atomic.Int32
	_, err := eventBus.Subscribe(events.TaskSessionStateChanged, func(_ context.Context, event *bus.Event) error {
		data, _ := event.Data.(map[string]interface{})
		if data != nil && data[metaKeySessionID] == sessionID &&
			data[metaKeyNewState] == string(models.TaskSessionStateWaitingForInput) {
			falseWaitingEvents.Add(1)
		}
		return nil
	})
	require.NoError(t, err)
	return &falseWaitingEvents
}

func countTurnStartWorkflowTransitions(t *testing.T, fixture *profileSwitchFixture) int {
	t.Helper()
	var count int
	require.NoError(t, fixture.repo.DB().QueryRowContext(
		context.Background(), `SELECT COUNT(*) FROM task_step_transitions WHERE task_id = ?`, "t1",
	).Scan(&count))
	return count
}

func assertTurnStartProfileFailurePreserved(
	t *testing.T,
	fixture *profileSwitchFixture,
	session *models.TaskSession,
	attemptID string,
	startupErrorMessage string,
	transitionsBefore int,
	falseWaitingEvents *atomic.Int32,
) {
	t.Helper()
	task, err := fixture.repo.GetTask(context.Background(), "t1")
	require.NoError(t, err)
	require.Equal(t, "step-b", task.WorkflowStepID, "step transition must stay committed")
	require.Equal(t, v1.TaskStateInProgress, task.State, "failed preparation must not project Review")
	require.Equal(t, 1, countTurnStartWorkflowTransitions(t, fixture)-transitionsBefore)
	updated, err := fixture.repo.GetTaskSession(context.Background(), session.ID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateStarting, updated.State)
	require.Equal(t, attemptID, models.StringFromAny(updated.Metadata[models.SessionMetaKeyAgentStartAttemptID]))
	require.Equal(t, startupErrorMessage, updated.ErrorMessage)
	require.Zero(t, falseWaitingEvents.Load(), "failed profile preparation must not publish a false waiting event")
	projected := fixture.svc.taskRepo.(*mockTaskRepo)
	projected.mu.Lock()
	reviewWrites := append([]v1.TaskState(nil), projected.stateHistory["t1"]...)
	projected.mu.Unlock()
	require.NotContains(t, reviewWrites, v1.TaskStateReview, "failed profile preparation must not publish a Review projection")
}
