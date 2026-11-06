package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestDismissLastAgentErrorSQLiteBehavior(t *testing.T) {
	testAgentErrorDismissalBehavior(t, newRepoForSessionTests(t))
}

// @covers AC-TASKS-AGENT-ERROR-DISMISSAL-001.1
// @covers AC-TASKS-AGENT-ERROR-DISMISSAL-001.2
// @covers AC-TASKS-AGENT-ERROR-DISMISSAL-001.3
// @covers AC-TASKS-AGENT-ERROR-DISMISSAL-001.5
func testAgentErrorDismissalBehavior(t *testing.T, repo *Repository) {
	t.Helper()
	expected := models.LastAgentError{
		Message: "agent connection failed", OccurredAt: time.Date(2026, 10, 18, 9, 0, 0, 310000000, time.UTC),
		Scope: models.ErrorScopeSession, AgentExecutionID: "execution-1", ExecutionID: "execution-1",
		Code: "agent_transport_lost", Details: "connection closed", RemediationURL: "https://example.com/help",
		StampValue: "failure-1", DeliverySubmissionID: "submission-1",
		RecoveryActions: []string{}, Causes: []models.AgentErrorCause{},
	}
	cases := []struct {
		name           string
		message        string
		occurredAt     string
		absentMetadata string
		missingSession bool
		wantChanged    bool
	}{
		{name: "matching_error", wantChanged: true},
		{name: "equivalent_fraction", occurredAt: "2026-10-18T09:00:00.310Z", wantChanged: true},
		{name: "equivalent_trailing_zeros", occurredAt: "2026-10-18T09:00:00.310000Z", wantChanged: true},
		{name: "equivalent_offset", occurredAt: "2026-10-18T11:00:00.310+02:00", wantChanged: true},
		{name: "different_message_same_time", message: "replacement error"},
		{name: "same_message_different_time", occurredAt: "2026-10-18T09:00:01.31Z"},
		{name: "same_message_one_millisecond_later", occurredAt: "2026-10-18T09:00:00.311Z"},
		{name: "different_message_and_time", message: "replacement error", occurredAt: "2026-10-18T10:00:00Z"},
		{name: "absent_error", absentMetadata: "{}"},
		{name: "json_null_metadata", absentMetadata: "null"},
		{name: "sql_null_metadata", absentMetadata: "sql-null"},
		{name: "missing_session", missingSession: true},
	}
	for index, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			sessionID := fmt.Sprintf("dismiss-session-%d", index)
			seedAgentErrorDismissalSession(t, ctx, repo, sessionID, expected)
			seedAgentErrorDismissalSession(t, ctx, repo, sessionID+"-other", expected)
			if test.message != "" || test.occurredAt != "" {
				value := agentErrorDismissalMetadata(t, expected)
				if test.message != "" {
					value["message"] = test.message
				}
				if test.occurredAt != "" {
					value["occurred_at"] = test.occurredAt
				}
				require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, value))
			}
			if test.absentMetadata != "" {
				var metadata any = test.absentMetadata
				if test.absentMetadata == "sql-null" {
					metadata = nil
				}
				_, err := repo.db.ExecContext(ctx, repo.db.Rebind("UPDATE task_sessions SET metadata = ? WHERE id = ?"), metadata, sessionID)
				require.NoError(t, err)
			}
			if test.absentMetadata == "sql-null" {
				assertNullMetadataDismissal(t, ctx, repo, sessionID, expected)
				return
			}
			assertAgentErrorDismissal(t, ctx, repo, sessionID, expected, test.missingSession, test.wantChanged)
		})
	}
}

func assertNullMetadataDismissal(t *testing.T, ctx context.Context, repo *Repository, sessionID string, expected models.LastAgentError) {
	t.Helper()
	type snapshot struct {
		Metadata  sql.NullString `db:"metadata"`
		UpdatedAt time.Time      `db:"updated_at"`
	}
	query := repo.db.Rebind("SELECT metadata, updated_at FROM task_sessions WHERE id = ?")
	var before, after snapshot
	require.NoError(t, repo.db.GetContext(ctx, &before, query, sessionID))
	changed, err := repo.DismissLastAgentError(ctx, sessionID, expected, time.Now().UTC())
	require.NoError(t, err)
	require.False(t, changed)
	require.NoError(t, repo.db.GetContext(ctx, &after, query, sessionID))
	require.False(t, after.Metadata.Valid)
	require.Equal(t, before, after)
}

func agentErrorDismissalMetadata(t *testing.T, value models.LastAgentError) map[string]any {
	t.Helper()
	encoded, err := json.Marshal(value)
	require.NoError(t, err)
	var metadata map[string]any
	require.NoError(t, json.Unmarshal(encoded, &metadata))
	return metadata
}

func seedAgentErrorDismissalSession(t *testing.T, ctx context.Context, repo *Repository, sessionID string, lastError models.LastAgentError) {
	t.Helper()
	taskID := sessionID + "-task"
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: taskID, Title: "Dismissal behavior"}))
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{ID: sessionID, TaskID: taskID}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, "independent", map[string]any{"enabled": true, "value": "keep"}))
	require.NoError(t, repo.SetSessionMetadataKey(ctx, sessionID, models.SessionMetaKeyLastAgentError, lastError))
	_, err := repo.db.ExecContext(ctx, repo.db.Rebind("UPDATE task_sessions SET updated_at = ? WHERE id = ?"),
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), sessionID)
	require.NoError(t, err)
}

func assertAgentErrorDismissal(t *testing.T, ctx context.Context, repo *Repository, sessionID string, expected models.LastAgentError, missingSession, wantChanged bool) {
	t.Helper()
	before, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	otherBefore, err := repo.GetTaskSession(ctx, sessionID+"-other")
	require.NoError(t, err)
	target := sessionID
	if missingSession {
		target += "-missing"
	}
	dismissedAt := time.Date(2026, 10, 18, 9, 30, 0, 120000000, time.UTC)
	changed, err := repo.DismissLastAgentError(ctx, target, expected, dismissedAt)
	require.NoError(t, err, "actual repository dismissal")
	require.Equal(t, wantChanged, changed)
	after, err := repo.GetTaskSession(ctx, sessionID)
	require.NoError(t, err)
	if wantChanged {
		got, ok := models.LoadLastAgentError(after.Metadata)
		require.True(t, ok, "real model reader must retain the error")
		require.True(t, got.IsDismissed(), "persisted error must be acknowledged")
		expected.DismissedAt = &dismissedAt
		require.Equal(t, expected, got)
		require.Equal(t, before.Metadata["independent"], after.Metadata["independent"])
		require.True(t, after.UpdatedAt.After(before.UpdatedAt))
	} else {
		require.Equal(t, before, after, "stale/missing dismissal must preserve the complete session")
	}
	otherAfter, err := repo.GetTaskSession(ctx, sessionID+"-other")
	require.NoError(t, err)
	require.Equal(t, otherBefore, otherAfter, "another session must remain untouched")
}
