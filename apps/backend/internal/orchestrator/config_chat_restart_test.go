package orchestrator

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type configChatRetirerUnderTest interface {
	RetireConfigChatSession(context.Context, string, string, func(context.Context) error) error
}

func TestConfigChatRestartBlankStartDoesNotDispatchConfigInstructions(t *testing.T) {
	ctx := context.Background()
	repo := setupTestRepo(t)
	seedTaskAndSession(t, repo, "new-task", "new-session", models.TaskSessionStateCreated)
	task, err := repo.GetTask(ctx, "new-task")
	require.NoError(t, err)
	task.IsEphemeral = true
	task.Description = ""
	task.WorkflowID, task.WorkflowStepID = "", ""
	task.Metadata = map[string]interface{}{"config_mode": true}
	require.NoError(t, repo.UpdateTask(ctx, task))
	session, err := repo.GetTaskSession(ctx, "new-session")
	require.NoError(t, err)
	require.NoError(t, repo.UpdateSessionMetadata(ctx, session.ID, map[string]interface{}{"config_mode": true}))
	taskRepo := newMockTaskRepo()
	taskRepo.tasks[task.ID] = &v1.Task{ID: task.ID, Title: task.Title, IsEphemeral: true}
	var launched *executor.LaunchAgentRequest
	manager := &mockAgentManager{launchAgentFunc: func(_ context.Context, request *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
		launched = request
		return &executor.LaunchAgentResponse{AgentExecutionID: "new-execution", Status: v1.AgentStatusStarting}, nil
	}}
	svc := createTestServiceWithScheduler(repo, newMockStepGetter(), taskRepo, manager)
	_, err = svc.LaunchSession(ctx, &LaunchSessionRequest{TaskID: task.ID, SessionID: session.ID, AgentProfileID: "profile1", Intent: IntentStartCreated, ActivationSource: LaunchActivationSourceUserAction, NoInitialPrompt: true, SkipMessageRecord: true})
	require.NoError(t, err)
	require.NotNil(t, launched)
	assert.True(t, launched.StartAgent)
	assert.Equal(t, executor.McpModeConfig, launched.McpMode)
	assert.Empty(t, launched.TaskDescription, "configuration instructions accompany a real prompt, not an empty start")
}

func TestConfigChatRestartPhysicalStopBeforeDelete(t *testing.T) {
	for _, state := range []models.TaskSessionState{models.TaskSessionStateStarting, models.TaskSessionStateRunning, models.TaskSessionStateWaitingForInput, models.TaskSessionStateFailed, models.TaskSessionStateCancelled} {
		t.Run(string(state), func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "old-task", "old-session", "step1")
			require.NoError(t, repo.UpdateTaskSessionState(context.Background(), "old-session", state, ""))
			entered, release, deleted := make(chan struct{}), make(chan struct{}), make(chan struct{}, 1)
			var releaseOnce sync.Once
			t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "execution-old", nil },
				stopAgentWithReasonFunc:      func(context.Context, string, string, bool) error { close(entered); <-release; return nil },
			}
			svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
			retirer, ok := any(svc).(configChatRetirerUnderTest)
			require.True(t, ok, "configuration chat retirement must own the stop/delete boundary")
			done := make(chan error, 1)
			go func() {
				done <- retirer.RetireConfigChatSession(context.Background(), "old-task", "old-session", func(ctx context.Context) error {
					deleted <- struct{}{}
					return repo.DeleteTask(ctx, "old-task")
				})
			}()
			<-entered
			select {
			case <-deleted:
				t.Fatal("deleted before physical stop completed")
			default:
			}
			assert.ErrorIs(t, svc.validatePromptTaskStart("old-session"), ErrSessionResetInProgress)
			releaseOther, acquired := svc.tryAcquireSessionLifecycleLock("old-session")
			if acquired {
				releaseOther()
			}
			assert.False(t, acquired, "retirement must hold lifecycle exclusion until deletion")
			releaseOnce.Do(func() { close(release) })
			require.NoError(t, <-done)
			_, err := repo.GetTaskSession(context.Background(), "old-session")
			assert.Error(t, err)
		})
	}
}

func TestConfigChatRestartAbsentAndFailedRuntime(t *testing.T) {
	for _, test := range []struct {
		name               string
		lookupErr, stopErr error
		deleted            bool
	}{
		{name: "confirmed absent", lookupErr: lifecycle.ErrNoExecutionForSession, deleted: true},
		{name: "lookup failed", lookupErr: errors.New("unavailable")},
		{name: "stop failed", stopErr: errors.New("unavailable")},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "old-task", "old-session", "step1")
			manager := &mockAgentManager{
				getExecutionIDForSessionFunc: func(context.Context, string) (string, error) { return "execution-old", test.lookupErr },
				stopAgentWithReasonErr:       test.stopErr,
			}
			svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
			retirer, ok := any(svc).(configChatRetirerUnderTest)
			require.True(t, ok)
			deleted := false
			err := retirer.RetireConfigChatSession(context.Background(), "old-task", "old-session", func(context.Context) error { deleted = true; return nil })
			assert.Equal(t, test.deleted, deleted)
			if test.deleted {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			assert.False(t, svc.isSessionResetInProgress("old-session"), "failure must allow retry")
		})
	}
}

func TestConfigChatRestartAuthorizationAndPair(t *testing.T) {
	for _, test := range []string{"authorization", "pair", "missing session"} {
		t.Run(test, func(t *testing.T) {
			repo := setupTestRepo(t)
			seedSession(t, repo, "old-task", "old-session", "step1")
			manager := &mockAgentManager{}
			svc := newCoordinatorStopTestService(repo, newMockTaskRepo(), manager)
			if test == "authorization" {
				svc.SetSessionAccessChecker(func(context.Context, string) error { return errors.New("denied") })
			}
			taskID, sessionID := "old-task", "old-session"
			if test == "pair" {
				taskID = "different-task"
			}
			if test == "missing session" {
				sessionID = "missing"
			}
			retirer, ok := any(svc).(configChatRetirerUnderTest)
			require.True(t, ok)
			deleted := false
			err := retirer.RetireConfigChatSession(context.Background(), taskID, sessionID, func(context.Context) error { deleted = true; return nil })
			require.Error(t, err)
			assert.False(t, deleted)
			assert.Empty(t, manager.stopAgentWithReasonArgs)
		})
	}
}
