package sqlite

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/common/launchwrapper"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionSnapshotProjectionSQLite(t *testing.T) {
	runExecutionSnapshotProjection(t, newRepoForSessionTests(t))
}

func TestExecutionSnapshotProjectionPostgres(t *testing.T) {
	runExecutionSnapshotProjection(t, newPostgresRetentionTestRepo(t))
}

func runExecutionSnapshotProjection(t *testing.T, repo *Repository) {
	t.Helper()
	_, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	require.NoError(t, err)
	seedExecutorRunningCleanupTask(t, repo, "snapshot-task")
	seedExecutorRunningCleanupSession(t, repo, "snapshot-task", "snapshot-session", models.TaskSessionStateStarting, "execution")
	seedExecutorRunningCleanupSession(t, repo, "snapshot-task", "sibling-session", models.TaskSessionStateStarting, "sibling-execution")
	session, err := repo.GetTaskSession(t.Context(), "snapshot-session")
	require.NoError(t, err)
	require.Nil(t, session.ExecutionSnapshot, "legacy history must stay unknown")
	input := models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt", SessionID: session.ID, TaskID: session.TaskID, ExecutionID: "execution", ExecutorType: "local", Transport: "acp", Agent: launchwrapper.SummarizeCommand([]string{"agent"}), Sandbox: &models.ExecutionSandboxSnapshot{
		AttemptID: "attempt", Generation: 7, InstallationID: "installation", ProviderKey: "provider", ProviderVersion: "1.2.3", PackageDigest: strings.Repeat("a", 64), RuntimeVersion: "4.5.6", RuntimeDigest: strings.Repeat("b", 64), RevisionID: "revision", RevisionNumber: 2, PolicyName: "Original policy", PolicyDigest: strings.Repeat("c", 64), SelectionSource: "task", CeilingRevisionID: "ceiling", CeilingDigest: strings.Repeat("d", 64),
	}}
	captured, err := repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	input.Sandbox.PolicyName = "Later settings"
	session, err = repo.GetTaskSession(t.Context(), session.ID)
	require.NoError(t, err)
	require.Equal(t, captured, session.ExecutionSnapshot)
	require.Equal(t, captured, session.ToAPI()["execution_snapshot"])
	sessions, err := repo.ListTaskSessions(t.Context(), session.TaskID)
	require.NoError(t, err)
	for _, listed := range sessions {
		if listed.ID == session.ID {
			require.Equal(t, captured, listed.ExecutionSnapshot)
		} else {
			require.Nil(t, listed.ExecutionSnapshot)
		}
	}
	foreign := *captured
	foreign.SessionID = "foreign"
	foreignJSON, err := json.Marshal(foreign)
	require.NoError(t, err)
	private := *captured
	private.Agent.Executable = "/private/agent"
	privateJSON, err := json.Marshal(private)
	require.NoError(t, err)
	for _, corrupt := range []string{`{`, string(foreignJSON), string(privateJSON)} {
		_, err = repo.db.Exec(repo.db.Rebind(`UPDATE agent_execution_snapshots SET payload = ? WHERE attempt_id = ?`), corrupt, captured.AttemptID)
		require.NoError(t, err)
		session, err = repo.GetTaskSession(t.Context(), session.ID)
		require.NoError(t, err)
		require.Nil(t, session.ExecutionSnapshot)
	}
	raw, err := json.Marshal(captured)
	require.NoError(t, err)
	_, err = repo.db.Exec(repo.db.Rebind(`UPDATE agent_execution_snapshots SET payload = ? WHERE attempt_id = ?`), string(raw), captured.AttemptID)
	require.NoError(t, err)
	require.NoError(t, repo.DeleteTaskSession(t.Context(), session))
	var count int
	require.NoError(t, repo.db.QueryRow(`SELECT COUNT(*) FROM agent_execution_snapshots`).Scan(&count))
	require.Zero(t, count)
}
