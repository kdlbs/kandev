//go:build unix

package orchestrator

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type blockingRecoverySelectionStore struct {
	worktree.Store
	reader      worktree.RecoverySelectionSnapshotReader
	entered     chan struct{}
	release     chan struct{}
	once        sync.Once
	releaseOnce sync.Once
}

func (s *blockingRecoverySelectionStore) releaseInspection() {
	s.releaseOnce.Do(func() { close(s.release) })
}

func (s *blockingRecoverySelectionStore) GetWorktreeByID(
	ctx context.Context,
	id string,
) (*worktree.Worktree, error) {
	return s.Store.GetWorktreeByID(context.WithoutCancel(ctx), id)
}

func (s *blockingRecoverySelectionStore) ReadRecoverySelectionSnapshot(
	ctx context.Context,
	expected models.WorkspaceRecoverySelectionSnapshot,
) (models.WorkspaceRecoverySelectionSnapshot, error) {
	s.once.Do(func() {
		close(s.entered)
		<-s.release
	})
	return s.reader.ReadRecoverySelectionSnapshot(context.WithoutCancel(ctx), expected)
}

type observedInspectionWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observedInspectionWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-003.6, AC-TASKS-WORKTREE-METADATA-RECOVERY-003.7
func TestSessionOpenResumeWaitsForWorkspaceInspection(t *testing.T) {
	fixture := newCompletedRelocationServiceFixture(t, 1)
	ctx := context.Background()
	session, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	session.ErrorMessage = "previous session error"
	session.Metadata[models.SessionMetaKeyGitCredentialSnapshot] = map[string]interface{}{
		"source": "previous-credential",
	}
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, session))
	require.NoError(t, fixture.repo.SetSessionMetadataKey(
		ctx, fixture.sessionID, models.SessionMetaKeyGitCredentialSnapshot,
		map[string]interface{}{"source": "previous-credential"},
	))

	store := &blockingRecoverySelectionStore{
		Store: fixture.store, reader: fixture.store,
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	defer store.releaseInspection()
	manager, err := worktree.NewManager(fixture.config, store, testLogger())
	require.NoError(t, err)
	fixture.manager = manager
	starts := make(chan completedRelocationProviderStart, 2)
	fixture.rebuildService(starts, nil)
	taskRepo := fixture.svc.taskRepo.(*mockTaskRepo)
	require.NoError(t, taskRepo.UpdateTaskState(ctx, fixture.taskID, v1.TaskStateReview))

	var admissions atomic.Int32
	waiting := make(chan struct{})
	var resumeInspectionWait *observedInspectionWaitContext
	resumeAdmissionWait := make(chan time.Duration, 1)
	fixture.svc.executor.SetSelectedWorktreeRecoveryAdmission(func(
		admissionCtx context.Context,
		request worktree.RecoveryAdmissionRequest,
	) (*worktree.RecoveryAdmission, error) {
		if admissions.Add(1) == 2 {
			resumeAdmissionWait <- request.InspectionWait
			resumeInspectionWait = &observedInspectionWaitContext{Context: admissionCtx, waiting: waiting}
			admissionCtx = resumeInspectionWait
		}
		return manager.AdmitRecovery(admissionCtx, request)
	})

	ownerResult := make(chan error, 1)
	go func() {
		_, preflightErr := fixture.svc.executor.PreflightSessionWorktreeRecovery(
			ctx, fixture.taskID, session, false,
		)
		ownerResult <- preflightErr
	}()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace inspection did not acquire its selected-worktree lock")
	}

	resumeResult := make(chan error, 1)
	go func() {
		_, launchErr := fixture.svc.LaunchSession(ctx, &LaunchSessionRequest{
			TaskID: fixture.taskID, SessionID: fixture.sessionID,
			Intent: IntentResume, ActivationSource: LaunchActivationSourceSessionOpen,
		})
		resumeResult <- launchErr
	}()

	var inspectionWait time.Duration
	select {
	case inspectionWait = <-resumeAdmissionWait:
	case err := <-resumeResult:
		require.NoError(t, err, "session-open resume returned while workspace inspection still owned the lock")
	case <-time.After(5 * time.Second):
		t.Fatal("session-open resume did not reach selected-worktree admission")
	}
	if inspectionWait == 0 {
		select {
		case resumeErr := <-resumeResult:
			storedTask, loadErr := taskRepo.GetTask(ctx, fixture.taskID)
			require.NoError(t, loadErr)
			require.Equal(t, v1.TaskStateReview, storedTask.State,
				"inspection contention must not fail the task")
			require.NoError(t, resumeErr,
				"session-open resume must wait for the active workspace inspection")
		case <-time.After(5 * time.Second):
			t.Fatal("nonblocking resume admission did not return while the lock was held")
		}
	}

	select {
	case <-waiting:
	case err := <-resumeResult:
		require.NoError(t, err, "session-open resume failed instead of waiting for workspace inspection")
	case <-time.After(5 * time.Second):
		t.Fatal("session-open resume did not wait for the active workspace inspection")
	}

	storedTask, err := taskRepo.GetTask(ctx, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateReview, storedTask.State)
	storedSession, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, storedSession.State)
	require.Equal(t, "previous session error", storedSession.ErrorMessage)
	require.Equal(t, map[string]interface{}{"source": "previous-credential"},
		storedSession.Metadata[models.SessionMetaKeyGitCredentialSnapshot])

	store.releaseInspection()
	select {
	case err := <-ownerResult:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("workspace inspection did not complete after release")
	}

	select {
	case start := <-starts:
		require.Equal(t, fixture.sessionID, start.sessionID)
		require.Equal(t, "provider-conversation-kept", start.resumeToken)
		waitForCompletedRelocationAttempt(t, fixture, start)
		fixture.svc.handleAgentBootReady(context.Background(), completedRelocationReadyEvent(start))
	case err := <-resumeResult:
		require.NoError(t, err)
		t.Fatal("resume completed without starting the existing provider conversation")
	case <-time.After(5 * time.Second):
		t.Fatal("resume did not start the existing provider conversation")
	}
	select {
	case err := <-resumeResult:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("session-open resume did not finish after the provider became ready")
	}
	require.Equal(t, 1, fixture.launchStarts, "one existing conversation should start")
	storedTask, err = taskRepo.GetTask(ctx, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateReview, storedTask.State, "silent resume must retain workflow position")
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-003.6, AC-TASKS-WORKTREE-METADATA-RECOVERY-003.7
func TestSessionOpenResumeHonorsEarlierCallerDeadline(t *testing.T) {
	fixture := newCompletedRelocationServiceFixture(t, 1)
	ctx := context.Background()
	session, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, fixture.repo.UpdateTaskSession(ctx, session))
	require.NoError(t, fixture.repo.SetSessionMetadataKey(
		ctx, fixture.sessionID, models.SessionMetaKeyGitCredentialSnapshot,
		map[string]interface{}{"source": "previous-credential"},
	))

	store := &blockingRecoverySelectionStore{
		Store: fixture.store, reader: fixture.store,
		entered: make(chan struct{}), release: make(chan struct{}),
	}
	defer store.releaseInspection()
	manager, err := worktree.NewManager(fixture.config, store, testLogger())
	require.NoError(t, err)
	fixture.manager = manager
	fixture.rebuildService(make(chan completedRelocationProviderStart, 1), nil)
	taskRepo := fixture.svc.taskRepo.(*mockTaskRepo)
	require.NoError(t, taskRepo.UpdateTaskState(ctx, fixture.taskID, v1.TaskStateReview))

	ownerResult := make(chan error, 1)
	go func() {
		_, preflightErr := fixture.svc.executor.PreflightSessionWorktreeRecovery(
			ctx, fixture.taskID, session, false,
		)
		ownerResult <- preflightErr
	}()
	select {
	case <-store.entered:
	case <-time.After(5 * time.Second):
		t.Fatal("workspace inspection did not acquire its selected-worktree lock")
	}

	requestCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	callerDeadline, ok := requestCtx.Deadline()
	require.True(t, ok)
	resumeAdmission := make(chan worktree.RecoveryAdmissionRequest, 1)
	fixture.svc.executor.SetSelectedWorktreeRecoveryAdmission(func(
		admissionCtx context.Context,
		request worktree.RecoveryAdmissionRequest,
	) (*worktree.RecoveryAdmission, error) {
		resumeAdmission <- request
		return manager.AdmitRecovery(admissionCtx, request)
	})

	startedAt := time.Now()
	_, launchErr := fixture.svc.LaunchSession(requestCtx, &LaunchSessionRequest{
		TaskID: fixture.taskID, SessionID: fixture.sessionID,
		Intent: IntentResume, ActivationSource: LaunchActivationSourceSessionOpen,
	})
	elapsed := time.Since(startedAt)
	var contention *worktree.RecoveryInspectionContentionError
	require.ErrorAs(t, launchErr, &contention)
	require.Less(t, elapsed, 2*time.Second,
		"session-open resume should honor the shorter caller deadline instead of starting a 15-second wait")
	select {
	case request := <-resumeAdmission:
		require.True(t, request.InspectionDeadline.Equal(callerDeadline),
			"session-open admission must retain the caller's absolute deadline")
	case <-time.After(time.Second):
		t.Fatal("session-open resume did not request selected-worktree admission")
	}

	storedTask, err := taskRepo.GetTask(ctx, fixture.taskID)
	require.NoError(t, err)
	require.Equal(t, v1.TaskStateReview, storedTask.State,
		"caller-deadline contention must not fail the task")
	storedSession, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
	require.NoError(t, err)
	require.Equal(t, models.TaskSessionStateWaitingForInput, storedSession.State,
		"caller-deadline contention must preserve the session")

	store.releaseInspection()
	select {
	case ownerErr := <-ownerResult:
		require.NoError(t, ownerErr)
	case <-time.After(5 * time.Second):
		t.Fatal("workspace inspection did not finish after releasing its lock")
	}
}

