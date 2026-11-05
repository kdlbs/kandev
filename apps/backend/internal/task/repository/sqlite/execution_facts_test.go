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
	value.Facts.Audit = contract.AuditFacts{Integrity: "valid", Finalization: "final", Signer: "unsigned", Verification: &contract.AuditVerificationFact{ID: "a89ae798-dce6-4617-956b-5485a4eb6a87", CompletedAt: now.Add(-time.Hour), ArtifactSetDigest: snapshot.Sandbox.RuntimeDigest, VerifierVersion: "fixture/1", SignatureState: "unsigned", LedgerState: "valid"}}
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
	require.Equal(t, value.Facts.Audit, read.Facts.Audit)
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
	require.Equal(t, value.Facts.Audit, old.Facts.Audit, "legacy refresh cannot erase a completed audit")
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
	require.Equal(t, value.Facts.Audit, history.Facts.Audit, "backend replacement preserves the original audit time")
	require.True(t, history.ValidFields())
	assertAuditTrustRefreshFailure(t, repo, snapshot, now)
}

func assertAuditTrustRefreshFailure(t *testing.T, repo *Repository, snapshot *models.ExecutionSnapshot, completed time.Time) {
	t.Helper()
	audit := contract.AuditFacts{Integrity: "valid", Finalization: "final", Signer: "trusted", Verification: &contract.AuditVerificationFact{ID: "a89ae798-dce6-4617-956b-5485a4eb6a87", CompletedAt: completed, ArtifactSetDigest: snapshot.Sandbox.RuntimeDigest, VerifierVersion: "fixture/1", SignatureState: "verified", LedgerState: "valid", TrustRevision: "8734165f-cdcd-43de-847b-0eb54152194f"}}
	value := contract.Observation{AttemptID: snapshot.AttemptID, Generation: 7, AttemptVersion: 3, Status: "unknown", ProviderHealth: "unavailable", CheckedAt: time.Now().UTC(), Facts: &contract.ExecutionFacts{SchemaVersion: 1, Process: contract.ProcessFact{State: "unknown"}, LaunchChecks: contract.LaunchChecksFact{State: "unavailable"}, Runtime: contract.RuntimeFacts{State: "unavailable"}, Audit: audit}}
	_, err := repo.RecordExecutionObservation(t.Context(), "second", snapshot, value)
	require.NoError(t, err)
	value.CheckedAt = time.Now().UTC()
	value.Facts = nil
	failed, err := repo.RecordExecutionObservation(t.Context(), "second", snapshot, value)
	require.NoError(t, err)
	require.Equal(t, "untrusted", failed.Facts.Audit.Signer, "failed refresh cannot renew current trust")
	require.Equal(t, audit.Verification, failed.Facts.Audit.Verification, "failed refresh preserves original signature evidence")
}

func marshalObservation(t *testing.T, value *models.ExecutionObservation) string {
	t.Helper()
	raw, err := json.Marshal(value)
	require.NoError(t, err)
	return string(raw)
}
