package lifecycle

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
	_ "github.com/mattn/go-sqlite3"
)

func decodeFixturePrunedMarker(raw json.RawMessage, target *map[string]any) error {
	if err := json.Unmarshal(raw, target); err == nil && *target != nil {
		return nil
	}
	var encoded string
	if err := json.Unmarshal(raw, &encoded); err != nil {
		return err
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, target)
}

func repairPrunedIntentMetadataCAS(ctx context.Context, db *sqlx.DB, row fixtureOperatorRow, input prunedIntentCleanupInput, encoded string) error {
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return errors.New("begin exact metadata repair transaction")
	}
	defer func() { _ = tx.Rollback() }()
	var currentMetadata, currentUpdated string
	if err := tx.QueryRowxContext(ctx, `SELECT metadata, CAST(updated_at AS TEXT) FROM executors_running WHERE id=? AND session_id=? AND task_id=? AND agent_execution_id=? AND executor_id=? AND runtime='nomad' AND status='stopped'`, row.ID, row.SessionID, row.TaskID, row.ExecutionID, row.ExecutorID).Scan(&currentMetadata, &currentUpdated); err != nil || currentMetadata != row.Metadata || currentUpdated != row.UpdatedAt {
		return errors.New("cleanup row changed since exact identity capture")
	}
	var currentOwner string
	if err := tx.QueryRowxContext(ctx, `SELECT executor_id || ':' || executor_profile_id FROM task_environments WHERE id=? AND task_id=? AND executor_type='nomad' AND ownership_generation=? AND status='creating'`, input.EnvironmentID, input.TaskID, input.Generation).Scan(&currentOwner); err != nil || currentOwner != input.ExecutorID+":"+input.ExecutorProfileID {
		return errors.New("environment owner changed during metadata repair")
	}
	result, err := tx.ExecContext(ctx, `UPDATE executors_running SET metadata=?, updated_at=? WHERE id=? AND session_id=? AND task_id=? AND agent_execution_id=? AND executor_id=? AND runtime='nomad' AND status='stopped' AND updated_at=? AND metadata=?`, encoded, time.Now().UTC(), row.ID, row.SessionID, row.TaskID, row.ExecutionID, row.ExecutorID, row.UpdatedAt, row.Metadata)
	if err != nil {
		return errors.New("CAS corrected cleanup metadata")
	}
	if affected, err := result.RowsAffected(); err != nil || affected != 1 {
		return errors.New("exact metadata repair CAS did not affect one row")
	}
	if err := tx.Commit(); err != nil {
		return errors.New("commit corrected cleanup metadata")
	}
	return nil
}

func decodeFixtureSecretReference(raw json.RawMessage) (string, error) {
	var ref string
	if err := json.Unmarshal(raw, &ref); err != nil {
		return "", err
	}
	if ref != "" && !strings.Contains(ref, "***") && (strings.HasPrefix(ref, "$kandev-secret:") || strings.HasPrefix(ref, "kandev-runtime:")) {
		return ref, nil
	}
	decoded, err := base64.StdEncoding.DecodeString(ref)
	if err != nil {
		return "", err
	}
	if err := json.Unmarshal(decoded, &ref); err != nil {
		return "", err
	}
	if ref == "" || strings.Contains(ref, "***") {
		return "", errors.New("decoded reference is not an original SecretStore reference")
	}
	return ref, nil
}

func assertPrunedRepairSnapshotRow(ctx context.Context, input prunedIntentCleanupInput, prior, repaired fixtureOperatorRow) error {
	snapshot, err := sqlx.Open("sqlite3", "file:"+input.RepairSnapshotPath+"?mode=ro&_query_only=1")
	if err != nil {
		return err
	}
	defer func() { _ = snapshot.Close() }()
	before, err := readFixtureOperatorRow(ctx, snapshot, input.TaskID)
	if err != nil {
		return err
	}
	var encoded map[string]json.RawMessage
	if err := json.Unmarshal([]byte(before.Metadata), &encoded); err != nil {
		return err
	}
	var marker map[string]any
	if err := decodeFixturePrunedMarker(encoded["fixture_operator_pruned_restore"], &marker); err != nil {
		return err
	}
	if err := verifyFixtureCleanupClaim(input, marker); err != nil {
		return err
	}
	wantAuthRef := kubernetesRuntimeSecretID(input.ExecutionID, "agentctl-auth")
	authRef, err := decodeFixtureSecretReference(encoded[MetadataKeyAuthTokenSecret])
	if err != nil || authRef != wantAuthRef {
		return errors.New("snapshot auth reference is not original operation reference")
	}
	if before.ID != prior.ID || before.SessionID != prior.SessionID || before.TaskID != prior.TaskID || before.ExecutionID != prior.ExecutionID || before.ExecutorID != prior.ExecutorID || before.Runtime != prior.Runtime || before.Status != prior.Status || before.UpdatedAt != prior.UpdatedAt || before.Metadata != prior.Metadata {
		return errors.New("snapshot row identity differs from repaired row")
	}
	if repaired.ID != prior.ID || repaired.SessionID != prior.SessionID || repaired.TaskID != prior.TaskID || repaired.ExecutionID != prior.ExecutionID || repaired.ExecutorID != prior.ExecutorID || repaired.Runtime != prior.Runtime || repaired.Status != prior.Status {
		return errors.New("repaired row identity differs from private pre-repair snapshot")
	}
	return nil
}