// @covers AC-TASKS-WORKTREE-METADATA-RECOVERY-003.7
func TestSessionOpenLaunchFailureExemptsOnlyVerifiedInspectionDeferral(t *testing.T) {
	contention := &worktree.RecoveryInspectionContentionError{}
	startupErr := errors.New("lifecycle startup cleanup failed")
	tests := []struct {
		name          string
		launchErr     error
		rollbackErr   error
		wantTaskState v1.TaskState
		wantSession   models.TaskSessionState
	}{
		{
			name:          "safe lifecycle contention preserves workflow state",
			launchErr:     contention,
			wantTaskState: v1.TaskStateReview,
			wantSession:   models.TaskSessionStateWaitingForInput,
		},
		{
			name:          "joined startup failure receives normal failure bookkeeping",
			launchErr:     errors.Join(contention, startupErr),
			wantTaskState: v1.TaskStateFailed,
			wantSession:   models.TaskSessionStateFailed,
		},
		{
			name:          "rollback failure receives normal failure bookkeeping",
			launchErr:     contention,
			rollbackErr:   errors.New("resume rollback persistence failed"),
			wantTaskState: v1.TaskStateFailed,
			wantSession:   models.TaskSessionStateFailed,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fixture := newCompletedRelocationServiceFixture(t, 1)
			ctx := context.Background()
			session, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
			require.NoError(t, err)
			session.State = models.TaskSessionStateWaitingForInput
			session.ErrorMessage = "previous session error"
			require.NoError(t, fixture.repo.UpdateTaskSession(ctx, session))
			taskRepo := fixture.svc.taskRepo.(*mockTaskRepo)
			require.NoError(t, taskRepo.UpdateTaskState(ctx, fixture.taskID, v1.TaskStateReview))
			fixture.agent.launchAgentFunc = func(
				context.Context,
				*executor.LaunchAgentRequest,
			) (*executor.LaunchAgentResponse, error) {
				return nil, tt.launchErr
			}
			if tt.rollbackErr != nil {
				fixture.svc.executor.SetOnResumeFailureRollback(func(
					context.Context,
					executor.ResumeFailureRollbackRequest,
				) (bool, error) {
					return false, tt.rollbackErr
				})
			}

			_, resumeErr := fixture.svc.LaunchSession(ctx, &LaunchSessionRequest{
				TaskID: fixture.taskID, SessionID: fixture.sessionID,
				Intent: IntentResume, ActivationSource: LaunchActivationSourceSessionOpen,
			})
			require.Error(t, resumeErr)
			var gotContention *worktree.RecoveryInspectionContentionError
			require.ErrorAs(t, resumeErr, &gotContention)
			if tt.rollbackErr != nil {
				require.ErrorIs(t, resumeErr, tt.rollbackErr)
			}
			storedTask, err := taskRepo.GetTask(ctx, fixture.taskID)
			require.NoError(t, err)
			require.Equal(t, tt.wantTaskState, storedTask.State)
			storedSession, err := fixture.repo.GetTaskSession(ctx, fixture.sessionID)
			require.NoError(t, err)
			require.Equal(t, tt.wantSession, storedSession.State)
			if tt.wantSession == models.TaskSessionStateWaitingForInput {
				require.Equal(t, "previous session error", storedSession.ErrorMessage)
			}
		})
	}
}
