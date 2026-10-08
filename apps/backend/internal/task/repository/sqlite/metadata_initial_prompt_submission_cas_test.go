package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestInitialPromptSubmissionRecordSurvivesSQLiteReopen(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "initial-submission-restart.db")
	connection, err := db.OpenSQLite(databasePath)
	require.NoError(t, err)
	writer := sqlx.NewDb(connection, "sqlite3")
	t.Cleanup(func() { _ = writer.Close() })
	repo, err := NewWithDB(writer, writer, nil)
	require.NoError(t, err)
	seedForMsgTest(t, repo, "task-submission-restart", "session-submission-restart", "turn-submission-restart")

	pending, err := models.NewInitialPromptSubmission("restart this original request", true, []v1.MessageAttachment{{
		AttachmentID: "image-restart", Type: "image", Name: "screen.png", MimeType: "image/png",
		SizeBytes: 19, DeliveryMode: "prompt",
	}})
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(context.Background(), "session-submission-restart", models.SessionMetaKeyInitialPromptSubmission, pending))
	require.NoError(t, writer.Close())

	reopenedConnection, err := db.OpenSQLite(databasePath)
	require.NoError(t, err)
	reopenedWriter := sqlx.NewDb(reopenedConnection, "sqlite3")
	t.Cleanup(func() { _ = reopenedWriter.Close() })
	reopenedRepo := NewWithInitializedDB(reopenedWriter, reopenedWriter, nil)
	session, err := reopenedRepo.GetTaskSession(context.Background(), "session-submission-restart")
	require.NoError(t, err)

	loaded, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	require.Equal(t, pending, loaded)
}

func TestSetSessionMetadataKeyIfJSONValueIsCompareAndSet(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-submission-cas", "session-submission-cas", "turn-submission-cas")
	ctx := context.Background()
	pending := map[string]interface{}{"version": 1, "state": "pending", "content": "original"}
	dispatching := map[string]interface{}{"version": 1, "state": "dispatching", "content": "original", "attempt_id": "resume-1"}
	accepted := map[string]interface{}{"version": 1, "state": "accepted", "content": "original", "attempt_id": "resume-1"}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-submission-cas", "unrelated", "keep"))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-submission-cas", "initial_prompt_submission", pending))

	stored, err := repo.SetSessionMetadataKeyIfJSONValue(ctx, "session-submission-cas", "initial_prompt_submission", pending, dispatching)
	require.NoError(t, err)
	require.True(t, stored)
	stored, err = repo.SetSessionMetadataKeyIfJSONValue(ctx, "session-submission-cas", "initial_prompt_submission", pending, accepted)
	require.NoError(t, err)
	require.False(t, stored, "a stale state must not overwrite the owned dispatch")
	stored, err = repo.SetSessionMetadataKeyIfJSONValue(ctx, "session-submission-cas", "initial_prompt_submission", dispatching, accepted)
	require.NoError(t, err)
	require.True(t, stored)

	session, err := repo.GetTaskSession(ctx, "session-submission-cas")
	require.NoError(t, err)
	storedSubmission, ok := session.Metadata["initial_prompt_submission"].(map[string]interface{})
	require.True(t, ok)
	require.Equal(t, "accepted", storedSubmission["state"])
	require.Equal(t, "resume-1", storedSubmission["attempt_id"])
	require.Equal(t, "original", storedSubmission["content"])
	require.Equal(t, "keep", session.Metadata["unrelated"])
}

func TestSetSessionMetadataKeyIfJSONValueSupportsInitialPromptSubmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-submission-model-cas", "session-submission-model-cas", "turn-submission-model-cas")
	ctx := context.Background()
	pending, err := models.NewInitialPromptSubmission("original", true, []v1.MessageAttachment{{
		AttachmentID: "image-1", Type: "image", Name: "screen.png", MimeType: "image/png",
		SizeBytes: 17, DeliveryMode: "prompt",
	}})
	require.NoError(t, err)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-submission-model-cas", models.SessionMetaKeyInitialPromptSubmission, pending))

	dispatching := *pending
	dispatching.State = models.InitialPromptSubmissionDispatching
	dispatching.ExecutionID = "execution-1"
	dispatching.AttemptID = "attempt-1"
	dispatching.DispatchStartedAt = "2026-10-06T00:00:00Z"
	stored, err := repo.SetSessionMetadataKeyIfJSONValue(
		ctx, "session-submission-model-cas", models.SessionMetaKeyInitialPromptSubmission, pending, &dispatching,
	)
	require.NoError(t, err)
	require.True(t, stored)
}

