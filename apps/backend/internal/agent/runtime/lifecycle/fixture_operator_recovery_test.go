package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"

	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	"golang.org/x/crypto/ssh"
)

// TestFixtureOperatorRecovery is an explicit, test-only operator procedure for
// draining one failed disposable fixture. It never repairs the typed state or
// changes the failed session. The legacy missing workspace_path is preserved.
func TestFixtureOperatorRecovery(t *testing.T) {
	mode := os.Getenv("FF_OPERATOR_MODE")
	if mode == "" {
		t.Skip("fixture operator procedure requires explicit mode")
	}
	if mode != "inspect" && mode != "drain" && mode != "finalize" {
		t.Fatal("fixture operator mode must be inspect, drain, or finalize")
	}
	if os.Getenv("FF_ALLOW_FIXTURE_OPERATOR_RECOVERY") != "yes" {
		t.Fatal("fixture operator procedure requires explicit fixture-only opt-in")
	}
	root := requiredFixtureEnv(t, "FF_FIXTURE_ROOT")
	taskID := requiredFixtureEnv(t, "FF_FIXTURE_TASK_ID")
	expectedJobID := requiredFixtureEnv(t, "FF_EXPECTED_JOB_ID")
	expectedAllocationID := requiredFixtureEnv(t, "FF_EXPECTED_ALLOCATION_ID")
	expectedNodeID := requiredFixtureEnv(t, "FF_EXPECTED_NODE_ID")
	expectedPID, err := strconv.Atoi(requiredFixtureEnv(t, "FF_EXPECTED_LEADER_PID"))
	if err != nil || expectedPID < 1 {
		t.Fatal("expected fixture leader PID is invalid")
	}
	expectedStart := requiredFixtureEnv(t, "FF_EXPECTED_LEADER_START")
	verifyFixturePrivateRoot(t, root)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dbPath := filepath.Join(root, "kandev", "home", "data", "kandev.db")
	db, err := sqlx.Open("sqlite3", "file:"+dbPath+"?_busy_timeout=10000&_foreign_keys=on")
	if err != nil {
		t.Fatal("open isolated fixture database")
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	key, err := secrets.NewMasterKeyProvider(filepath.Join(root, "kandev", "home", "data"))
	if err != nil {
		t.Fatal("open isolated fixture secret key")
	}
	secretStore, closeStore, err := secrets.Provide(db, db, key)
	if err != nil {
		t.Fatal("open isolated fixture secret store")
	}
	defer func() { _ = closeStore() }()

	row, err := readFixtureOperatorRow(ctx, db, taskID)
	if err != nil {
		t.Fatal("read exact failed fixture runtime row")
	}
	assertFixtureOperatorOwnership(t, ctx, db, row, taskID, expectedJobID, expectedAllocationID, expectedNodeID)
	metadata, state := fixtureMetadataAndLegacyState(t, row.Metadata)
	if state.WorkspacePath != "" || !metadataHasMissingWorkspacePath(row.Metadata) {
		// A legacy missing path is an explicit pre-release constraint. Do not
		// derive, fill, or otherwise make this state eligible for production reuse.
		t.Fatal("legacy fixture workspace_path shape changed; operator recovery refused")
	}
	profile, err := nomadProfileFromMetadata(metadataAsAny(metadata))
	if err != nil {
		t.Fatal("decode immutable fixture profile")
	}
	nonceRef := fixtureRawString(t, metadata, MetadataKeyBootstrapNonceSecret)
	authRef := fixtureRawString(t, metadata, MetadataKeyAuthTokenSecret)
	if nonceRef == "" {
		t.Fatal("fixture no longer retains its original nonce reference")
	}
	if mode == "finalize" {
		finalizeFixtureOperatorRecovery(t, ctx, db, secretStore, root, row, metadata, state, profile, authRef, expectedPID, expectedStart)
		return
	}
	if authRef != "" {
		t.Fatal("auth token reference already exists; use finalize mode without replaying the nonce")
	}
	if _, exists := metadata["fixture_operator_recovery"]; exists {
		t.Fatal("a prior one-shot handshake marker exists; do not spend or replay the nonce")
	}
	nonce, err := secretStore.Reveal(ctx, nonceRef)
	if err != nil || nonce == "" {
		t.Fatal("reveal original fixture nonce reference")
	}
	authTokenID := kubernetesRuntimeSecretID(row.ExecutionID, "agentctl-auth")
	if _, err := secretStore.Get(ctx, authTokenID); err == nil {
		t.Fatal("an unreferenced auth token already exists; do not spend the one-use nonce")
	} else if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("inspect deterministic fixture auth token reference")
	}
	nomadToken, err := secretStore.Reveal(ctx, profile.NomadTokenSecretID)
	if err != nil || nomadToken == "" {
		t.Fatal("reveal fixture Nomad API credential")
	}
	nomadClient, err := nomad.NewClient(profile.Address, nomadToken)
	if err != nil {
		t.Fatal("configure loopback fixture Nomad API client")
	}
	defer nomadClient.CloseIdleConnections()
	job, err := nomadClient.GetJob(ctx, state.Namespace, state.JobID)
	if err != nil || !jobMatchesState(job, state) {
		t.Fatal("exact immutable fixture job identity is unavailable")
	}
	allocations, err := nomadClient.ListAllocations(ctx, state.Namespace, state.JobID)
	if err != nil || len(allocations) != 1 || allocations[0].ID != state.AllocationID {
		t.Fatal("fixture job has an unexpected allocation set; preserve it for investigation")
	}
	allocation, err := nomadClient.GetAllocation(ctx, state.Namespace, state.AllocationID)
	if err != nil || allocation.ID != state.AllocationID || allocation.JobID != state.JobID ||
		allocation.Namespace != state.Namespace || allocation.NodeID != state.NodeID ||
		allocation.CreateIndex != state.AllocationCreateIndex || allocation.ClientStatus != "running" ||
		allocation.DesiredStatus != "run" || allocation.TaskStates["agentctl"].State != "running" {
		t.Fatal("full live allocation identity or task status changed; preserve the fence")
	}
	nodeStatus, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID)
	if err != nil || nodeStatus != "ready" {
		t.Fatal("fixture Nomad node is not positively ready")
	}
	if err := verifyFixtureProcessIdentity(expectedPID, expectedStart, root, state.JobID); err != nil {
		t.Fatal("exact fixture leader PID/start identity did not match")
	}

	target, err := ResolveSSHTarget(SSHConnConfig{
		Host: profile.SSHHost, Port: profile.SSHPort, User: profile.SSHUser,
		IdentitySource: SSHIdentitySourceFile, IdentityFile: filepath.Join(root, "ssh", "client_ed25519"),
		PinnedFingerprint: profile.SSHHostFingerprint,
	})
	if err != nil {
		t.Fatal("resolve exact pinned fixture SSH identity")
	}
	sshClient, err := DialSSH(ctx, target)
	if err != nil {
		t.Fatal("connect to exact pinned fixture worker")
	}
	defer func() { _ = sshClient.Close() }()
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("exact immutable worker fence owner did not match")
	}
	if mode == "inspect" {
		t.Logf("fixture preflight passed: task=%s job=%s allocation=%s node=%s phase=%s leader_pid=%d start=%q workspace_path=missing", taskID, state.JobID, state.AllocationID, state.NodeID, state.Phase, expectedPID, expectedStart)
		return
	}
	if state.Phase != "cleanup_pending" {
		t.Fatal("operator drain only accepts the exact cleanup_pending legacy fixture")
	}

	started := time.Now().UTC()
	manual := map[string]any{
		"fixture_only": true, "task_id": taskID, "session_id": row.SessionID,
		"environment_id": state.EnvironmentID, "generation": state.Generation,
		"execution_id": row.ExecutionID, "job_id": state.JobID,
		"allocation_id": state.AllocationID, "node_id": state.NodeID,
		"workspace_path_preserved_missing": true,
		"handshake_attempt_started_at":     started.Format(time.RFC3339Nano),
		"handshake_attempt_count":          1,
	}
	updated, err := fixtureOperatorCAS(ctx, db, row, metadata, manual)
	if err != nil {
		t.Fatal("persist one-shot marker before using the original nonce")
	}
	row = updated
	metadata, state = fixtureMetadataAndLegacyState(t, row.Metadata)

	forward, err := StartPortForward(sshClient, state.ControlPort, newTestLogger())
	if err != nil {
		t.Fatal("open exact worker-owned loopback control forward")
	}
	defer func() { _ = forward.Close() }()
	if err := waitAgentctlHealthy(ctx, forward.LocalPort(), sshAgentctlHealthTimeout); err != nil {
		t.Fatal("agentctl control health did not pass before one-shot handshake")
	}
	bootstrap := agentctl.NewControlClient("127.0.0.1", forward.LocalPort(), newTestLogger())
	defer bootstrap.Close()
	authToken, err := bootstrap.Handshake(ctx, nonce)
	if err != nil || authToken == "" {
		t.Fatal("the sole original-nonce handshake failed; leave marker, job, and fence quarantined")
	}
	if err := secretStore.Create(ctx, &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: authTokenID, Name: "fixture-agentctl-auth"}, Value: authToken,
	}); err != nil {
		t.Fatal("persist returned fixture auth token")
	}
	metadata[MetadataKeyAuthTokenSecret], _ = json.Marshal(authTokenID)
	manual["auth_token_reference_persisted"] = true
	manual["handshake_completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	row, err = fixtureOperatorCAS(ctx, db, row, metadata, manual)
	if err != nil {
		t.Fatal("CAS original auth token reference to the exact failed runtime row")
	}

	control := agentctl.NewControlClient("127.0.0.1", forward.LocalPort(), newTestLogger(), agentctl.WithControlAuthToken(authToken))
	defer control.Close()
	identity, err := control.GetIdentity(ctx)
	if err != nil || identity == nil || strings.TrimSpace(identity.ServerIdentity) == "" || !containsNomadCapability(identity.Capabilities, nomadDrainCapability) {
		t.Fatal("authenticated exact-runtime identity or drain capability was unavailable")
	}
	drainRequest := agentctl.RuntimeDrainRequest{
		RequestID:              "manual-fixture-drain-" + state.AllocationID,
		ExpectedServerIdentity: identity.ServerIdentity,
		DeadlineMS:             int64(30 * time.Second / time.Millisecond),
	}
	receipt, err := control.SealAndDrainRuntime(ctx, drainRequest)
	if err != nil {
		t.Fatal("authenticated exact-runtime seal-and-drain did not return positive evidence")
	}
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("encode validated fixture drain receipt")
	}
	receiptDigest := sha256.Sum256(receiptBytes)
	manual["drain_receipt_digest"] = hex.EncodeToString(receiptDigest[:])
	manual["drain_request_id"] = receipt.RequestID
	manual["server_identity"] = identity.ServerIdentity
	manual["drain_completed_at"] = receipt.CompletedAt.UTC().Format(time.RFC3339Nano)
	stateRaw := fixtureRawObject(t, metadata[nomad.EnvironmentStateMetadataKey])
	stateRaw["drain_receipt_digest"], _ = json.Marshal(hex.EncodeToString(receiptDigest[:]))
	stateRaw["drain_receipt_generation"], _ = json.Marshal(state.Generation)
	stateRaw["server_identity"], _ = json.Marshal(identity.ServerIdentity)
	metadata[nomad.EnvironmentStateMetadataKey], _ = json.Marshal(stateRaw)
	row, err = fixtureOperatorCAS(ctx, db, row, metadata, manual)
	if err != nil {
		t.Fatal("CAS receipt digest and generation to the exact legacy runtime row")
	}

	if err := deregisterFixtureJobWithoutPurge(ctx, profile.Address, nomadToken, state); err != nil {
		t.Fatal("deregister exact immutable fixture job without purge")
	}
	finalAllocs, err := waitFixtureJobTerminal(ctx, db, profile.Address, nomadToken, state, taskID, row.SessionID)
	if err != nil {
		t.Fatal("whole-job deregistration did not reach positively terminal allocation history")
	}
	if status, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID); err != nil || status != "ready" {
		t.Fatal("known worker did not remain ready after whole-job deregistration")
	}
	assertFixturePIDGone(t, expectedPID, expectedStart, root, state.JobID)
	if err := assertFixturePortsClosed(ctx, sshClient, state.ControlPort, state.InstancePort); err != nil {
		t.Fatal("exact fixture runtime ports remain open")
	}
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("exact worker fence changed before release")
	}
	manual["job_deregistered_without_purge"] = true
	manual["allocation_history"] = finalAllocs
	manual["leader_pid_start_absent"] = true
	manual["reserved_ports_closed"] = true
	manual["worker_ready"] = true
	manual["fence_release_pending"] = true
	row, err = fixtureOperatorCAS(ctx, db, row, metadata, manual)
	if err != nil {
		t.Fatal("persist terminal fixture evidence before exact fence release")
	}
	if err := releaseExactFixtureFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("exact compare-and-delete of fixture fence failed")
	}
	manual["fence_released"] = true
	manual["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	row, err = fixtureOperatorCAS(ctx, db, row, metadata, manual)
	if err != nil {
		t.Fatal("persist final fixture cleanup evidence")
	}
	evidence := map[string]any{
		"fixture_only": true, "task_id": taskID, "session_id": row.SessionID,
		"environment_id": state.EnvironmentID, "generation": state.Generation,
		"execution_id": row.ExecutionID, "job_id": state.JobID, "allocations": finalAllocs,
		"node_id": state.NodeID, "workspace_path_preserved_missing": true,
		"drain_receipt_validated": true, "leader_pid_start_absent": true,
		"reserved_ports_closed": true, "worker_ready": true,
		"fence_released": true, "session_state_preserved_failed": true,
		"completed_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	writeFixtureEvidence(t, root, evidence)
	t.Logf("fixture operator cleanup passed: task=%s job=%s allocation_history=%d worker=%s workspace_path=missing session=FAILED fence_released=true", taskID, state.JobID, len(finalAllocs), state.NodeID)
}

func finalizeFixtureOperatorRecovery(
	t *testing.T,
	ctx context.Context,
	db *sqlx.DB,
	secretStore secrets.SecretStore,
	root string,
	row fixtureOperatorRow,
	metadata map[string]json.RawMessage,
	state nomad.EnvironmentState,
	profile nomad.ProfileConfig,
	authRef string,
	expectedPID int,
	expectedStart string,
) {
	t.Helper()
	if state.Phase != "cleanup_pending" || authRef != kubernetesRuntimeSecretID(row.ExecutionID, "agentctl-auth") || !metadataHasMissingWorkspacePath(row.Metadata) {
		t.Fatal("finalize requires the exact one-shot legacy fixture and its retained missing workspace path")
	}
	var manual map[string]any
	if err := json.Unmarshal(metadata["fixture_operator_recovery"], &manual); err != nil || manual == nil {
		t.Fatal("one-shot fixture recovery marker is missing")
	}
	count, ok := manual["handshake_attempt_count"].(float64)
	if !ok || count != 1 || manual["auth_token_reference_persisted"] != true {
		t.Fatal("one-shot handshake and token persistence evidence is incomplete")
	}
	var receiptDigest, requestID, serverIdentity string
	stateRaw := fixtureRawObject(t, metadata[nomad.EnvironmentStateMetadataKey])
	_ = json.Unmarshal(stateRaw["drain_receipt_digest"], &receiptDigest)
	_ = json.Unmarshal(stateRaw["server_identity"], &serverIdentity)
	requestID, _ = manual["drain_request_id"].(string)
	markerDigest, _ := manual["drain_receipt_digest"].(string)
	markerServer, _ := manual["server_identity"].(string)
	if len(receiptDigest) != 64 || markerDigest != receiptDigest || requestID != "manual-fixture-drain-"+state.AllocationID || markerServer == "" || markerServer != serverIdentity {
		t.Fatal("persisted drain receipt provenance does not match the exact runtime state")
	}
	if _, err := hex.DecodeString(receiptDigest); err != nil {
		t.Fatal("persisted drain receipt digest is invalid")
	}
	if _, err := secretStore.Reveal(ctx, authRef); err != nil {
		t.Fatal("persisted exact-runtime auth token reference is unavailable")
	}
	var nonceRef string
	if err := json.Unmarshal(metadata[MetadataKeyBootstrapNonceSecret], &nonceRef); err != nil || nonceRef == "" {
		t.Fatal("original bootstrap nonce reference was not preserved")
	}

	nomadToken, err := secretStore.Reveal(ctx, profile.NomadTokenSecretID)
	if err != nil || nomadToken == "" {
		t.Fatal("reveal fixture Nomad API credential")
	}
	client, err := nomad.NewClient(profile.Address, nomadToken)
	if err != nil {
		t.Fatal("configure loopback fixture Nomad API client")
	}
	defer client.CloseIdleConnections()
	job, err := client.GetJob(ctx, state.Namespace, state.JobID)
	if err != nil || !jobMatchesState(job, state) {
		t.Fatal("deregistered job no longer matches the exact immutable operation")
	}
	allocations, err := waitFixtureJobTerminal(ctx, db, profile.Address, nomadToken, state, row.TaskID, row.SessionID)
	if err != nil || len(allocations) == 0 {
		t.Fatal("whole-job terminal allocation history is not positively established")
	}
	if status, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID); err != nil || status != "ready" {
		t.Fatal("known fixture node is not ready during finalization")
	}
	assertFixturePIDGone(t, expectedPID, expectedStart, root, state.JobID)

	target, err := ResolveSSHTarget(SSHConnConfig{
		Host: profile.SSHHost, Port: profile.SSHPort, User: profile.SSHUser,
		IdentitySource: SSHIdentitySourceFile, IdentityFile: filepath.Join(root, "ssh", "client_ed25519"),
		PinnedFingerprint: profile.SSHHostFingerprint,
	})
	if err != nil {
		t.Fatal("resolve exact pinned fixture SSH identity")
	}
	sshClient, err := DialSSH(ctx, target)
	if err != nil {
		t.Fatal("connect to exact pinned fixture worker during finalization")
	}
	defer func() { _ = sshClient.Close() }()
	if err := assertFixturePortsClosed(ctx, sshClient, state.ControlPort, state.InstancePort); err != nil {
		t.Fatal("exact fixture runtime ports are not closed")
	}
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("exact worker fence owner changed before release")
	}
	manual["job_deregistered_without_purge"] = true
	manual["allocation_history"] = allocations
	manual["leader_pid_start_absent"] = true
	manual["reserved_ports_closed"] = true
	manual["worker_ready"] = true
	manual["fence_release_pending"] = true
	row, err = fixtureOperatorCAS(ctx, db, row, metadata, manual)
	if err != nil {
		t.Fatal("persist terminal fixture evidence before exact fence release")
	}
	if err := releaseExactFixtureFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("exact compare-and-delete of fixture fence failed")
	}
	manual["fence_released"] = true
	manual["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	row, err = fixtureOperatorCAS(ctx, db, row, metadata, manual)
	if err != nil {
		t.Fatal("persist final fixture cleanup evidence")
	}
	evidence := map[string]any{
		"fixture_only": true, "task_id": row.TaskID, "session_id": row.SessionID,
		"environment_id": state.EnvironmentID, "generation": state.Generation,
		"execution_id": row.ExecutionID, "job_id": state.JobID, "allocations": allocations,
		"node_id": state.NodeID, "workspace_path_preserved_missing": true,
		"drain_receipt_validated": true, "leader_pid_start_absent": true,
		"reserved_ports_closed": true, "worker_ready": true,
		"fence_released": true, "session_state_preserved_failed": true,
		"completed_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	writeFixtureEvidence(t, root, evidence)
	t.Logf("fixture operator cleanup finalized: task=%s job=%s allocation_history=%d worker=%s workspace_path=missing session=FAILED fence_released=true", row.TaskID, state.JobID, len(allocations), state.NodeID)
}

type fixtureOperatorRow struct {
	ID          string `db:"id"`
	SessionID   string `db:"session_id"`
	TaskID      string `db:"task_id"`
	ExecutionID string `db:"execution_id"`
	ExecutorID  string `db:"executor_id"`
	Runtime     string `db:"runtime"`
	Status      string `db:"status"`
	Metadata    string `db:"metadata"`
	UpdatedAt   string `db:"updated_at"`
}

func requiredFixtureEnv(t *testing.T, key string) string {
	t.Helper()
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		t.Fatalf("fixture operator requires %s", key)
	}
	return v
}

