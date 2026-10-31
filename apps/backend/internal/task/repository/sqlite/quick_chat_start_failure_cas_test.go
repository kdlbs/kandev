package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestCommitQuickChatStartFailureRequiresCurrentOwner(t *testing.T) {
	tests := []struct {
		name                string
		state               models.TaskSessionState
		quickChatAttemptID  string
		agentStartAttemptID string
		wantChanged         bool
	}{
		{
			name:                "current attempt",
			state:               models.TaskSessionStateStarting,
			quickChatAttemptID:  "creation-attempt",
			agentStartAttemptID: "execution-attempt",
			wantChanged:         true,
		},
		{
			name:                "replacement creation",
			state:               models.TaskSessionStateWaitingForInput,
			quickChatAttemptID:  "replacement-creation",
			agentStartAttemptID: "replacement-execution",
		},
		{
			name:                "replacement execution",
			state:               models.TaskSessionStateWaitingForInput,
			quickChatAttemptID:  "creation-attempt",
			agentStartAttemptID: "replacement-execution",
		},
		{
			name:                "settled state changed",
			state:               models.TaskSessionStateWaitingForInput,
			quickChatAttemptID:  "creation-attempt",
			agentStartAttemptID: "execution-attempt",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ctx := context.Background()
			repo := newRepoForSessionTests(t)
			taskID := "task-quick-chat-cas"
			sessionID := "session-quick-chat-cas"
			seedForMsgTest(t, repo, taskID, sessionID, "turn-quick-chat-cas")
			require.NoError(t, repo.UpdateTaskSessionState(ctx, sessionID, test.state, ""))
			require.NoError(t, repo.UpdateSessionMetadata(ctx, sessionID, map[string]interface{}{
				models.SessionMetaKeyQuickChatStartAttemptID: test.quickChatAttemptID,
				models.SessionMetaKeyAgentStartAttemptID:     test.agentStartAttemptID,
			}))
			errorValue := models.LastAgentError{Message: "startup failed", OccurredAt: time.Now().UTC()}
			changed, _, err := repo.CommitQuickChatStartFailureIfCurrentAttempt(
				ctx,
				taskID,
				sessionID,
				"creation-attempt",
				"execution-attempt",
				models.TaskSessionStateStarting,
				errorValue,
			)

			require.NoError(t, err)
			require.Equal(t, test.wantChanged, changed)
			stored, getErr := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, getErr)
			if test.wantChanged {
				require.Equal(t, models.TaskSessionStateFailed, stored.State)
				lastError, ok := models.LoadLastAgentError(stored.Metadata)
				require.True(t, ok)
				require.Equal(t, "startup failed", lastError.Message)
				return
			}
			require.Equal(t, test.state, stored.State)
			_, hasError := models.LoadLastAgentError(stored.Metadata)
			require.False(t, hasError)
		})
	}
}
