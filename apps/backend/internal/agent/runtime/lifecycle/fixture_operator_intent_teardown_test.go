package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/secrets"
	_ "github.com/mattn/go-sqlite3"
)

// TestFixtureOperatorIntentTeardown is a one-time operator-only recovery for a
// failed registration-response-loss fixture. It never resumes or rewrites the
// failed operation. It authenticates the already-running exact allocation,
// seals/drains it, deregisters the whole immutable job without purge, proves
// all allocations/processes/ports are gone, and only then releases its fence.
func TestFixtureOperatorIntentTeardown(t *testing.T) {
	if os.Getenv("FF_INTENT_TEARDOWN") != "yes" {
		t.Skip("fixture teardown requires explicit operator opt-in")
	}
	if os.Getenv("FF_ALLOW_FIXTURE_OPERATOR_RECOVERY") != "yes" {
		t.Fatal("fixture teardown requires explicit fixture-only opt-in")
	}
	root := requiredFixtureEnv(t, "FF_FIXTURE_ROOT")
	taskID := requiredFixtureEnv(t, "FF_FIXTURE_TASK_ID")
	expectedJob := requiredFixtureEnv(t, "FF_EXPECTED_JOB_ID")
	expectedAllocation := requiredFixtureEnv(t, "FF_EXPECTED_ALLOCATION_ID")
	expectedNode := requiredFixtureEnv(t, "FF_EXPECTED_NODE_ID")
	expectedPID, err := strconv.Atoi(requiredFixtureEnv(t, "FF_EXPECTED_LEADER_PID"))
	if err != nil || expectedPID <= 1 {
		t.Fatal("fixture leader PID is invalid")
	}
	expectedStart := requiredFixtureEnv(t, "FF_EXPECTED_LEADER_START")
	verifyFixturePrivateRoot(t, root)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dbPath := filepath.Join(root, "kandev", "fresh2", "home", "data", "kandev.db")
	db, err := sqlx.Open("sqlite3", "file:"+dbPath+"?_busy_timeout=10000&_foreign_keys=on")
	if err != nil {
		t.Fatal("open isolated fixture database")
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	key, err := secrets.NewMasterKeyProvider(filepath.Dir(dbPath))
	if err != nil {
		t.Fatal("open isolated fixture secret key")
	}
	secretStore, closeStore, err := secrets.Provide(db, db, key)
	if err != nil {
		t.Fatal("open isolated fixture secret store")
	}
	defer func() { _ = closeStore() }()

	row, err := readFixtureOperatorRow(ctx, db, taskID)
	if err != nil || row.TaskID != taskID || row.SessionID == "" || row.ExecutionID == "" || row.Runtime != "nomad" || row.Status != "starting" {
		t.Fatal("exact failed fixture runtime row changed; preserve the operation")
	}
	var sessionState, environmentID, executorType, environmentStatus string
	var generation int64
	if err := db.GetContext(ctx, &sessionState, `SELECT state FROM task_sessions WHERE task_id = ? AND id = ?`, taskID, row.SessionID); err != nil || sessionState != "FAILED" {
		t.Fatal("fixture session is not the expected terminal FAILED state")
	}
	if err := db.QueryRowxContext(ctx, `SELECT id, executor_type, status, ownership_generation FROM task_environments WHERE task_id = ?`, taskID).
		Scan(&environmentID, &executorType, &environmentStatus, &generation); err != nil || executorType != "nomad" || environmentStatus != "failed" || generation != 1 {
		t.Fatal("fixture environment ownership or generation changed")
	}
	metadata, state := fixtureMetadataAndLegacyState(t, row.Metadata)
	if state.Phase != "intent" || state.AllocationID != "" || state.AllocationCreateIndex != 0 ||
		state.JobID != expectedJob || state.NodeID != expectedNode || state.EnvironmentID != environmentID ||
		state.Generation != generation || state.OperationID != row.ExecutionID || state.ExecutionID != row.ExecutionID ||
		state.WorkspacePath == "" || metadataHasMissingWorkspacePath(row.Metadata) {
		t.Fatal("fixture operation identity/phase changed; preserve the operation")
	}
	if _, exists := metadata["fixture_operator_intent_teardown"]; exists {
		t.Fatal("one-shot teardown marker already exists; do not repeat credential use")
	}
	bootstrapRef := fixtureRawString(t, metadata, MetadataKeyBootstrapNonceSecret)
	if bootstrapRef == "" || fixtureRawString(t, metadata, MetadataKeyAuthTokenSecret) != "" {
		t.Fatal("original nonce reference is unavailable or an auth token already exists; preserve the fixture")
	}
	authTokenID := kubernetesRuntimeSecretID(row.ExecutionID, "agentctl-auth")
	if _, err := secretStore.Get(ctx, authTokenID); err == nil {
		t.Fatal("unreferenced auth token already exists; do not spend the original nonce")
	} else if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("inspect deterministic auth token reference")
	}
	nonce, err := secretStore.Reveal(ctx, bootstrapRef)
	if err != nil || nonce == "" {
		t.Fatal("reveal original bootstrap reference")
	}
	profile, err := nomadProfileFromMetadata(metadataAsAny(metadata))
	if err != nil || profile.NodeID != state.NodeID || profile.Address == "" || profile.NomadTokenSecretID == "" {
		t.Fatal("exact fixture Nomad profile is unavailable")
	}
	nomadToken, err := secretStore.Reveal(ctx, profile.NomadTokenSecretID)
	if err != nil || nomadToken == "" {
		t.Fatal("reveal fixture Nomad API credential")
	}
	client, err := nomad.NewClient(profile.Address, nomadToken)
	if err != nil {
		t.Fatal("configure loopback fixture Nomad client")
	}
	defer client.CloseIdleConnections()
	job, err := client.GetJob(ctx, state.Namespace, state.JobID)
	if err != nil || !jobMatchesState(job, state) || job.CreateIndex == 0 || job.JobModifyIndex == 0 {
		t.Fatal("complete immutable Nomad job identity/policy is unavailable")
	}
	allocations, err := client.ListAllocations(ctx, state.Namespace, state.JobID)
	if err != nil || len(allocations) != 1 || allocations[0].ID != expectedAllocation {
		t.Fatal("fixture job has an unexpected allocation set; preserve it")
	}
	allocation, err := client.GetAllocation(ctx, state.Namespace, expectedAllocation)
	if err != nil || allocation.ID != expectedAllocation || allocation.JobID != state.JobID ||
		allocation.Namespace != state.Namespace || allocation.NodeID != state.NodeID || allocation.NodeID != expectedNode ||
		allocation.CreateIndex == 0 || allocation.DesiredStatus != "run" || allocation.ClientStatus != "running" ||
		allocation.TaskStates["agentctl"].State != "running" {
		t.Fatal("full allocation owner/status changed; preserve the fixture")
	}
	if err := nomad.ValidateNativeJobResponse(allocation.Job, job.Spec); err != nil {
		t.Fatalf("allocation's embedded job policy is not the immutable expected policy: %v", err)
	}
	if status, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID); err != nil || status != "ready" {
		t.Fatal("fixture worker is not positively ready")
	}
	if err := verifyFixtureProcessIdentity(expectedPID, expectedStart, root, state.JobID); err != nil {
		t.Fatal("exact fixture leader PID/start identity did not match")
	}
	ports := map[string]int{}
	if allocation.AllocatedResources == nil {
		t.Fatal("exact allocated runtime ports are unavailable")
	}
	for _, port := range allocation.AllocatedResources.Shared.Ports {
		if port.Label == "agentctl_control" || port.Label == "agentctl_instance" {
			if port.Value < 1 || ports[port.Label] != 0 {
				t.Fatal("allocated runtime port identity is invalid")
			}
			ports[port.Label] = port.Value
		}
	}
	if ports["agentctl_control"] == 0 || ports["agentctl_instance"] == 0 || ports["agentctl_control"] == ports["agentctl_instance"] {
		t.Fatal("both exact runtime ports are required")
	}
	state.AllocationID, state.AllocationCreateIndex = allocation.ID, allocation.CreateIndex
	state.JobCreateIndex, state.JobDefinitionIndex = job.CreateIndex, job.JobModifyIndex
	state.ControlPort, state.InstancePort, state.Phase = ports["agentctl_control"], ports["agentctl_instance"], "allocated"
	if err := state.Validate(); err != nil {
		t.Fatalf("bound fixture identity is invalid: %v", err)
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

	// Persist the one-shot marker before contacting the runtime. Any ambiguity
	// after this point deliberately prevents another nonce attempt.
	marker := map[string]any{
		"fixture_only": true, "task_id": taskID, "session_id": row.SessionID,
		"execution_id": row.ExecutionID, "environment_id": environmentID, "generation": generation,
		"job_id": state.JobID, "allocation_id": state.AllocationID, "node_id": state.NodeID,
		"handshake_attempts": 1, "handshake_started_at": time.Now().UTC().Format(time.RFC3339Nano),
	}
	row, err = fixtureIntentTeardownCAS(ctx, db, row, environmentID, metadata, marker)
	if err != nil {
		t.Fatal("persist one-shot manual recovery marker")
	}
	metadata, _ = fixtureMetadataAndLegacyState(t, row.Metadata)

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
		t.Fatal("sole original-nonce handshake failed; preserve marker, job, and fence")
	}
	if err := secretStore.Create(ctx, &secrets.SecretWithValue{Secret: secrets.Secret{ID: authTokenID, Name: "fixture-agentctl-auth"}, Value: authToken}); err != nil {
		t.Fatal("persist returned original-operation auth token")
	}
	metadata[MetadataKeyAuthTokenSecret], _ = json.Marshal(authTokenID)
	marker["auth_token_reference_persisted"] = true
	marker["handshake_completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	row, err = fixtureIntentTeardownCAS(ctx, db, row, environmentID, metadata, marker)
	if err != nil {
		t.Fatal("CAS auth-token reference to exact failed operation")
	}
	metadata, _ = fixtureMetadataAndLegacyState(t, row.Metadata)

	control := agentctl.NewControlClient("127.0.0.1", forward.LocalPort(), newTestLogger(), agentctl.WithControlAuthToken(authToken))
	defer control.Close()
	identity, err := control.GetIdentity(ctx)
	if err != nil || identity == nil || identity.ServerIdentity == "" || !containsNomadCapability(identity.Capabilities, nomadDrainCapability) {
		t.Fatal("authenticated exact-runtime identity or drain capability unavailable")
	}
	requestID := "manual-intent-fixture-drain-" + state.AllocationID
	receipt, err := control.SealAndDrainRuntime(ctx, agentctl.RuntimeDrainRequest{
		RequestID: requestID, ExpectedServerIdentity: identity.ServerIdentity,
		DeadlineMS: int64(30 * time.Second / time.Millisecond),
	})
	if err != nil || receipt == nil || receipt.RequestID != requestID || receipt.ServerIdentity != identity.ServerIdentity || !receipt.Sealed || !receipt.Drained {
		t.Fatal("authenticated exact-runtime seal-and-drain did not return positive evidence")
	}
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("encode validated drain receipt")
	}
	receiptSum := sha256.Sum256(receiptBytes)
	receiptDigest := hex.EncodeToString(receiptSum[:])
	marker["drain_receipt_digest"] = receiptDigest
	marker["drain_request_id"] = receipt.RequestID
	marker["server_identity"] = identity.ServerIdentity
	marker["drain_completed_at"] = receipt.CompletedAt.UTC().Format(time.RFC3339Nano)
	row, err = fixtureIntentTeardownCAS(ctx, db, row, environmentID, metadata, marker)
	if err != nil {
		t.Fatal("CAS drain receipt digest to exact failed operation")
	}

	if err := deregisterFixtureJobWithoutPurge(ctx, profile.Address, nomadToken, state); err != nil {
		t.Fatal("deregister exact immutable fixture job without purge")
	}
	terminal, err := waitFixtureJobTerminal(ctx, db, profile.Address, nomadToken, state, taskID, row.SessionID)
	if err != nil || len(terminal) == 0 {
		t.Fatal("whole-job deregistration did not yield terminal history for every allocation")
	}
	if status, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID); err != nil || status != "ready" {
		t.Fatal("fixture node not positively ready after teardown")
	}
	assertFixturePIDGone(t, expectedPID, expectedStart, root, state.JobID)
	if err := assertFixturePortsClosed(ctx, sshClient, state.ControlPort, state.InstancePort); err != nil {
		t.Fatal("owned runtime ports remain open after job exit")
	}
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("fence owner changed before explicit release")
	}
	if err := releaseExactFixtureFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("exact owner compare-and-delete of fixture fence failed")
	}
	var afterState string
	if err := db.GetContext(ctx, &afterState, `SELECT state FROM task_sessions WHERE task_id = ? AND id = ?`, taskID, row.SessionID); err != nil || afterState != "FAILED" {
		t.Fatal("failed task/session history changed during operator cleanup")
	}
	marker["session_state_preserved"] = afterState
	marker["terminal_allocations"] = terminal
	marker["agentctl_leader_absent"] = true
	marker["agentctl_or_acp_process_count"] = 0
	marker["runtime_ports_closed"] = true
	marker["fence_released"] = true
	marker["completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	metadata, stateAfter := fixtureMetadataAndLegacyState(t, row.Metadata)
	if stateAfter.Phase != "intent" || stateAfter.JobID != expectedJob || stateAfter.WorkspacePath != state.WorkspacePath {
		t.Fatal("immutable failed operation metadata changed unexpectedly")
	}
	row, err = fixtureIntentTeardownCAS(ctx, db, row, environmentID, metadata, marker)
	if err != nil {
		t.Fatal("persist final manual recovery receipt")
	}
	writeFixtureEvidence(t, root, map[string]any{
		"fixture_only": true, "task_id": taskID, "session_id": row.SessionID,
		"execution_id": row.ExecutionID, "environment_id": environmentID,
		"generation": generation, "job_id": state.JobID, "allocation_id": state.AllocationID,
		"allocation_create_index": state.AllocationCreateIndex, "node_id": state.NodeID,
		"server_identity": identity.ServerIdentity, "workspace_path": state.WorkspacePath,
		"drain_receipt_digest": receiptDigest, "terminal_allocations": terminal,
		"processes_for_job": 0, "ports_closed": true, "worker_ready": true,
		"fence_released": true, "session_state_preserved": "FAILED",
		"completed_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
	t.Logf("fixture-only operator teardown passed: task=%s job=%s allocation=%s terminal=%d session=FAILED fence_released=true", taskID, state.JobID, state.AllocationID, len(terminal))
}

func fixtureIntentTeardownCAS(ctx context.Context, db *sqlx.DB, previous fixtureOperatorRow, expectedEnvironmentID string, metadata map[string]json.RawMessage, marker map[string]any) (fixtureOperatorRow, error) {
	_, err := json.Marshal(marker)
	if err != nil {
		return fixtureOperatorRow{}, err
	}
	metadata["fixture_operator_intent_teardown"], _ = json.Marshal(marker)
	encodedMetadata, err := json.Marshal(metadata)
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
		Scan(&environmentID, &executorType, &environmentStatus, &generation); err != nil || environmentID != expectedEnvironmentID || executorType != "nomad" || environmentStatus != "failed" || generation != 1 {
		return fixtureOperatorRow{}, errors.New("fixture operator CAS owner/generation/state changed")
	}
	if environmentID == "" {
		return fixtureOperatorRow{}, errors.New("fixture operator CAS environment identity missing")
	}
	result, err := tx.ExecContext(ctx, `UPDATE executors_running SET metadata = ?, updated_at = ?
		WHERE id = ? AND session_id = ? AND task_id = ? AND executor_id = ? AND agent_execution_id = ? AND runtime = 'nomad'
		AND status = 'starting' AND updated_at = ? AND metadata = ?`, string(encodedMetadata), time.Now().UTC(),
		previous.ID, previous.SessionID, previous.TaskID, previous.ExecutorID, previous.ExecutionID, previous.UpdatedAt, previous.Metadata)
	if err != nil {
		return fixtureOperatorRow{}, err
	}
	count, err := result.RowsAffected()
	if err != nil || count != 1 {
		return fixtureOperatorRow{}, errors.New("fixture operator row CAS did not affect exactly one row")
	}
	if err := tx.Commit(); err != nil {
		return fixtureOperatorRow{}, err
	}
	return readFixtureOperatorRow(ctx, db, previous.TaskID)
}