func verifyFixturePrivateRoot(t *testing.T, root string) {
	t.Helper()
	info, err := os.Lstat(root)
	if err != nil {
		t.Fatal("fixture root must be an owned private directory")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		t.Fatal("fixture root must be an owned private directory")
	}
}

func readFixtureOperatorRow(ctx context.Context, db *sqlx.DB, taskID string) (fixtureOperatorRow, error) {
	var row fixtureOperatorRow
	err := db.GetContext(ctx, &row, `SELECT id, session_id, task_id, agent_execution_id AS execution_id, executor_id,
		runtime, status, metadata, CAST(updated_at AS TEXT) AS updated_at
		FROM executors_running WHERE task_id = ? AND runtime = 'nomad'`, taskID)
	return row, err
}

func assertFixtureOperatorOwnership(t *testing.T, ctx context.Context, db *sqlx.DB, row fixtureOperatorRow, taskID, jobID, allocationID, nodeID string) {
	t.Helper()
	if row.ID == "" || row.SessionID != row.ID || row.TaskID != taskID || row.Runtime != "nomad" || row.ExecutionID == "" || row.Status != models.ExecutorRunningStatusStarting {
		t.Fatal("exact fixture executor row identity or status changed")
	}
	var env struct {
		ID           string `db:"id"`
		TaskID       string `db:"task_id"`
		ExecutorID   string `db:"executor_id"`
		ExecutorType string `db:"executor_type"`
		Status       string `db:"status"`
		Generation   int64  `db:"ownership_generation"`
	}
	if err := db.GetContext(ctx, &env, `SELECT id, task_id, executor_id, executor_type, status, ownership_generation
		FROM task_environments WHERE task_id = ?`, taskID); err != nil || env.ID == "" || env.TaskID != taskID || env.ExecutorType != "nomad" || env.Generation != 1 || env.Status != "failed" {
		t.Fatal("exact failed fixture environment ownership/generation changed")
	}
	if env.ExecutorID != row.ExecutorID {
		t.Fatal("fixture executor row and environment owner differ")
	}
	var sessionState string
	if err := db.GetContext(ctx, &sessionState, `SELECT state FROM task_sessions WHERE task_id = ? AND id = ?`, taskID, row.SessionID); err != nil || sessionState != "FAILED" {
		t.Fatal("fixture session is not the original failed session")
	}
	metadata, state := fixtureMetadataAndLegacyState(t, row.Metadata)
	_ = metadata
	if state.JobID != jobID || state.AllocationID != allocationID || state.NodeID != nodeID || state.ExecutionID != row.ExecutionID || state.OperationID != row.ExecutionID || state.EnvironmentID != env.ID || state.Generation != env.Generation {
		t.Fatal("immutable fixture operation, environment, job, allocation, or worker identity changed")
	}
	if state.Phase != "cleanup_pending" {
		t.Fatal("fixture row is not cleanup_pending")
	}
}