func buildFixturePrunedCleanupIdentity(t *testing.T, input prunedIntentCleanupInput, row fixtureOperatorRow, state nomad.EnvironmentState) (string, string) {
	t.Helper()
	_ = row
	_ = state
	configPath := filepath.Join(input.FixtureRoot, "evidence", "early3-cleanup-session-config-repaired.json")
	manifestPath := filepath.Join(input.FixtureRoot, "evidence", "early3-cleanup-identity-repaired.json")
	return manifestPath, configPath
}

func fixtureIdentityManifestForPruned(input prunedIntentCleanupInput, row fixtureOperatorRow, state nomad.EnvironmentState, configPath string) fixtureIdentityManifest {
	metadataDigest := sha256.Sum256([]byte(row.Metadata))
	return fixtureIdentityManifest{
		SchemaVersion: 1, FixtureRoot: input.FixtureRoot, DatabasePath: input.DatabasePath,
		SessionConfigPath: configPath, ExpectedSessionState: "CREATED",
		ExpectedRow: fixtureIdentityExpectedRow{ID: row.ID, TaskID: row.TaskID, SessionID: row.SessionID, ExecutionID: row.ExecutionID,
			ExecutorID: row.ExecutorID, Runtime: row.Runtime, Status: row.Status, UpdatedAt: row.UpdatedAt,
			MetadataSHA256: hex.EncodeToString(metadataDigest[:])},
		ExpectedNomad: fixtureIdentityExpectedNomad{Phase: state.Phase, Revision: state.Revision, EnvironmentID: state.EnvironmentID,
			Generation: state.Generation, OperationID: state.OperationID, JobID: state.JobID, AllocationID: state.AllocationID,
			AllocationCreateIndex: state.AllocationCreateIndex, NodeID: state.NodeID, WorkspacePath: state.WorkspacePath,
			ServerIdentity: state.ServerIdentity},
		ExpectedProcess: fixtureIdentityExpectedProc{AgentctlPID: input.AgentctlPID, AgentctlStart: input.AgentctlStart, AgentctlRuntimePath: input.AgentctlRuntimePath},
	}
}

type prunedIntentCleanupInput struct {
	SchemaVersion        int    `json:"schema_version"`
	FixtureOnly          bool   `json:"fixture_only"`
	FixtureRoot          string `json:"fixture_root"`
	DatabasePath         string `json:"database_path"`
	SnapshotPath         string `json:"snapshot_path"`
	RepairSnapshotPath   string `json:"repair_snapshot_path"`
	RepairSnapshotSHA256 string `json:"repair_snapshot_sha256"`
	ProfileSourcePath    string `json:"profile_source_path"`
	ProfileSourceSHA256  string `json:"profile_source_sha256"`
	ProfileConfigSHA256  string `json:"profile_config_sha256"`
	SnapshotConfigSHA256 string `json:"snapshot_profile_config_sha256"`
	CurrentConfigSHA256  string `json:"current_profile_config_sha256"`
	SessionConfigPath    string `json:"session_config_path"`
	TaskID               string `json:"task_id"`
	SessionID            string `json:"session_id"`
	ExecutionID          string `json:"execution_id"`
	ExecutorID           string `json:"executor_id"`
	ExecutorProfileID    string `json:"executor_profile_id"`
	EnvironmentID        string `json:"environment_id"`
	Generation           int64  `json:"generation"`
	ExpectedJobID        string `json:"job_id"`
	ExpectedAllocationID string `json:"allocation_id"`
	NodeID               string `json:"node_id"`
	WorkspacePath        string `json:"workspace_path"`
	AgentctlPID          int    `json:"agentctl_pid"`
	AgentctlStart        string `json:"agentctl_start"`
	AgentctlRuntimePath  string `json:"agentctl_runtime_path"`
	BootstrapSecretID    string `json:"bootstrap_secret_id"`
}

