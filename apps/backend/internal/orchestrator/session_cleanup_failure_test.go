package orchestrator

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

type cleanupFailureAgentManager struct {
	mockAgentManager
	cleanup func(context.Context, string) error
}

func (m *cleanupFailureAgentManager) CleanupStaleExecutionBySessionID(ctx context.Context, sessionID string) error {
	return m.cleanup(ctx, sessionID)
}

// @covers AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-002.12
func TestSessionCleanupFailureProjection(t *testing.T) {
	for _, superseded := range []string{"", "state", "error", "fallback"} {
		name := "durable-reload"
		if superseded != "" {
			name = "newer-" + superseded + "-wins"
		}
		t.Run(name, func(t *testing.T) {
			ctx := context.Background()
			repo := setupTestRepo(t)
			initialState := models.TaskSessionStateFailed
			if superseded == "fallback" {
				initialState = models.TaskSessionStateWaitingForInput
			}
			seedTaskAndSession(t, repo, "task1", "session1", initialState)
			now := time.Now().UTC()
			require.NoError(t, repo.UpsertExecutorRunning(ctx, &models.ExecutorRunning{ID: "running1", SessionID: "session1", TaskID: "task1", ResumeToken: "saved-native-conversation", CreatedAt: now, UpdatedAt: now}))
			session, err := repo.GetTaskSession(ctx, "session1")
			require.NoError(t, err)
			session.AgentProfileID = "profile-1"
			require.NoError(t, repo.UpdateTaskSession(ctx, session))
			cleanupErr := errors.New("proxy 192.0.2.10:10250 failed /var/private token=secret")
			manager := &cleanupFailureAgentManager{cleanup: func(ctx context.Context, sessionID string) error {
				if superseded == "state" || superseded == "error" {
					current, getErr := repo.GetTaskSession(ctx, sessionID)
					require.NoError(t, getErr)
					if superseded == "state" {
						current.State = models.TaskSessionStateRunning
					} else {
						require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, models.LastAgentError{Message: "newer error", StampValue: "newer-error", OccurredAt: time.Now().UTC()}))
					}
					current.ErrorMessage = "newer owner"
					require.NoError(t, repo.UpdateTaskSession(ctx, current))
				}
				return cleanupErr
			}}
			manager.launchAgentFunc = func(context.Context, *executor.LaunchAgentRequest) (*executor.LaunchAgentResponse, error) {
				return nil, lifecycle.ErrAgentAlreadyRunning
			}
			svc := createTestServiceWithAgent(repo, newMockStepGetter(), newMockTaskRepo(), manager)
			svc.messageCreator = newServiceBackedMessageCreator(repo)
			svc.executor = executor.NewExecutor(manager, repo, testLogger(), executor.ExecutorConfig{})
			_, err = svc.ResumeTaskSession(ctx, "task1", "session1")
			require.ErrorIs(t, err, cleanupErr)
			current, getErr := repo.GetTaskSession(ctx, "session1")
			require.NoError(t, getErr)
			lastError, present := models.LoadLastAgentError(current.Metadata)
			if superseded == "state" || superseded == "error" {
				if superseded == "state" {
					require.Equal(t, models.TaskSessionStateRunning, current.State)
					require.False(t, present)
				} else {
					require.True(t, present)
					require.Equal(t, "newer-error", lastError.Stamp())
				}
				return
			}
			require.True(t, present, "cleanup failure must survive reload")
			require.Equal(t, initialState, current.State)
			running, runningErr := repo.GetExecutorRunningBySessionID(ctx, "session1")
			require.NoError(t, runningErr)
			require.Equal(t, "saved-native-conversation", running.ResumeToken)
			require.Equal(t, "runtime_cleanup_failed", lastError.Causes[0].Code)
			require.NotEmpty(t, lastError.AttemptID)
			require.Empty(t, lastError.ExecutionID, "no replacement execution was started")
			require.NotContains(t, lastError.Details, "192.0.2.")
			require.NotContains(t, current.ErrorMessage, "secret")
			identity, ok := RecoveryFailureIdentity(err)
			require.True(t, ok)
			require.Equal(t, lastError.Stamp(), identity.ErrorStamp)
			messages, listErr := repo.ListMessages(ctx, "session1")
			require.NoError(t, listErr)
			require.NotEmpty(t, messages, "accepted failure remains in conversation history")
		})
	}
}