func fixtureMetadataAndLegacyState(t *testing.T, raw string) (map[string]json.RawMessage, nomad.EnvironmentState) {
	t.Helper()
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(raw), &metadata); err != nil {
		t.Fatal("decode exact fixture metadata")
	}
	var state nomad.EnvironmentState
	if err := json.Unmarshal(metadata[nomad.EnvironmentStateMetadataKey], &state); err != nil {
		t.Fatal("decode legacy fixture Nomad state")
	}
	return metadata, state
}

func metadataHasMissingWorkspacePath(raw string) bool {
	var envelope map[string]json.RawMessage
	var state map[string]json.RawMessage
	if json.Unmarshal([]byte(raw), &envelope) != nil || json.Unmarshal(envelope[nomad.EnvironmentStateMetadataKey], &state) != nil {
		return false
	}
	_, exists := state["workspace_path"]
	return !exists
}

func metadataMapHasMissingWorkspacePath(metadata map[string]json.RawMessage) bool {
	var state map[string]json.RawMessage
	if json.Unmarshal(metadata[nomad.EnvironmentStateMetadataKey], &state) != nil {
		return false
	}
	_, exists := state["workspace_path"]
	return !exists
}

func metadataAsAny(metadata map[string]json.RawMessage) map[string]interface{} {
	result := make(map[string]interface{}, len(metadata))
	for key, raw := range metadata {
		var value any
		if json.Unmarshal(raw, &value) == nil {
			result[key] = value
		}
	}
	return result
}

