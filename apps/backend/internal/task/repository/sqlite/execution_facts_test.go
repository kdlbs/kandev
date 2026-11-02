package sqlite

import (
	"encoding/json"
	"testing"
	"time"

	contract "github.com/kandev/kandev/internal/common/sandbox"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

func TestExecutionFactsSQLite(t *testing.T)   { runExecutionFacts(t, newRepoForSessionTests(t)) }
func TestExecutionFactsPostgres(t *testing.T) { runExecutionFacts(t, newPostgresRetentionTestRepo(t)) }

func runExecutionFacts(t *testing.T, repo *Repository) {
	seedExecutorRunningCleanupTask(t, repo, "task")
	seedExecutorRunningCleanupSession(t, repo, "task", "session", models.TaskSessionStateRunning, "execution")
	snapshot, err := repo.CaptureExecutionSnapshot(t.Context(), observationSnapshotInput())
	require.NoError(t, err)
	_, err = repo.ClaimExecutionObserver(t.Context(), "first")
	require.NoError(t, err)
	now := time.Now().UTC().Truncate(time.Millisecond)
	until := now.Add(30 * time.Second)
	receipt := &contract.LaunchCheckReceipt{CheckedAt: now, PolicyDigest: snapshot.Sandbox.PolicyDigest, PackageDigest: snapshot.Sandbox.PackageDigest, RuntimeDigest: snapshot.Sandbox.RuntimeDigest, Checks: []string{"policy_ceiling", "runtime_checksum", "private_artifacts", "process_containment"}}
	value := contract.Observation{AttemptID: snapshot.AttemptID, Generation: 7, AttemptVersion: 3, Status: "unknown", ProviderHealth: "unavailable", Reason: "provider_unavailable", CheckedAt: now, Facts: &contract.ExecutionFacts{SchemaVersion: 1, Process: contract.ProcessFact{State: "running", CheckedAt: &now, ValidUntil: &until}, LaunchChecks: contract.LaunchChecksFact{State: "passed", Receipt: receipt}, Runtime: contract.RuntimeFacts{State: "reported", Observations: []contract.RuntimeObservation{{SourceID: "runtime.v2", EvidenceDigest: snapshot.Sandbox.RuntimeDigest, ObservedAt: now, Platform: "linux"}}}, Audit: contract.AuditFacts{Integrity: "unchecked", Finalization: "unknown", Signer: "unchecked"}}}
	invalid := value
	invalidFacts := *value.Facts
	invalidReceipt := *receipt
	invalidReceipt.PolicyDigest = snapshot.Sandbox.RuntimeDigest
	invalidFacts.LaunchChecks.Receipt = &invalidReceipt
	invalid.Facts = &invalidFacts
	_, err = repo.RecordExecutionObservation(t.Context(), "first", snapshot, invalid)
	require.Error(t, err, "launch receipt must match the captured policy, package, and runtime")
	first, err := repo.RecordExecutionObservation(t.Context(), "first", snapshot, value)
	require.NoError(t, err)
	read, err := readSessionObservation(t, repo, "session")
	require.NoError(t, err)
	require.Equal(t, first, read)
	refresh := value
	refreshedFacts := *value.Facts
	refreshedFacts.Process = contract.ProcessFact{State: contract.StatusUnknown}
	refreshedFacts.Runtime = contract.RuntimeFacts{State: contract.FactUnavailable}
	refresh.Facts = &refreshedFacts
	refresh.CheckedAt = time.Now().UTC().Truncate(time.Millisecond)
	updated, err := repo.RecordExecutionObservation(t.Context(), "first", snapshot, refresh)
	require.NoError(t, err)
	require.Equal(t, value.Facts.Runtime.Observations, updated.Facts.Runtime.Observations, "failed refresh preserves original historical metadata")
	require.Equal(t, receipt, updated.Facts.LaunchChecks.Receipt)
	legacy := refresh
	legacy.CheckedAt = time.Now().UTC().Truncate(time.Millisecond)
	legacy.Facts = nil
	old, err := repo.RecordExecutionObservation(t.Context(), "first", snapshot, legacy)
	require.NoError(t, err)
	require.Equal(t, "unknown", old.Facts.Process.State, "a complete legacy observation cannot reuse live facts")
	require.Equal(t, receipt, old.Facts.LaunchChecks.Receipt)
	expired := decodeExecutionObservation(marshalObservation(t, first), snapshot, first.ObserverGeneration, until)
	require.Equal(t, "unknown", expired.Facts.Process.State)
	require.Nil(t, expired.Facts.Process.ValidUntil)
	require.Equal(t, receipt, expired.Facts.LaunchChecks.Receipt)
	_, err = repo.ClaimExecutionObserver(t.Context(), "second")
	require.NoError(t, err)
	history, err := readSessionObservation(t, repo, "session")
	require.NoError(t, err)
	require.Equal(t, "unknown", history.Facts.Process.State)
	require.Equal(t, "unavailable", history.Facts.Runtime.State)
	require.Equal(t, receipt, history.Facts.LaunchChecks.Receipt)
	require.Equal(t, now, history.Facts.Runtime.Observations[0].ObservedAt)
	require.True(t, history.ValidFields())
}

func marshalObservation(t *testing.T, value *models.ExecutionObservation) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}
