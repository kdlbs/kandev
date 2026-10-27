package sqlite

import (
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestExecutionObservationCandidatesSQLite(t *testing.T) {
	runObservationCandidates(t, newRepoForSessionTests(t))
}
func TestExecutionObservationCandidatesPostgres(t *testing.T) {
	runObservationCandidates(t, newPostgresRetentionTestRepo(t))
}

func runObservationCandidates(t *testing.T, repo *Repository) {
	t.Helper()
	_, err := messagequeue.NewSQLiteRepository(repo.db, repo.db)
	require.NoError(t, err)
	require.NoError(t, repo.CreateWorkspace(t.Context(), &models.Workspace{ID: "ws-task", Name: "Workspace", OrgID: "tenant"}))
	require.NoError(t, repo.CreateTask(t.Context(), &models.Task{ID: "task", Title: "Task", WorkspaceID: "ws-task"}))
	snapshots := make([]*models.ExecutionSnapshot, 0, 3)
	for _, id := range []string{"a", "b", "c"} {
		seedExecutorRunningCleanupSession(t, repo, "task", id, models.TaskSessionStateRunning, "execution")
		input := observationSnapshotInput()
		input.SessionID, input.AttemptID, input.Sandbox.AttemptID = id, id, id
		if id == "c" {
			input.Sandbox = nil
		}
		snapshot, captureErr := repo.CaptureExecutionSnapshot(t.Context(), input)
		require.NoError(t, captureErr)
		snapshots = append(snapshots, snapshot)
	}
	first, err := repo.ListExecutionObservationCandidates(t.Context(), "", 1)
	require.NoError(t, err)
	require.Len(t, first, 1)
	require.Equal(t, snapshots[0], first[0].Snapshot)
	require.Equal(t, "tenant", first[0].OrgID)
	second, err := repo.ListExecutionObservationCandidates(t.Context(), first[0].Snapshot.AttemptID, 1)
	require.NoError(t, err)
	require.Len(t, second, 1)
	require.Equal(t, snapshots[1], second[0].Snapshot)
	empty, err := repo.ListExecutionObservationCandidates(t.Context(), "b", 1)
	require.NoError(t, err)
	require.Empty(t, empty, "core wrappers must not be sent to the sandbox observer")
	_, err = repo.ClaimExecutionObserver(t.Context(), "owner")
	require.NoError(t, err)
	_, err = repo.RecordExecutionObservation(t.Context(), "owner", snapshots[0], contract.Observation{
		AttemptID: "a", Generation: 7, AttemptVersion: 4, Status: "stopped", ProviderHealth: "unknown", CheckedAt: time.Now().UTC(),
	})
	require.NoError(t, err)
	input := observationSnapshotInput()
	input.SessionID, input.AttemptID, input.Sandbox.AttemptID = "b", "d", "d"
	latest, err := repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	remaining, err := repo.ListExecutionObservationCandidates(t.Context(), "", 100)
	require.NoError(t, err)
	require.Len(t, remaining, 1, "only the latest nonterminal selected attempt is eligible")
	require.Equal(t, latest, remaining[0].Snapshot)
	session, err := repo.GetTaskSession(t.Context(), "b")
	require.NoError(t, err)
	require.NoError(t, repo.DeleteTaskSession(t.Context(), session))
	remaining, err = repo.ListExecutionObservationCandidates(t.Context(), "", 100)
	require.NoError(t, err)
	require.Empty(t, remaining)
	var rows int
	require.NoError(t, repo.db.QueryRow(`SELECT COUNT(*) FROM agent_execution_observations`).Scan(&rows))
	require.Equal(t, 1, rows, "session deletion must cascade both pending and persisted observations")
	for _, limit := range []int{0, -1, 101} {
		_, err = repo.ListExecutionObservationCandidates(t.Context(), "", limit)
		require.Error(t, err)
	}
}