func readPrunedIntentInput(t *testing.T, path string) prunedIntentCleanupInput {
	t.Helper()
	if !filepath.IsAbs(path) {
		t.Fatal("pruned-intent manifest path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		t.Fatal("pruned-intent manifest must be a private regular file")
	}
	var input prunedIntentCleanupInput
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, &input) != nil || input.SchemaVersion != 1 || !input.FixtureOnly {
		t.Fatal("pruned-intent manifest is invalid")
	}
	if input.FixtureRoot == "" || input.DatabasePath == "" || input.SnapshotPath == "" || input.RepairSnapshotPath == "" || input.ProfileSourcePath == "" ||
		input.RepairSnapshotSHA256 == "" || input.ProfileSourceSHA256 == "" || input.ProfileConfigSHA256 == "" || input.SnapshotConfigSHA256 == "" || input.CurrentConfigSHA256 == "" ||
		input.SessionConfigPath == "" || input.TaskID == "" || input.SessionID == "" || input.ExecutionID == "" ||
		input.ExecutorID == "" || input.ExecutorProfileID == "" || input.EnvironmentID == "" || input.Generation < 1 ||
		input.ExpectedJobID == "" || input.ExpectedAllocationID == "" || input.NodeID == "" || input.WorkspacePath == "" ||
		input.AgentctlPID <= 1 || input.AgentctlStart == "" || input.AgentctlRuntimePath == "" || input.BootstrapSecretID == "" {
		t.Fatal("pruned-intent manifest lacks exact ownership evidence")
	}
	for _, path := range []string{input.DatabasePath, input.SnapshotPath, input.RepairSnapshotPath, input.ProfileSourcePath, input.SessionConfigPath} {
		rel, err := filepath.Rel(filepath.Clean(input.FixtureRoot), filepath.Clean(path))
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			t.Fatal("pruned-intent manifest contains a path outside its fixture root")
		}
	}
	return input
}

func verifyPrunedRepairSnapshotDigest(t *testing.T, input prunedIntentCleanupInput) {
	t.Helper()
	data, err := os.ReadFile(input.RepairSnapshotPath)
	if err != nil {
		t.Fatal("read private pre-repair snapshot")
	}
	digest := sha256.Sum256(data)
	if hex.EncodeToString(digest[:]) != input.RepairSnapshotSHA256 {
		t.Fatal("private pre-repair snapshot digest differs from the captured manifest")
	}
}

