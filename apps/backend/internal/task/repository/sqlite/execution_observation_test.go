package sqlite

import (
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/launchwrapper"
	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionObservationSQLite(t *testing.T) {
	runExecutionObservation(t, newRepoForSessionTests(t))
}
func TestExecutionObservationPostgres(t *testing.T) {
	runExecutionObservation(t, newPostgresRetentionTestRepo(t))
}

func runExecutionObservation(t *testing.T, repo *Repository) {
	t.Helper()
	seedExecutorRunningCleanupTask(t, repo, "task")
	seedExecutorRunningCleanupSession(t, repo, "task", "session", models.TaskSessionStateRunning, "execution")
	input := observationSnapshotInput()
	snapshot, err := repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	epoch, err := repo.ClaimExecutionObserver(t.Context(), "owner-one")
	require.NoError(t, err)
	require.Equal(t, uint64(1), epoch)
	replayed, err := repo.ClaimExecutionObserver(t.Context(), "owner-one")
	require.NoError(t, err)
	require.Equal(t, epoch, replayed, "same owner claim must be idempotent")
	now := time.Now().UTC().Truncate(time.Millisecond)
	evidenceAt := now.Add(-time.Second)
	until := evidenceAt.Add(30 * time.Second)
	active := contract.Observation{AttemptID: "attempt", Generation: 7, AttemptVersion: 3, Status: "active", ProviderHealth: "available", Mechanism: "fixture-kernel", CheckedAt: now, EvidenceAt: &evidenceAt, ValidUntil: &until}
	for _, mutation := range []func(*contract.Observation){
		func(v *contract.Observation) { v.AttemptID = "foreign" },
		func(v *contract.Observation) { v.Generation++ },
		func(v *contract.Observation) { v.Status = "private-error" },
		func(v *contract.Observation) { v.ProviderHealth = "/private/provider" },
		func(v *contract.Observation) { v.Reason = "secret-value" },
		func(v *contract.Observation) { v.EvidenceAt = nil },
		func(v *contract.Observation) { v.ValidUntil = nil },
		func(v *contract.Observation) { v.Mechanism = "/private/path" },
	} {
		invalid := active
		mutation(&invalid)
		_, err = repo.RecordExecutionObservation(t.Context(), "owner-one", snapshot, invalid)
		require.Error(t, err)
	}
	first, err := repo.RecordExecutionObservation(t.Context(), "owner-one", snapshot, active)
	require.NoError(t, err)
	require.Equal(t, uint64(1), first.Revision)
	require.Equal(t, epoch, first.ObserverGeneration)
	read, err := readSessionObservation(t, repo, "session")
	require.NoError(t, err)
	require.Equal(t, first, read)
	session, err := repo.GetTaskSession(t.Context(), "session")
	require.NoError(t, err)
	require.Equal(t, first, session.ExecutionObservation)
	require.Equal(t, first, session.ToAPI()["execution_observation"])
	sessions, err := repo.ListTaskSessions(t.Context(), "task")
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.Equal(t, first, sessions[0].ExecutionObservation)

	stopped := active
	stopped.Status = "stopped"
	stopped.AttemptVersion = 4
	stopped.EvidenceAt = nil
	stopped.ValidUntil = nil
	stopped.Mechanism = ""
	terminal, err := repo.RecordExecutionObservation(t.Context(), "owner-one", snapshot, stopped)
	require.NoError(t, err)
	require.Equal(t, "stopped", terminal.Status)
	require.Equal(t, first.EvidenceAt, terminal.EvidenceAt, "last evidence survives for history")
	require.Nil(t, terminal.ValidUntil)
	for _, version := range []int64{3, 5} {
		late := active
		late.AttemptVersion = version
		_, err = repo.RecordExecutionObservation(t.Context(), "owner-one", snapshot, late)
		require.Error(t, err, "terminal attempt cannot become active")
	}
	input.AttemptID = "next"
	input.Sandbox.AttemptID = "next"
	input.Sandbox.Generation++
	next, err := repo.CaptureExecutionSnapshot(t.Context(), input)
	require.NoError(t, err)
	_, err = repo.RecordExecutionObservation(t.Context(), "owner-one", snapshot, stopped)
	require.Error(t, err, "old attempt cannot update the session projection")
	active.AttemptID = "next"
	active.Generation++
	current, err := repo.RecordExecutionObservation(t.Context(), "owner-one", next, active)
	require.NoError(t, err)
	newEpoch, err := repo.ClaimExecutionObserver(t.Context(), "owner-two")
	require.NoError(t, err)
	require.Greater(t, newEpoch, epoch)
	historical, err := readSessionObservation(t, repo, "session")
	require.NoError(t, err)
	require.Equal(t, "unknown", historical.Status, "backend replacement must not restore active status")
	require.Equal(t, newEpoch, historical.ObserverGeneration)
	require.Equal(t, current.EvidenceAt, historical.EvidenceAt)
	require.Nil(t, historical.ValidUntil)
	_, err = repo.RecordExecutionObservation(t.Context(), "owner-one", next, active)
	require.Error(t, err, "late work from the prior backend must be fenced")
	fresh, err := repo.RecordExecutionObservation(t.Context(), "owner-two", next, active)
	require.NoError(t, err)
	require.Equal(t, "active", fresh.Status)
	require.Equal(t, newEpoch, fresh.ObserverGeneration)
	require.Greater(t, fresh.Revision, current.Revision)

}

func readSessionObservation(t *testing.T, repo *Repository, id string) (*models.ExecutionObservation, error) {
	t.Helper()
	session, err := repo.GetTaskSession(t.Context(), id)
	if err != nil {
		return nil, err
	}
	return session.ExecutionObservation, nil
}

func observationSnapshotInput() models.ExecutionSnapshot {
	return models.ExecutionSnapshot{SchemaVersion: 1, AttemptID: "attempt", SessionID: "session", TaskID: "task", ExecutionID: "execution", Agent: launchwrapper.SummarizeCommand([]string{"agent"}), Sandbox: &models.ExecutionSandboxSnapshot{
		AttemptID: "attempt", Generation: 7, InstallationID: "installation", ProviderKey: "fixture", ProviderVersion: "1", PackageDigest: strings.Repeat("a", 64), RuntimeVersion: "1", RuntimeDigest: strings.Repeat("b", 64), RevisionID: "revision", RevisionNumber: 1, PolicyName: "Policy", PolicyDigest: strings.Repeat("c", 64), SelectionSource: "task", CeilingRevisionID: "ceiling", CeilingDigest: strings.Repeat("d", 64),
	}}
}
