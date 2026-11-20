package sqlite

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestPostgresCommitQuickChatStartFailureIfCurrentAttempt(t *testing.T) {
	db := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	repo, err := NewWithDB(db, db, nil)
	require.NoError(t, err)
	ctx := context.Background()

	tests := []struct {
		name                string
		state               models.TaskSessionState
		storedCreationID    string
		storedExecutionID   string
		expectedCreationID  string
		expectedExecutionID string
		wantChanged         bool
	}{
		{
			name:                "current attempt commits failure",
			state:               models.TaskSessionStateStarting,
			storedCreationID:    "quick-attempt-current",
			storedExecutionID:   "execution-attempt-current",
			expectedCreationID:  "quick-attempt-current",
			expectedExecutionID: "execution-attempt-current",
			wantChanged:         true,
		},
		{
			name:                "old execution cannot fail its settled replacement",
			state:               models.TaskSessionStateWaitingForInput,
			storedCreationID:    "quick-attempt-current",
			storedExecutionID:   "execution-attempt-replacement",
			expectedCreationID:  "quick-attempt-current",
			expectedExecutionID: "execution-attempt-current",
		},
		{
			name:                "old creation cannot fail its settled replacement",
			state:               models.TaskSessionStateWaitingForInput,
			storedCreationID:    "quick-attempt-replacement",
			storedExecutionID:   "execution-attempt-current",
			expectedCreationID:  "quick-attempt-current",
			expectedExecutionID: "execution-attempt-current",
		},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			taskID := "task-quick-chat-failure-pg-" + strconv.Itoa(index)
			sessionID := "session-quick-chat-failure-pg-" + strconv.Itoa(index)
			seedPostgresTask(t, repo, taskID)
			require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
				ID: sessionID, TaskID: taskID, State: test.state, ErrorMessage: "replacement remains healthy",
			}))
			require.NoError(t, repo.UpdateSessionMetadata(ctx, sessionID, map[string]interface{}{
				models.SessionMetaKeyQuickChatStartAttemptID: test.storedCreationID,
				models.SessionMetaKeyAgentStartAttemptID:     test.storedExecutionID,
			}))

			changed, _, err := repo.CommitQuickChatStartFailureIfCurrentAttempt(
				ctx,
				taskID,
				sessionID,
				test.expectedCreationID,
				test.expectedExecutionID,
				test.state,
				models.LastAgentError{Message: "stale Quick Chat startup failure", OccurredAt: time.Now().UTC()},
			)
			require.NoError(t, err)
			require.Equal(t, test.wantChanged, changed)

			stored, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			if test.wantChanged {
				require.Equal(t, models.TaskSessionStateFailed, stored.State)
				_, hasError := models.LoadLastAgentError(stored.Metadata)
				require.True(t, hasError)
				return
			}
			require.Equal(t, models.TaskSessionStateWaitingForInput, stored.State)
			require.Equal(t, "replacement remains healthy", stored.ErrorMessage)
			_, hasError := models.LoadLastAgentError(stored.Metadata)
			require.False(t, hasError, "a stale owner cannot persist a failure on PostgreSQL")
		})
	}
}