func verifyPrunedProfileProvenance(ctx context.Context, current *sqlx.DB, input prunedIntentCleanupInput, source struct {
	SchemaVersion int               `json:"schema_version"`
	ProfileID     string            `json:"profile_id"`
	ExecutorID    string            `json:"executor_id"`
	Config        map[string]string `json:"config"`
	NomadTokenRef string            `json:"nomad_token_ref"`
}) error {
	if source.ProfileID != input.ExecutorProfileID || source.ExecutorID != input.ExecutorID || source.Config["nomad_token"] != source.NomadTokenRef {
		return errors.New("profile source identity or secret reference does not match the manifest")
	}
	profileKeys := map[string]bool{"address": true, "namespace": true, "region": true, "datacenter": true, "node_id": true, "platform": true, "ssh_target": true, "ssh_port": true, "ssh_host_fingerprint": true, "workspace_root": true, "allocation_root": true, "cpu_mhz": true, "memory_mb": true, "nomad_token": true}
	for key, value := range source.Config {
		if !profileKeys[key] || value == "" {
			return errors.New("profile source contains an unsupported or empty config field")
		}
	}
	if len(source.Config) != len(profileKeys) {
		return errors.New("profile source does not contain the complete allowlisted profile")
	}
	configDigest := func(raw string) (string, error) {
		var config map[string]string
		if err := json.Unmarshal([]byte(raw), &config); err != nil || len(config) != len(profileKeys) {
			return "", errors.New("canonical profile config is not a complete string map")
		}
		for key, value := range config {
			if !profileKeys[key] || value == "" {
				return "", errors.New("canonical profile config has unsupported or empty fields")
			}
		}
		encoded, err := json.Marshal(config)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(encoded)
		return hex.EncodeToString(digest[:]), nil
	}
	encodedSource, err := json.Marshal(source.Config)
	if err != nil {
		return err
	}
	sourceDigest := sha256.Sum256(encodedSource)
	if hex.EncodeToString(sourceDigest[:]) != input.ProfileConfigSHA256 {
		return errors.New("profile source config checksum does not match manifest")
	}
	readProfile := func(db *sqlx.DB) (string, string, error) {
		var executorID, config string
		if err := db.QueryRowxContext(ctx, `SELECT executor_id, config FROM executor_profiles WHERE id = ?`, input.ExecutorProfileID).Scan(&executorID, &config); err != nil {
			return "", "", err
		}
		return executorID, config, nil
	}
	currentExecutor, currentJSON, err := readProfile(current)
	if err != nil || currentExecutor != input.ExecutorID {
		return errors.New("current profile owner does not match manifest")
	}
	currentDigest, err := configDigest(currentJSON)
	if err != nil {
		return err
	}
	snapshot, err := sqlx.Open("sqlite3", "file:"+input.SnapshotPath+"?mode=ro&_query_only=1")
	if err != nil {
		return errors.New("open captured profile snapshot read-only")
	}
	defer func() { _ = snapshot.Close() }()
	snapshotExecutor, snapshotJSON, err := readProfile(snapshot)
	if err != nil || snapshotExecutor != input.ExecutorID {
		return errors.New("snapshot profile owner does not match manifest")
	}
	snapshotDigest, err := configDigest(snapshotJSON)
	if err != nil {
		return err
	}
	if currentDigest != input.CurrentConfigSHA256 || snapshotDigest != input.SnapshotConfigSHA256 || currentDigest != snapshotDigest || currentDigest != input.ProfileConfigSHA256 {
		return errors.New("profile config digest differs across source, snapshot, or current database")
	}
	var canonicalSource map[string]string
	if err := json.Unmarshal([]byte(currentJSON), &canonicalSource); err != nil {
		return err
	}
	if len(canonicalSource) != len(source.Config) {
		return errors.New("profile source field count differs from canonical row")
	}
	for key, value := range canonicalSource {
		if source.Config[key] != value {
			return errors.New("profile source field differs from canonical profile row")
		}
	}
	return nil
}

func prunedIntentState(profile nomad.ProfileConfig, job *nomad.JobRecord, input prunedIntentCleanupInput) (nomad.EnvironmentState, error) {
	if job == nil || len(job.Spec.TaskGroups) != 1 || len(job.Spec.TaskGroups[0].Tasks) != 1 {
		return nomad.EnvironmentState{}, errors.New("unexpected task-group shape")
	}
	meta := job.Spec.Meta
	task := job.Spec.TaskGroups[0].Tasks[0]
	path, err := prunedIntentTaskBootstrapPath(task.Config)
	if err != nil {
		return nomad.EnvironmentState{}, err
	}
	policyVersion, err := strconv.Atoi(meta["kandev.native_job_policy_version"])
	if err != nil {
		return nomad.EnvironmentState{}, errors.New("native policy version is invalid")
	}
	workspacePath, err := nomad.EnvironmentWorkspacePath(profile.WorkspaceRoot, input.EnvironmentID, input.Generation)
	if err != nil || workspacePath != input.WorkspacePath {
		return nomad.EnvironmentState{}, errors.New("workspace path differs from the captured owner and generation")
	}
	state := nomad.EnvironmentState{
		Version: 1, Revision: 1, EnvironmentID: input.EnvironmentID, Generation: input.Generation,
		OperationID: meta["kandev.operation_id"], InstallationID: meta["kandev.installation_id"],
		ExecutionID: input.ExecutionID, InputDigest: meta["kandev.input_digest"], JobID: input.ExpectedJobID,
		Namespace: profile.Namespace, Region: profile.Region, Datacenter: profile.Datacenter, NodeID: profile.NodeID,
		Platform: profile.Platform, SSHHost: profile.SSHHost, SSHUser: profile.SSHUser, SSHPort: profile.SSHPort,
		SSHHostFingerprint: profile.SSHHostFingerprint, WorkspaceRoot: profile.WorkspaceRoot, WorkspacePath: workspacePath,
		AllocationRoot: profile.AllocationRoot, AgentctlPath: task.Env["KANDEV_AGENTCTL_PATH"], BootstrapPath: path,
		NativeJobPolicyVersion: policyVersion, NativeJobSpecSHA256: meta["kandev.native_job_spec_sha256"],
		CPUMHz: profile.CPUMHz, MemoryMB: profile.MemoryMB, Phase: "intent",
	}
	if state.OperationID != input.ExecutionID || meta["kandev.environment_id"] != input.EnvironmentID ||
		meta["kandev.generation"] != strconv.FormatInt(input.Generation, 10) || meta["kandev.node_id"] != input.NodeID ||
		task.Env["KANDEV_OPERATION_ID"] != input.ExecutionID || task.Env["KANDEV_ENVIRONMENT_ID"] != input.EnvironmentID ||
		task.Env["KANDEV_ENVIRONMENT_GENERATION"] != strconv.FormatInt(input.Generation, 10) || task.Env["KANDEV_NOMAD_NODE_ID"] != "${node.unique.id}" {
		return nomad.EnvironmentState{}, errors.New("job metadata or bootstrap environment differs from immutable operation")
	}
	return state, state.Validate()
}

