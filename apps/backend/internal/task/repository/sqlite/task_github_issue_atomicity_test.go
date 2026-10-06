package sqlite

import (
	"context"
	"database/sql"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

// @covers AC-TASKS-LINK-EXISTING-TASK-GITHUB-ISSUE-001.5
func TestTaskGitHubIssueSQLiteAtomicity(t *testing.T) {
	runTaskGitHubIssueAtomicity(t, newRepoForEntityTests(t), false)
}
func TestTaskGitHubIssuePostgresAtomicity(t *testing.T) {
	database := testutil.OpenIsolatedPostgres(t, testutil.PostgresDSNFromEnv(t))
	runTaskGitHubIssueAtomicity(t, newPostgresMetadataCASRepo(t, database), true)
}
func runTaskGitHubIssueAtomicity(t *testing.T, repo *Repository, postgres bool) {
	t.Helper()
	ctx := context.Background()
	id := "issue-atomic"
	require.NoError(t, repo.CreateWorkspace(ctx, &models.Workspace{ID: "atomic-ws", Name: "Atomic"}))
	require.NoError(t, repo.CreateTask(ctx, &models.Task{ID: id, WorkspaceID: "atomic-ws", Title: "Untouched", Metadata: map[string]interface{}{"keep": true}}))
	link := &models.TaskGitHubIssueLink{URL: "https://github.com/acme/api/issues/42", Number: 42, Owner: "acme", Repo: "api"}
	for _, raw := range []string{`{"bad":`, `[]`, `"text"`} {
		_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), raw, id)
		require.NoError(t, err)
		assertIssueWriteFailure(t, repo, id, link)
		assertIssueWriteFailure(t, repo, id, nil)
	}
	_, err := repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET metadata=? WHERE id=?`), `{"keep":true}`, id)
	require.NoError(t, err)
	reject := `CREATE TRIGGER reject_issue BEFORE UPDATE OF metadata ON tasks BEGIN SELECT RAISE(ABORT,'issue rejected'); END`
	if postgres {
		_, err = repo.DB().Exec(`CREATE FUNCTION reject_issue() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'issue rejected'; END; $$`)
		require.NoError(t, err)
		reject = `CREATE TRIGGER reject_issue BEFORE UPDATE OF metadata ON tasks FOR EACH ROW EXECUTE FUNCTION reject_issue()`
	}
	_, err = repo.DB().Exec(reject)
	require.NoError(t, err)
	assertIssueWriteFailure(t, repo, id, link)
	assertIssueWriteFailure(t, repo, id, nil)
	drop := `DROP TRIGGER reject_issue`
	if postgres {
		drop += ` ON tasks`
	}
	_, err = repo.DB().Exec(drop)
	require.NoError(t, err)
	// Reject the candidate observation after the UPDATE; the transaction must roll back.
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET workflow_agent_overrides=? WHERE id=?`), `{"bad":`, id)
	require.NoError(t, err)
	assertIssueWriteFailure(t, repo, id, link)
	_, err = repo.DB().ExecContext(ctx, repo.db.Rebind(`UPDATE tasks SET workflow_agent_overrides=? WHERE id=?`), nil, id)
	require.NoError(t, err)
	_, err = repo.UpdateTaskGitHubIssue(ctx, "missing", link)
	require.ErrorIs(t, err, ErrTaskNotFound)
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	_, err = repo.UpdateTaskGitHubIssue(cancelled, id, link)
	require.ErrorIs(t, err, context.Canceled)
	_, err = repo.UpdateTaskGitHubIssue(ctx, id, link)
	require.NoError(t, err)
	stored, err := repo.GetTask(ctx, id)
	require.NoError(t, err)
	require.Equal(t, float64(42), stored.Metadata["issue_number"])
}
func assertIssueWriteFailure(t *testing.T, repo *Repository, id string, link *models.TaskGitHubIssueLink) {
	t.Helper()
	var before, after sql.NullString
	var timestamp, actual time.Time
	query := repo.db.Rebind(`SELECT metadata,updated_at FROM tasks WHERE id=?`)
	require.NoError(t, repo.DB().QueryRow(query, id).Scan(&before, &timestamp))
	candidate, err := repo.UpdateTaskGitHubIssue(context.Background(), id, link)
	require.Error(t, err)
	require.Nil(t, candidate)
	require.NoError(t, repo.DB().QueryRow(query, id).Scan(&after, &actual))
	require.Equal(t, before, after)
	require.Equal(t, timestamp, actual)
}
