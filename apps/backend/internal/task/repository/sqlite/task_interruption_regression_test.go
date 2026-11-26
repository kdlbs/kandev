package sqlite

import (
	"context"
	"encoding/json"
	testing "testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestTaskInterruptionWritesPreserveUnrelatedRawJSON(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)
	require.NoError(t, repo.CreateTaskSession(ctx, &models.TaskSession{
		ID: "session-raw-interruption", TaskID: casTaskID,
		QueueIncarnationID: "incarnation-raw-interruption",
	}))
	original := `{"large":9007199254740993,"nested":{"precise":9007199254740995,"text":"leave exactly"},"unrelated":[1,2,3]}`
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), original, casTaskID)
	require.NoError(t, err)
	marker := models.InterruptedSessionMarker{
		IncarnationID: "incarnation-raw-interruption",
		InterruptedAt: "2026-11-25T10:00:00Z",
		Token:         "token-raw-interruption",
	}

	changed, err := repo.SetTaskInterruptedSessionIfNotArchived(ctx, casTaskID, "session-raw-interruption", marker)
	require.NoError(t, err)
	require.True(t, changed)
	requireRawInterruptionValues(t, repo, casTaskID)

	changed, err = repo.ResolveTaskInterruptedSession(ctx, casTaskID, "session-raw-interruption", marker.IncarnationID, marker.Token, "")
	require.NoError(t, err)
	require.True(t, changed)
	requireRawInterruptionValues(t, repo, casTaskID)

	marker.Token = "token-delete-interruption"
	changed, err = repo.SetTaskInterruptedSessionIfNotArchived(ctx, casTaskID, "session-raw-interruption", marker)
	require.NoError(t, err)
	require.True(t, changed)
	deleted, err := repo.DeleteTaskSessionWithAttachments(ctx, &models.TaskSession{
		ID: "session-raw-interruption", TaskID: casTaskID,
		QueueIncarnationID: "incarnation-raw-interruption",
	})
	require.NoError(t, err)
	require.Empty(t, deleted)
	requireRawInterruptionValues(t, repo, casTaskID)
}

func requireRawInterruptionValues(t *testing.T, repo *Repository, taskID string) {
	t.Helper()
	var raw string
	require.NoError(t, repo.DB().QueryRowContext(context.Background(), repo.db.Rebind(
		`SELECT metadata FROM tasks WHERE id=?`,
	), taskID).Scan(&raw))
	var values map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(raw), &values))
	require.Equal(t, json.RawMessage(`9007199254740993`), values["large"])
	require.Equal(t, json.RawMessage(`{"precise":9007199254740995,"text":"leave exactly"}`), values["nested"])
	require.Equal(t, json.RawMessage(`[1,2,3]`), values["unrelated"])
}

func TestDeleteTaskSessionWithMalformedInterruptedMetadata(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)
	session := &models.TaskSession{
		ID: "session-malformed-interruption", TaskID: casTaskID,
		QueueIncarnationID: "incarnation-malformed-interruption",
	}
	require.NoError(t, repo.CreateTaskSession(ctx, session))
	malformed := `{"interrupted_sessions":"legacy-malformed-value","interrupted_at":"2026-11-25T10:00:00Z","keep":true}`
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), malformed, casTaskID)
	require.NoError(t, err)

	_, err = repo.DeleteTaskSessionWithAttachments(ctx, session)
	require.NoError(t, err)
	_, err = repo.GetTaskSession(ctx, session.ID)
	require.Error(t, err)
	var stored string
	require.NoError(t, repo.DB().QueryRowContext(ctx, repo.db.Rebind(`SELECT metadata FROM tasks WHERE id=?`), casTaskID).Scan(&stored))
	require.JSONEq(t, malformed, stored)
}

func TestRemoveTaskInterruptedSessionReportsMalformedEntries(t *testing.T) {
	repo := newRepoForMetadataCASTests(t)
	ctx := context.Background()
	seedMetadataCASTask(t, repo, nil)
	malformed := `{"interrupted_sessions":"legacy-malformed-value","interrupted_at":"2026-11-25T10:00:00Z","keep":true}`
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), malformed, casTaskID)
	require.NoError(t, err)

	tx, err := repo.db.BeginTxx(ctx, nil)
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }()
	_, err = repo.removeTaskInterruptedSessionTx(ctx, tx, casTaskID, "session", "incarnation")
	require.ErrorIs(t, err, errMalformedTaskInterruptionEntries)
	require.ErrorContains(t, err, "decode task interruption entries")
}