func prunedIntentTaskBootstrapPath(config map[string]any) (string, error) {
	args, ok := config["args"].([]any)
	if !ok || len(args) != 1 {
		return "", errors.New("bootstrap command arguments do not match the fixed task template")
	}
	path, ok := args[0].(string)
	if !ok || !filepath.IsAbs(path) || filepath.Ext(path) != ".sh" {
		return "", errors.New("bootstrap path is not an absolute shell script")
	}
	return path, nil
}

func prunedIntentAllocatedPorts(allocation *nomad.AllocationRecord) (int, int, error) {
	if allocation == nil || allocation.AllocatedResources == nil {
		return 0, 0, errors.New("allocated ports are unavailable")
	}
	ports := map[string]int{}
	for _, port := range allocation.AllocatedResources.Shared.Ports {
		if port.Label == "agentctl_control" || port.Label == "agentctl_instance" {
			if port.Value < 1 || port.Value > 65535 || ports[port.Label] != 0 {
				return 0, 0, errors.New("allocated port label is duplicated or invalid")
			}
			ports[port.Label] = port.Value
		}
	}
	if ports["agentctl_control"] == 0 || ports["agentctl_instance"] == 0 || ports["agentctl_control"] == ports["agentctl_instance"] {
		return 0, 0, errors.New("two distinct allocated runtime ports are required")
	}
	return ports["agentctl_control"], ports["agentctl_instance"], nil
}

func prunedIntentMetadata(config map[string]string, tokenRef string, state nomad.EnvironmentState, input prunedIntentCleanupInput) map[string]any {
	metadata := make(map[string]any, len(config)+4)
	for key, value := range config {
		metadata[key] = value
	}
	metadata["nomad_token"] = tokenRef
	metadata["executor_type"] = string(models.ExecutorTypeNomad)
	metadata["executor_profile_id"] = input.ExecutorProfileID
	metadata[MetadataKeyBootstrapNonceSecret] = input.BootstrapSecretID
	metadata[nomad.EnvironmentStateMetadataKey] = state
	return metadata
}