func fixtureRawObject(t *testing.T, raw json.RawMessage) map[string]json.RawMessage {
	t.Helper()
	var result map[string]json.RawMessage
	if err := json.Unmarshal(raw, &result); err != nil || result == nil {
		t.Fatal("decode fixture metadata object")
	}
	return result
}

func fixtureRawString(t *testing.T, metadata map[string]json.RawMessage, key string) string {
	t.Helper()
	if len(metadata[key]) == 0 {
		return ""
	}
	var value string
	if raw := metadata[key]; json.Unmarshal(raw, &value) != nil {
		t.Fatalf("fixture credential reference %s is unavailable", key)
	}
	return value
}

func fixtureOperatorCAS(ctx context.Context, db *sqlx.DB, previous fixtureOperatorRow, metadata map[string]json.RawMessage, manual map[string]any) (fixtureOperatorRow, error) {
	if !metadataMapHasMissingWorkspacePath(metadata) {
		return fixtureOperatorRow{}, errors.New("fixture operator CAS refuses to alter legacy missing workspace_path")
	}
	encodedManual, err := json.Marshal(manual)
	if err != nil {
		return fixtureOperatorRow{}, err
	}
	metadata["fixture_operator_recovery"] = encodedManual
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fixtureOperatorRow{}, err
	}
	tx, err := db.BeginTxx(ctx, nil)
	if err != nil {
		return fixtureOperatorRow{}, err
	}
	defer func() { _ = tx.Rollback() }()
	var environmentID, executorType, environmentStatus string
	var generation int64
	if err := tx.QueryRowxContext(ctx, `SELECT id, executor_type, status, ownership_generation FROM task_environments WHERE task_id = ?`, previous.TaskID).
		Scan(&environmentID, &executorType, &environmentStatus, &generation); err != nil {
		return fixtureOperatorRow{}, err
	}
	if environmentID == "" || executorType != "nomad" || environmentStatus != "failed" || generation != 1 {
		return fixtureOperatorRow{}, errors.New("fixture environment CAS owner/generation/status changed")
	}
	result, err := tx.ExecContext(ctx, `UPDATE executors_running SET metadata = ?, updated_at = ?
		WHERE id = ? AND session_id = ? AND task_id = ? AND executor_id = ? AND agent_execution_id = ? AND runtime = 'nomad'
		AND status = 'starting' AND updated_at = ? AND metadata = ?`, string(encoded), time.Now().UTC(),
		previous.ID, previous.SessionID, previous.TaskID, previous.ExecutorID, previous.ExecutionID, previous.UpdatedAt, previous.Metadata)
	if err != nil {
		return fixtureOperatorRow{}, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return fixtureOperatorRow{}, errors.New("fixture runtime row compare-and-swap did not affect exactly one row")
	}
	if err := tx.Commit(); err != nil {
		return fixtureOperatorRow{}, err
	}
	return readFixtureOperatorRow(ctx, db, previous.TaskID)
}