func TestSetSessionMetadataKeyIfJSONValueSupportsLoadedInitialPromptSubmission(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-loaded-submission-cas", "session-loaded-submission-cas", "turn-loaded-submission-cas")
	ctx := context.Background()
	attachments := []v1.MessageAttachment{
		{AttachmentID: "image-1", Type: "image", Name: "screen.png", MimeType: "image/png", SizeBytes: 20, DeliveryMode: "prompt"},
		{AttachmentID: "resource-1", Type: "resource", Name: "report.zip", MimeType: "application/zip", SizeBytes: 23, DeliveryMode: "path"},
	}
	pending := map[string]interface{}{"version": 1, "content": "original", "plan_mode": true, "attachments": attachments, "state": "pending"}
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-loaded-submission-cas", models.SessionMetaKeyInitialPromptSubmission, pending))
	session, err := repo.GetTaskSession(ctx, "session-loaded-submission-cas")
	require.NoError(t, err)
	loaded, found, err := models.LoadInitialPromptSubmission(session.Metadata)
	require.NoError(t, err)
	require.True(t, found)
	dispatching := *loaded
	dispatching.State = models.InitialPromptSubmissionDispatching
	dispatching.ExecutionID = "execution-1"
	dispatching.AttemptID = "attempt-1"
	dispatching.DispatchStartedAt = "2026-10-06T00:00:00Z"
	stored, err := repo.SetSessionMetadataKeyIfJSONValue(ctx, "session-loaded-submission-cas", models.SessionMetaKeyInitialPromptSubmission, loaded, &dispatching)
	require.NoError(t, err)
	require.True(t, stored)
}

func TestSetSessionMetadataKeyIfJSONValueRequiresMatchingObjectMembers(t *testing.T) {
	tests := []struct {
		name     string
		suffix   string
		actual   map[string]interface{}
		expected map[string]interface{}
	}{
		{
			name:   "swapped execution and attempt ownership",
			suffix: "swapped",
			actual: map[string]interface{}{
				"execution_id": "attempt-1",
				"attempt_id":   "execution-1",
			},
			expected: map[string]interface{}{
				"execution_id": "execution-1",
				"attempt_id":   "attempt-1",
			},
		},
		{
			name:   "renamed member with duplicate values",
			suffix: "renamed",
			actual: map[string]interface{}{
				"execution_id": "same-value",
				"renamed_id":   "same-value",
			},
			expected: map[string]interface{}{
				"execution_id": "same-value",
				"attempt_id":   "same-value",
			},
		},
		{
			name:   "removed member with duplicate values",
			suffix: "removed",
			actual: map[string]interface{}{
				"execution_id": "same-value",
			},
			expected: map[string]interface{}{
				"execution_id": "same-value",
				"attempt_id":   "same-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := newRepoForSessionTests(t)
			taskID := "task-metadata-members-" + tt.suffix
			sessionID := "session-metadata-members-" + tt.suffix
			seedForMsgTest(t, repo, taskID, sessionID, "turn-metadata-members-"+tt.suffix)
			ctx := context.Background()
			require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, "submission", tt.actual))

			stored, err := repo.SetSessionMetadataKeyIfJSONValue(ctx, sessionID, "submission", tt.expected, map[string]interface{}{"state": "must-not-write"})
			require.NoError(t, err)
			require.False(t, stored, "different object members must fail the expected-value guard")

			session, err := repo.GetTaskSession(ctx, sessionID)
			require.NoError(t, err)
			require.Equal(t, tt.actual, session.Metadata["submission"], "a failed compare-and-set must preserve the actual value")
		})
	}
}

func TestSetSessionMetadataKeyIfJSONValueIgnoresObjectSerializationOrder(t *testing.T) {
	repo := newRepoForSessionTests(t)
	seedForMsgTest(t, repo, "task-metadata-order", "session-metadata-order", "turn-metadata-order")
	ctx := context.Background()
	actual := json.RawMessage(`{"attempt_id":"attempt-1","execution_id":"execution-1"}`)
	expected := json.RawMessage(`{"execution_id":"execution-1","attempt_id":"attempt-1"}`)
	require.NoError(t, repo.SetSessionMetadataKey(ctx, "session-metadata-order", "submission", actual))

	stored, err := repo.SetSessionMetadataKeyIfJSONValue(ctx, "session-metadata-order", "submission", expected, map[string]interface{}{"state": "dispatching"})
	require.NoError(t, err)
	require.True(t, stored, "JSON object member order must not affect structural equality")
}