func insertPrunedIntentCleanupRow(ctx context.Context, db *sqlx.DB, input prunedIntentCleanupInput, session struct {
	TaskID          string `db:"task_id"`
	ID              string `db:"id"`
	State           string `db:"state"`
	ExecutionID     string `db:"execution_profile_id"`
	ExecutorID      string `db:"executor_id"`
	ExecutorProfile string `db:"executor_profile_id"`
	RepositoryID    string `db:"repository_id"`
	EnvironmentID   string `db:"task_environment_id"`
	StartedAt       string `db:"started_at"`
}, env struct {
	ID         string `db:"id"`
	TaskID     string `db:"task_id"`
	Generation int64  `db:"ownership_generation"`
	Type       string `db:"executor_type"`
	ExecutorID string `db:"executor_id"`
	ProfileID  string `db:"executor_profile_id"`
	Status     string `db:"status"`
}, metadata map[string]any) error {
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	var currentSessionState string
	if err := tx.GetContext(ctx, &currentSessionState, `SELECT state FROM task_sessions WHERE task_id = ? AND id = ? AND executor_id = ? AND executor_profile_id = ? AND task_environment_id = ?`, input.TaskID, input.SessionID, input.ExecutorID, input.ExecutorProfileID, input.EnvironmentID); err != nil || currentSessionState != "CREATED" {
		return errors.New("canonical CREATED session changed before cleanup restore")
	}
	var ownerID, ownerType, ownerStatus string
	var generation int64
	var executorID, profileID string
	if err := tx.QueryRowxContext(ctx, `SELECT id, executor_type, status, ownership_generation, executor_id, executor_profile_id FROM task_environments WHERE task_id = ?`, input.TaskID).Scan(&ownerID, &ownerType, &ownerStatus, &generation, &executorID, &profileID); err != nil ||
		ownerID != input.EnvironmentID || ownerType != "nomad" || generation != input.Generation || ownerStatus != "creating" || executorID != input.ExecutorID || profileID != input.ExecutorProfileID {
		return errors.New("canonical environment owner changed before cleanup restore")
	}
	var count int
	if err := tx.GetContext(ctx, &count, `SELECT count(*) FROM executors_running WHERE id = ? OR session_id = ? OR task_id = ?`, input.SessionID, input.SessionID, input.TaskID); err != nil || count != 0 {
		return errors.New("executor row is no longer absent; refuse cleanup restore")
	}
	now := time.Now().UTC()
	_, err = tx.ExecContext(ctx, `INSERT INTO executors_running (
		id, session_id, task_id, execution_profile_id, executor_id, runtime, status, resumable, agent_execution_id,
		metadata, created_at, updated_at
	) VALUES (?, ?, ?, ?, ?, 'nomad', 'stopped', 0, ?, ?, ?, ?)`,
		input.SessionID, input.SessionID, input.TaskID, session.ExecutionID, input.ExecutorID,
		input.ExecutionID, string(encoded), session.StartedAt, now)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func updatePrunedIntentCleanupRow(ctx context.Context, db *sqlx.DB, input prunedIntentCleanupInput, metadata map[string]any, marker map[string]any) error {
	metadata["fixture_operator_pruned_restore"] = marker
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	var current string
	if err := db.GetContext(ctx, &current, `SELECT metadata FROM executors_running WHERE id = ? AND session_id = ? AND task_id = ? AND agent_execution_id = ? AND executor_id = ? AND runtime = 'nomad' AND status = 'stopped'`, input.SessionID, input.SessionID, input.TaskID, input.ExecutionID, input.ExecutorID); err != nil {
		return err
	}
	result, err := db.ExecContext(ctx, `UPDATE executors_running SET metadata = ?, updated_at = ? WHERE id = ? AND session_id = ? AND task_id = ? AND agent_execution_id = ? AND executor_id = ? AND runtime = 'nomad' AND status = 'stopped' AND metadata = ?`, string(encoded), time.Now().UTC(), input.SessionID, input.SessionID, input.TaskID, input.ExecutionID, input.ExecutorID, current)
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return errors.New("cleanup row CAS did not affect exactly one original operation")
	}
	return nil
}

func newFixtureCleanupClaim(input prunedIntentCleanupInput) (string, string) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		panic("secure fixture cleanup claim generation failed")
	}
	claimID := hex.EncodeToString(nonce[:])
	bound := fixtureCleanupClaimBinding(input)
	digest := sha256.Sum256([]byte(bound + "\x00" + claimID))
	return claimID, hex.EncodeToString(digest[:])
}

func fixtureCleanupClaimBinding(input prunedIntentCleanupInput) string {
	return fmt.Sprintf("fixture-cleanup-v1\x00row=%s\x00task=%s\x00session=%s\x00executor=%s\x00profile=%s\x00runtime=nomad\x00status=stopped\x00environment=%s\x00generation=%d\x00operation=%s\x00job=%s\x00allocation=%s\x00node=%s",
		input.SessionID, input.TaskID, input.SessionID, input.ExecutorID, input.ExecutorProfileID,
		input.EnvironmentID, input.Generation, input.ExecutionID, input.ExpectedJobID, input.ExpectedAllocationID, input.NodeID)
}

func verifyFixtureCleanupClaim(input prunedIntentCleanupInput, marker map[string]any) error {
	claimID, ok := marker["cleanup_claim_id"].(string)
	if !ok || len(claimID) != 32 {
		return errors.New("fixture cleanup claim is missing")
	}
	digest := sha256.Sum256([]byte(fixtureCleanupClaimBinding(input) + "\x00" + claimID))
	want := hex.EncodeToString(digest[:])
	got, ok := marker["cleanup_claim_binding_sha256"].(string)
	if !ok || got != want {
		return errors.New("fixture cleanup claim does not bind the captured operation")
	}
	return nil
}

func fixtureCleanupReceiptBinding(claimBinding, receiptDigest string) string {
	digest := sha256.Sum256([]byte("fixture-cleanup-receipt-v1\x00" + claimBinding + "\x00" + receiptDigest))
	return hex.EncodeToString(digest[:])
}