func verifyFixtureProcessIdentity(pid int, start, root, jobID string) error {
	identity, found, err := fixtureProcessIdentity(pid)
	if err != nil {
		return errors.New("cannot inspect fixture process identity")
	}
	if !found || identity.Start != start || !strings.Contains(identity.Command, root) || !strings.Contains(identity.Command, jobID) || !strings.Contains(strings.ToLower(identity.Command), "agentctl") {
		return errors.New("fixture process does not match exact job path and agentctl identity")
	}
	return nil
}

func assertFixturePIDGone(t *testing.T, pid int, start, root, jobID string) {
	t.Helper()
	identity, found, err := fixtureProcessIdentity(pid)
	if err != nil {
		t.Fatal("could not verify fixture leader absence")
	}
	if found && identity.Start == start && strings.Contains(identity.Command, root) && strings.Contains(identity.Command, jobID) {
		t.Fatal("exact fixture leader PID/start identity remains alive")
	}
	if fixtureJobProcessCount(t, root, jobID) != 0 {
		t.Fatal("fixture agentctl/ACP process for the exact job remains alive")
	}
}

type fixtureProcIdentity struct {
	Start   string
	Command string
}

func fixtureProcessIdentity(pid int) (fixtureProcIdentity, bool, error) {
	output, err := exec.Command("/bin/ps", "-axo", "pid=,lstart=,command=").Output()
	if err != nil {
		return fixtureProcIdentity{}, false, err
	}
	for _, line := range strings.Split(string(output), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 {
			continue
		}
		current, parseErr := strconv.Atoi(fields[0])
		if parseErr != nil || current != pid {
			continue
		}
		return fixtureProcIdentity{Start: strings.Join(fields[1:6], " "), Command: strings.Join(fields[6:], " ")}, true, nil
	}
	return fixtureProcIdentity{}, false, nil
}

func fixtureJobProcessCount(t *testing.T, root, jobID string) int {
	t.Helper()
	output, err := exec.Command("/bin/ps", "-axo", "pid=,lstart=,command=").Output()
	if err != nil {
		t.Fatal("could not inspect fixture-scoped process table")
	}
	count := 0
	for _, line := range strings.Split(string(output), "\n") {
		if !strings.Contains(line, root) || !strings.Contains(line, jobID) {
			continue
		}
		lower := strings.ToLower(line)
		if strings.Contains(lower, "agentctl") || strings.Contains(lower, "inert-acp") || strings.Contains(lower, "fixture-acp") {
			count++
		}
	}
	return count
}

func deregisterFixtureJobWithoutPurge(ctx context.Context, address, token string, state nomad.EnvironmentState) error {
	parsed, err := url.Parse(strings.TrimRight(address, "/") + "/v1/job/" + url.PathEscape(state.JobID))
	if err != nil {
		return err
	}
	query := parsed.Query()
	query.Set("namespace", state.Namespace)
	query.Set("purge", "false")
	parsed.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, parsed.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Nomad-Token", token)
	client := &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("Nomad job deregistration returned HTTP %d", response.StatusCode)
	}
	return nil
}

func waitFixtureJobTerminal(ctx context.Context, db *sqlx.DB, address, token string, state nomad.EnvironmentState, taskID, sessionID string) ([]map[string]any, error) {
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		var job struct {
			ID, Namespace, Status string
			Stop                  bool
		}
		jobErr := fixtureNomadGet(ctx, address, token, "/v1/job/"+url.PathEscape(state.JobID)+"?namespace="+url.QueryEscape(state.Namespace), &job)
		client, err := nomad.NewClient(address, token)
		if err != nil {
			return nil, err
		}
		allocations, listErr := client.ListAllocations(ctx, state.Namespace, state.JobID)
		var evidence []map[string]any
		allTerminal := len(allocations) > 0
		seenOriginal := false
		for _, summary := range allocations {
			full, getErr := client.GetAllocation(ctx, state.Namespace, summary.ID)
			if getErr != nil || full.ID != summary.ID || full.JobID != state.JobID || full.Namespace != state.Namespace || full.NodeID != state.NodeID || full.CreateIndex != summary.CreateIndex {
				allTerminal = false
				break
			}
			if full.ID == state.AllocationID && full.CreateIndex == state.AllocationCreateIndex {
				seenOriginal = true
			}
			if (full.ClientStatus != "complete" && full.ClientStatus != "failed") || full.TaskStates["agentctl"].State != "dead" {
				allTerminal = false
			}
			evidence = append(evidence, map[string]any{"id": full.ID, "create_index": full.CreateIndex, "client_status": full.ClientStatus, "desired_status": full.DesiredStatus, "agentctl_state": full.TaskStates["agentctl"].State, "node_id": full.NodeID})
		}
		client.CloseIdleConnections()
		if jobErr == nil && job.ID == state.JobID && job.Namespace == state.Namespace && job.Status == "dead" && job.Stop && listErr == nil && allTerminal && seenOriginal {
			var stateValue string
			if err := db.GetContext(ctx, &stateValue, "SELECT state FROM task_sessions WHERE task_id = ? AND id = ?", taskID, sessionID); err != nil || stateValue != "FAILED" {
				return nil, errors.New("failed fixture session state changed during operator cleanup")
			}
			return evidence, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("job or allocation history did not become terminal within the fixture deadline")
		case <-ticker.C:
		}
	}
}

func assertFixturePortsClosed(ctx context.Context, sshClient *ssh.Client, ports ...int) error {
	for _, port := range ports {
		if port < 1 || port > 65535 {
			return errors.New("fixture runtime port is invalid")
		}
		command := "if nc -z -G 1 127.0.0.1 " + strconv.Itoa(port) + " >/dev/null 2>&1; then exit 91; else printf closed; fi"
		output, _, err := runSSHCommand(ctx, sshClient, command)
		if err != nil || strings.TrimSpace(output) != "closed" {
			return errors.New("fixture runtime port remains open or could not be checked")
		}
	}
	return nil
}

func releaseExactFixtureFence(ctx context.Context, sshClient *ssh.Client, profile nomad.ProfileConfig, state nomad.EnvironmentState) error {
	workerStateRoot := filepath.Join(filepath.Dir(filepath.Dir(profile.AllocationRoot)), "worker-state", state.NodeID)
	fenceDir := filepath.Join(workerStateRoot, "active-environment")
	fencePath := filepath.Join(fenceDir, "owner")
	owner := strings.Join([]string{state.InstallationID, state.NodeID, state.EnvironmentID, fmt.Sprint(state.Generation), state.OperationID, state.InputDigest, state.JobID}, ":") + "\n" + state.AllocationID
	command := "f=" + shellQuote(fencePath) + "; d=" + shellQuote(fenceDir) +
		`; [ ! -L "$d" ] && [ -d "$d" ] && [ ! -L "$f" ] && [ -f "$f" ] && [ "$(stat -f %Lp "$f")" = 600 ] && [ "$(cat "$f")" = ` + shellQuote(owner) +
		` ] || exit 78; rm "$f" && rmdir "$d" && printf released`
	output, _, err := runSSHCommand(ctx, sshClient, command)
	if err != nil || strings.TrimSpace(output) != "released" {
		return errors.New("exact owner compare-and-delete did not release the fixture fence")
	}
	return nil
}

func writeFixtureEvidence(t *testing.T, root string, value any) {
	t.Helper()
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal("encode sanitized fixture evidence")
	}
	var identity struct {
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(encoded, &identity); err != nil || identity.TaskID == "" || strings.ContainsAny(identity.TaskID, `/\\`) {
		t.Fatal("fixture evidence has no safe task identity")
	}
	path := filepath.Join(root, "manual-teardown-evidence-"+identity.TaskID+".json")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		t.Fatal("create private fixture evidence file")
	}
	if _, err := file.Write(append(encoded, '\n')); err != nil {
		_ = file.Close()
		t.Fatal("write private fixture evidence")
	}
	if err := file.Close(); err != nil {
		t.Fatal("close private fixture evidence")
	}
}
