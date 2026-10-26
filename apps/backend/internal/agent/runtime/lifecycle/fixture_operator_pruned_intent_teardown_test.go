package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
	_ "github.com/mattn/go-sqlite3"
)

// TestFixtureOperatorPrunedIntentRestoreAndTeardown is a fixture-only,
// one-shot recovery of an early-crash record that was incorrectly pruned.
// It restores only a cleanup_pending row after checking the exact live Nomad
// job, allocation, process, worker fence, session, and environment owner. It
// then uses the original nonce once and delegates the drain/whole-job stop,
// absence, and fence-release gates to TestFixtureOperatorTeardownCurrent.
func TestFixtureOperatorPrunedIntentRestoreAndTeardown(t *testing.T) {
	if os.Getenv("FF_PRUNED_INTENT_CLEANUP") != "yes" || os.Getenv("FF_ALLOW_FIXTURE_OPERATOR_RECOVERY") != "yes" {
		t.Skip("fixture-only pruned-intent cleanup requires explicit opt-in")
	}
	inputPath := requiredFixtureEnv(t, "FF_PRUNED_INTENT_MANIFEST")
	input := readPrunedIntentInput(t, inputPath)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	verifyFixturePrivateRoot(t, input.FixtureRoot)
	verifyFixtureOwnedFileUnderRoot(t, input.FixtureRoot, input.DatabasePath, false)
	verifyFixturePrivateFileUnderRoot(t, input.FixtureRoot, input.SnapshotPath)
	verifyFixturePrivateFileUnderRoot(t, input.FixtureRoot, input.RepairSnapshotPath)
	verifyPrunedRepairSnapshotDigest(t, input)
	verifyFixtureOwnedFileUnderRoot(t, input.FixtureRoot, input.ProfileSourcePath, false)

	db, err := sqlx.Open("sqlite3", "file:"+input.DatabasePath+"?_busy_timeout=10000&_foreign_keys=on")
	if err != nil {
		t.Fatal("open exact isolated fixture database")
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	keyProvider, err := secrets.NewMasterKeyProvider(filepath.Dir(input.DatabasePath))
	if err != nil {
		t.Fatal("open exact fixture key context")
	}
	secretStore, closeSecretStore, err := secrets.Provide(db, db, keyProvider)
	if err != nil {
		t.Fatal("open exact fixture SecretStore")
	}
	defer func() { _ = closeSecretStore() }()

	var source struct {
		SchemaVersion int               `json:"schema_version"`
		ProfileID     string            `json:"profile_id"`
		ExecutorID    string            `json:"executor_id"`
		Config        map[string]string `json:"config"`
		NomadTokenRef string            `json:"nomad_token_ref"`
	}
	profileBytes, err := os.ReadFile(input.ProfileSourcePath)
	if err != nil || json.Unmarshal(profileBytes, &source) != nil || source.SchemaVersion != 1 || source.ProfileID != input.ExecutorProfileID || source.ExecutorID != input.ExecutorID {
		t.Fatal("exact original fixture profile source is unavailable")
	}
	profileSourceHash := sha256.Sum256(profileBytes)
	if hex.EncodeToString(profileSourceHash[:]) != input.ProfileSourceSHA256 || source.Config == nil || source.NomadTokenRef == "" || strings.Contains(source.NomadTokenRef, "***") {
		t.Fatal("profile source does not contain an original secret reference")
	}
	if err := verifyPrunedProfileProvenance(ctx, db, input, source); err != nil {
		t.Fatalf("canonical profile provenance did not match snapshot/current database: %v", err)
	}
	source.Config["nomad_token"] = source.NomadTokenRef
	profile, err := nomad.ParseProfileConfig(source.Config)
	if err != nil || profile.NodeID != input.NodeID || profile.Address == "" || profile.NomadTokenSecretID == "" {
		t.Fatal("original Nomad profile no longer matches the captured worker")
	}

	var session struct {
		TaskID          string `db:"task_id"`
		ID              string `db:"id"`
		State           string `db:"state"`
		ExecutionID     string `db:"execution_profile_id"`
		ExecutorID      string `db:"executor_id"`
		ExecutorProfile string `db:"executor_profile_id"`
		RepositoryID    string `db:"repository_id"`
		EnvironmentID   string `db:"task_environment_id"`
		StartedAt       string `db:"started_at"`
	}
	if err := db.GetContext(ctx, &session, `SELECT task_id, id, state, execution_profile_id, executor_id, executor_profile_id, repository_id, task_environment_id, started_at FROM task_sessions WHERE task_id = ? AND id = ?`, input.TaskID, input.SessionID); err != nil ||
		session.TaskID != input.TaskID || session.ID != input.SessionID || session.State != "CREATED" ||
		session.ExecutorID != input.ExecutorID || session.ExecutorProfile != input.ExecutorProfileID || session.EnvironmentID != input.EnvironmentID {
		t.Fatal("canonical task/session owner, profile, environment, or CREATED state changed")
	}
	var env struct {
		ID         string `db:"id"`
		TaskID     string `db:"task_id"`
		Generation int64  `db:"ownership_generation"`
		Type       string `db:"executor_type"`
		ExecutorID string `db:"executor_id"`
		ProfileID  string `db:"executor_profile_id"`
		Status     string `db:"status"`
	}
	if err := db.GetContext(ctx, &env, `SELECT id, task_id, ownership_generation, executor_type, executor_id, executor_profile_id, status FROM task_environments WHERE task_id = ?`, input.TaskID); err != nil ||
		env.ID != input.EnvironmentID || env.TaskID != input.TaskID || env.Generation != input.Generation || env.Type != string(models.ExecutorTypeNomad) ||
		env.ExecutorID != input.ExecutorID || env.ProfileID != input.ExecutorProfileID || env.Status != "creating" {
		t.Fatal("canonical environment owner, generation, executor, or creating state changed")
	}
	var existing int
	if err := db.GetContext(ctx, &existing, `SELECT count(*) FROM executors_running WHERE id = ? OR session_id = ? OR task_id = ?`, input.SessionID, input.SessionID, input.TaskID); err != nil || existing != 0 {
		t.Fatal("an executor inventory row already exists; refuse operator restore")
	}
	if existingSecret, err := secretStore.Get(ctx, kubernetesRuntimeSecretID(input.ExecutionID, "agentctl-auth")); err == nil && existingSecret != nil {
		t.Fatal("an unreferenced auth token exists; refuse to spend or replace credentials")
	} else if !errors.Is(err, secrets.ErrNotFound) {
		t.Fatal("could not establish that no auth token already exists")
	}
	bootstrapID := kubernetesRuntimeSecretID(input.ExecutionID, "agentctl-bootstrap")
	if bootstrapID != input.BootstrapSecretID {
		t.Fatal("bootstrap secret reference differs from the deterministic original operation reference")
	}
	if _, err := secretStore.Get(ctx, bootstrapID); err != nil {
		t.Fatal("original bootstrap secret record is unavailable")
	}
	nonce, err := secretStore.Reveal(ctx, bootstrapID)
	if err != nil || nonce == "" {
		t.Fatal("original bootstrap secret could not be revealed from the fixture key context")
	}
	nomadToken, err := secretStore.Reveal(ctx, profile.NomadTokenSecretID)
	if err != nil || nomadToken == "" {
		t.Fatal("original Nomad API secret could not be revealed from the fixture key context")
	}
	client, err := nomad.NewClient(profile.Address, nomadToken)
	if err != nil {
		t.Fatal("configure exact loopback Nomad API client")
	}
	defer client.CloseIdleConnections()

	job, err := client.GetJob(ctx, profile.Namespace, input.ExpectedJobID)
	if err != nil || job.ID != input.ExpectedJobID || job.Namespace != profile.Namespace || job.Region != profile.Region {
		t.Fatal("full Nomad job identity does not match the captured operation")
	}
	state, err := prunedIntentState(profile, job, input)
	if err != nil {
		t.Fatalf("full Nomad job state reconstruction failed: %v (allocation_root=%q agentctl_path=%q bootstrap_path=%q)", err, state.AllocationRoot, state.AgentctlPath, state.BootstrapPath)
	}
	if !jobMatchesState(job, state) {
		t.Fatal("full Nomad job policy does not match the immutable captured operation")
	}
	allocations, err := client.ListAllocations(ctx, state.Namespace, state.JobID)
	if err != nil || len(allocations) == 0 {
		t.Fatal("full Nomad allocation history is unavailable")
	}
	activeCount := 0
	var active *nomad.AllocationRecord
	for i := range allocations {
		full, getErr := client.GetAllocation(ctx, state.Namespace, allocations[i].ID)
		if getErr != nil || full.ID != allocations[i].ID || full.JobID != state.JobID || full.Namespace != state.Namespace || full.NodeID != state.NodeID || full.CreateIndex == 0 {
			t.Fatal("allocation history contains an unowned or incomplete record")
		}
		if len(full.Job) == 0 || nomad.ValidateNativeJobResponse(full.Job, job.Spec) != nil {
			t.Fatal("allocation embedded job definition differs from the original fixed policy")
		}
		if full.ClientStatus == "running" || full.DesiredStatus == "run" {
			if full.ID != input.ExpectedAllocationID || full.ClientStatus != "running" || full.DesiredStatus != "run" || full.TaskStates["agentctl"].State != "running" {
				t.Fatal("a successor or unexpected nonterminal allocation exists; preserve the fence")
			}
			activeCount++
			active = full
		} else if (full.ClientStatus != "complete" && full.ClientStatus != "failed") || full.TaskStates["agentctl"].State != "dead" {
			t.Fatal("unexpected nonterminal historical allocation exists; preserve the fence")
		}
	}
	if activeCount != 1 || active == nil || active.ID != input.ExpectedAllocationID {
		t.Fatal("exactly one captured running allocation is required")
	}
	// Quarantine this dedicated worker before restoring the missing row or
	// contacting agentctl. The only live allocation permitted on it is the
	// exact operation being drained below.
	if err := quarantinePrunedIntentNode(ctx, profile.Address, nomadToken, state.NodeID, state.JobID); err != nil {
		t.Fatal("could not prove and set exact fixture worker ineligible")
	}
	state.JobCreateIndex = job.CreateIndex
	state.JobDefinitionIndex = job.JobModifyIndex
	state.AllocationID = active.ID
	state.AllocationCreateIndex = active.CreateIndex
	state.ControlPort, state.InstancePort, err = prunedIntentAllocatedPorts(active)
	if err != nil {
		t.Fatal("captured allocation port identity is unavailable")
	}
	state.Revision = 3
	state.Phase = "cleanup_pending"
	if err := state.Validate(); err != nil {
		t.Fatalf("operator cleanup identity is invalid: %v", err)
	}
	if status, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID); err != nil || status != "ready" {
		t.Fatal("captured worker is not positively ready")
	}
	if err := verifyFixtureProcessIdentity(input.AgentctlPID, input.AgentctlStart, input.FixtureRoot, state.JobID); err != nil {
		t.Fatalf("captured runtime leader PID/start identity could not be verified: %v", err)
	}
	if !strings.HasPrefix(input.AgentctlRuntimePath, state.AllocationRoot+string(os.PathSeparator)) {
		t.Fatal("captured runtime executable is outside the admitted allocation root")
	}

	target, err := ResolveSSHTarget(SSHConnConfig{
		Host: profile.SSHHost, Port: profile.SSHPort, User: profile.SSHUser,
		IdentitySource: SSHIdentitySourceFile, IdentityFile: filepath.Join(input.FixtureRoot, "ssh", "client_ed25519"),
		PinnedFingerprint: profile.SSHHostFingerprint,
	})
	if err != nil {
		t.Fatal("resolve the captured pinned SSH identity")
	}
	sshClient, err := DialSSH(ctx, target)
	if err != nil {
		t.Fatal("connect to the captured fixture worker")
	}
	defer func() { _ = sshClient.Close() }()
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("worker-local fence does not belong to the exact captured allocation")
	}
	forward, err := StartPortForward(sshClient, state.ControlPort, newTestLogger())
	if err != nil {
		t.Fatal("open control forward to the exact fixture allocation")
	}
	defer func() { _ = forward.Close() }()
	if err := waitAgentctlHealthy(ctx, forward.LocalPort(), sshAgentctlHealthTimeout); err != nil {
		t.Fatal("exact fixture agentctl failed its control-health readiness check")
	}
	bootstrapControl := agentctl.NewControlClient("127.0.0.1", forward.LocalPort(), newTestLogger())
	defer bootstrapControl.Close()

	metadata := prunedIntentMetadata(source.Config, source.NomadTokenRef, state, input)
	claimID, claimBinding := newFixtureCleanupClaim(input)
	manual := map[string]any{
		"fixture_only": true, "automatic_recovery": false, "operator_cleanup_only": true,
		"task_id": input.TaskID, "session_id": input.SessionID, "execution_id": input.ExecutionID,
		"environment_id": input.EnvironmentID, "generation": input.Generation,
		"job_id": state.JobID, "allocation_id": state.AllocationID, "node_id": state.NodeID,
		"cleanup_claim_id": claimID, "cleanup_claim_binding_sha256": claimBinding,
		"node_ineligible": true, "node_quarantined_at": time.Now().UTC().Format(time.RFC3339Nano),
		"operator_restore_reason":       "original CREATED session inventory row was pruned after registration response loss",
		"nonce_handshake_attempt_count": 0, "pre_restore_db_snapshot": input.SnapshotPath,
	}
	metadata["fixture_operator_pruned_restore"] = manual
	if err := insertPrunedIntentCleanupRow(ctx, db, input, session, env, metadata); err != nil {
		t.Fatalf("quarantine exact pruned operation for cleanup: %v", err)
	}

	// The marker is durable before the one-shot credential leaves SecretStore.
	manual["nonce_handshake_attempt_count"] = 1
	manual["nonce_handshake_started_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	if err := updatePrunedIntentCleanupRow(ctx, db, input, metadata, manual); err != nil {
		t.Fatal("persist one-shot handshake marker for the cleanup-only row")
	}
	authToken, err := bootstrapControl.Handshake(ctx, nonce)
	if err != nil || authToken == "" {
		t.Fatal("sole original-nonce handshake failed; leave the cleanup row and fence quarantined")
	}
	authTokenID := kubernetesRuntimeSecretID(input.ExecutionID, "agentctl-auth")
	if err := secretStore.Create(ctx, &secrets.SecretWithValue{
		Secret: secrets.Secret{ID: authTokenID, Name: "fixture-agentctl-auth"}, Value: authToken,
	}); err != nil {
		t.Fatal("persist auth token returned by the original one-shot handshake")
	}
	metadata[MetadataKeyAuthTokenSecret] = authTokenID
	manual["auth_token_reference_persisted"] = true
	manual["handshake_completed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	// Persist the secret reference immediately. A later identity read failure
	// must not leave a durable token that the cleanup row cannot recover.
	if err := updatePrunedIntentCleanupRow(ctx, db, input, metadata, manual); err != nil {
		t.Fatal("CAS the returned auth reference before reading runtime identity")
	}
	control := agentctl.NewControlClient("127.0.0.1", forward.LocalPort(), newTestLogger(), agentctl.WithControlAuthToken(authToken))
	defer control.Close()
	identity, err := control.GetIdentity(ctx)
	if err != nil || identity == nil || identity.ServerIdentity == "" || !containsNomadCapability(identity.Capabilities, nomadDrainCapability) {
		t.Fatal("authenticated runtime identity or drain capability is unavailable")
	}
	state.ServerIdentity = identity.ServerIdentity
	encodedState, err := nomad.MarshalEnvironmentState(state)
	if err != nil {
		t.Fatal("encode exact cleanup-only runtime state")
	}
	metadata[nomad.EnvironmentStateMetadataKey] = json.RawMessage(encodedState)
	manual["server_identity"] = identity.ServerIdentity
	if err := updatePrunedIntentCleanupRow(ctx, db, input, metadata, manual); err != nil {
		t.Fatal("CAS exact server identity to the cleanup-only row")
	}

	row, err := readFixtureOperatorRow(ctx, db, input.TaskID)
	if err != nil || row.SessionID != input.SessionID || row.ExecutionID != input.ExecutionID || row.Status != "stopped" || row.Runtime != "nomad" {
		t.Fatal("restored cleanup row does not retain exact original operation ownership")
	}
	stateInRow := nomadStateFromFixtureRow(t, row)
	metadataBytes := []byte(row.Metadata)
	metadataDigest := sha256.Sum256(metadataBytes)
	identityManifest := fixtureIdentityManifest{
		SchemaVersion: 1, FixtureRoot: input.FixtureRoot, DatabasePath: input.DatabasePath,
		SessionConfigPath: input.SessionConfigPath, ExpectedSessionState: "CREATED",
		ExpectedRow: fixtureIdentityExpectedRow{
			ID: row.ID, TaskID: row.TaskID, SessionID: row.SessionID, ExecutionID: row.ExecutionID,
			ExecutorID: row.ExecutorID, Runtime: row.Runtime, Status: row.Status, UpdatedAt: row.UpdatedAt,
			MetadataSHA256: hex.EncodeToString(metadataDigest[:]),
		},
		ExpectedNomad: fixtureIdentityExpectedNomad{
			Phase: stateInRow.Phase, Revision: stateInRow.Revision, EnvironmentID: stateInRow.EnvironmentID,
			Generation: stateInRow.Generation, OperationID: stateInRow.OperationID, JobID: stateInRow.JobID,
			AllocationID: stateInRow.AllocationID, AllocationCreateIndex: stateInRow.AllocationCreateIndex,
			NodeID: stateInRow.NodeID, WorkspacePath: stateInRow.WorkspacePath, ServerIdentity: stateInRow.ServerIdentity,
		},
		ExpectedProcess: fixtureIdentityExpectedProc{
			AgentctlPID: input.AgentctlPID, AgentctlStart: input.AgentctlStart, AgentctlRuntimePath: input.AgentctlRuntimePath,
		},
	}
	writeFixtureJSONFile(t, input.SessionConfigPath, map[string]any{
		"fixture_only": true, "fixture_root": input.FixtureRoot,
		"session": map[string]any{
			"task_id": input.TaskID, "session_id": input.SessionID, "environment_id": input.EnvironmentID,
			"environment_generation": input.Generation, "job_id": state.JobID, "allocation_id": state.AllocationID,
			"node_id": state.NodeID, "workspace_path": state.WorkspacePath,
			"agentctl_pid": input.AgentctlPID, "agentctl_runtime_path": input.AgentctlRuntimePath,
		},
	})
	manifestPath := filepath.Join(input.FixtureRoot, "evidence", "early3-cleanup-identity.json")
	writeFixtureJSONFile(t, manifestPath, identityManifest)
	t.Setenv("FF_CURRENT_FIXTURE_TEARDOWN", "yes")
	t.Setenv("FF_ALLOW_FIXTURE_OPERATOR_RECOVERY", "yes")
	t.Setenv("FF_FIXTURE_IDENTITY_MANIFEST", manifestPath)
	TestFixtureOperatorTeardownCurrent(t)
}

// TestFixtureOperatorRepairPrunedIntentMetadata repairs only the serialization
// bug from the first cleanup-only attempt. It never re-handshakes or creates a
// credential; the exact existing auth reference must already resolve.
func TestFixtureOperatorRepairPrunedIntentMetadata(t *testing.T) {
	if os.Getenv("FF_PRUNED_INTENT_METADATA_REPAIR") != "yes" || os.Getenv("FF_ALLOW_FIXTURE_OPERATOR_RECOVERY") != "yes" {
		t.Skip("fixture-only metadata repair requires explicit opt-in")
	}
	inputPath := requiredFixtureEnv(t, "FF_PRUNED_INTENT_MANIFEST")
	input := readPrunedIntentInput(t, inputPath)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	verifyFixturePrivateRoot(t, input.FixtureRoot)
	verifyFixtureOwnedFileUnderRoot(t, input.FixtureRoot, input.DatabasePath, false)
	verifyFixturePrivateFileUnderRoot(t, input.FixtureRoot, input.SnapshotPath)
	verifyFixturePrivateFileUnderRoot(t, input.FixtureRoot, input.RepairSnapshotPath)
	verifyPrunedRepairSnapshotDigest(t, input)

	db, err := sqlx.Open("sqlite3", "file:"+input.DatabasePath+"?_busy_timeout=10000&_foreign_keys=on")
	if err != nil {
		t.Fatal("open exact isolated fixture database")
	}
	db.SetMaxOpenConns(1)
	defer func() { _ = db.Close() }()
	keyProvider, err := secrets.NewMasterKeyProvider(filepath.Dir(input.DatabasePath))
	if err != nil {
		t.Fatal("open exact fixture key context")
	}
	secretStore, closeStore, err := secrets.Provide(db, db, keyProvider)
	if err != nil {
		t.Fatal("open exact fixture SecretStore")
	}
	defer func() { _ = closeStore() }()

	var source struct {
		SchemaVersion int               `json:"schema_version"`
		ProfileID     string            `json:"profile_id"`
		ExecutorID    string            `json:"executor_id"`
		Config        map[string]string `json:"config"`
		NomadTokenRef string            `json:"nomad_token_ref"`
	}
	sourceBytes, err := os.ReadFile(input.ProfileSourcePath)
	if err != nil || json.Unmarshal(sourceBytes, &source) != nil || source.ProfileID != input.ExecutorProfileID || source.ExecutorID != input.ExecutorID {
		t.Fatal("canonical fixture profile source does not match manifest")
	}
	profileDigest := sha256.Sum256(sourceBytes)
	if hex.EncodeToString(profileDigest[:]) != input.ProfileSourceSHA256 || verifyPrunedProfileProvenance(ctx, db, input, source) != nil {
		t.Fatal("canonical fixture profile provenance changed")
	}
	if source.Config == nil || source.NomadTokenRef == "" || strings.Contains(source.NomadTokenRef, "***") {
		t.Fatal("canonical profile lacks the original Nomad token reference")
	}
	source.Config["nomad_token"] = source.NomadTokenRef
	profile, err := nomad.ParseProfileConfig(source.Config)
	wantNomadTokenID, tokenRefOK := models.ExecutorProfileSecretID(source.NomadTokenRef)
	if err != nil || profile.NodeID != input.NodeID || !tokenRefOK || profile.NomadTokenSecretID != wantNomadTokenID {
		t.Fatal("canonical profile does not resolve the captured Nomad identity")
	}

	row, err := readFixtureOperatorRow(ctx, db, input.TaskID)
	if err != nil || row.ID != input.SessionID || row.SessionID != input.SessionID || row.ExecutionID != input.ExecutionID || row.ExecutorID != input.ExecutorID || row.Runtime != "nomad" || row.Status != "stopped" {
		t.Fatal("restored cleanup row does not match the exact captured operation")
	}
	var owner struct {
		ID, Type, Status, ExecutorID, ProfileID string
		Generation                              int64
	}
	if err := db.QueryRowxContext(ctx, `SELECT id, executor_type, status, executor_id, executor_profile_id, ownership_generation FROM task_environments WHERE task_id = ?`, input.TaskID).Scan(&owner.ID, &owner.Type, &owner.Status, &owner.ExecutorID, &owner.ProfileID, &owner.Generation); err != nil || owner.ID != input.EnvironmentID || owner.Type != "nomad" || owner.Status != "creating" || owner.ExecutorID != input.ExecutorID || owner.ProfileID != input.ExecutorProfileID || owner.Generation != input.Generation {
		t.Fatal("canonical environment owner changed before metadata repair")
	}
	var sessionState string
	if err := db.GetContext(ctx, &sessionState, `SELECT state FROM task_sessions WHERE task_id = ? AND id = ? AND executor_id = ? AND executor_profile_id = ? AND task_environment_id = ?`, input.TaskID, input.SessionID, input.ExecutorID, input.ExecutorProfileID, input.EnvironmentID); err != nil || sessionState != "CREATED" {
		t.Fatal("canonical CREATED session changed before metadata repair")
	}
	var metadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(row.Metadata), &metadata); err != nil {
		t.Fatal("decode exact cleanup metadata")
	}
	var storedState nomad.EnvironmentState
	if err := json.Unmarshal(metadata[nomad.EnvironmentStateMetadataKey], &storedState); err != nil || storedState.Phase != "cleanup_pending" || storedState.JobID != input.ExpectedJobID || storedState.AllocationID != input.ExpectedAllocationID || storedState.NodeID != input.NodeID || storedState.EnvironmentID != input.EnvironmentID || storedState.Generation != input.Generation || storedState.OperationID != input.ExecutionID {
		t.Fatal("cleanup state does not match exact captured runtime identity")
	}
	var marker map[string]any
	if err := decodeFixturePrunedMarker(metadata["fixture_operator_pruned_restore"], &marker); err != nil {
		t.Fatal("cleanup claim marker encoding is not repairable")
	}
	if err := verifyFixtureCleanupClaim(input, marker); err != nil {
		t.Fatal("cleanup claim does not match captured operation")
	}
	if marker["auth_token_reference_persisted"] != true {
		t.Fatal("original auth reference persistence was not recorded")
	}
	authRef, err := decodeFixtureSecretReference(metadata[MetadataKeyAuthTokenSecret])
	wantAuthRef := kubernetesRuntimeSecretID(input.ExecutionID, "agentctl-auth")
	if err != nil || authRef != wantAuthRef {
		t.Fatal("existing auth reference does not match original operation")
	}
	if _, err := secretStore.Get(ctx, wantAuthRef); err != nil {
		t.Fatal("existing auth secret record is unavailable")
	}
	if marker["drain_receipt_digest"] != nil {
		t.Fatal("unexpected prior drain receipt; preserve state for manual review")
	}

	// Revalidate the live fixture in read-only mode immediately before any
	// metadata repair. This can be run alone with PREFLIGHT_ONLY and never
	// performs the nonce handshake or changes Nomad state.
	nomadToken, err := secretStore.Reveal(ctx, profile.NomadTokenSecretID)
	if err != nil || nomadToken == "" {
		t.Fatal("original Nomad API credential is unavailable")
	}
	nomadClient, err := nomad.NewClient(profile.Address, nomadToken)
	if err != nil {
		t.Fatal("configure captured loopback Nomad API")
	}
	defer nomadClient.CloseIdleConnections()
	job, err := nomadClient.GetJob(ctx, storedState.Namespace, storedState.JobID)
	if err != nil || job.ID != storedState.JobID || job.Namespace != storedState.Namespace || !jobMatchesState(job, storedState) {
		t.Fatal("live job does not match exact captured cleanup identity")
	}
	allocations, err := nomadClient.ListAllocations(ctx, storedState.Namespace, storedState.JobID)
	if err != nil || len(allocations) == 0 {
		t.Fatal("live allocation history is unavailable")
	}
	activeAllocations := 0
	for _, summary := range allocations {
		full, getErr := nomadClient.GetAllocation(ctx, storedState.Namespace, summary.ID)
		if getErr != nil || full.ID != summary.ID || full.JobID != storedState.JobID || full.NodeID != storedState.NodeID || len(full.Job) == 0 || nomad.ValidateNativeJobResponse(full.Job, job.Spec) != nil {
			t.Fatal("live allocation history contains an unverified record")
		}
		if full.ClientStatus == "running" || full.DesiredStatus == "run" {
			if full.ID != storedState.AllocationID || full.ClientStatus != "running" || full.DesiredStatus != "run" || full.TaskStates["agentctl"].State != "running" {
				t.Fatal("successor or unexpected active allocation exists")
			}
			activeAllocations++
		} else if (full.ClientStatus != "complete" && full.ClientStatus != "failed") || full.TaskStates["agentctl"].State != "dead" {
			t.Fatal("allocation history contains an unexpected nonterminal record")
		}
	}
	if activeAllocations != 1 {
		t.Fatal("exactly one captured allocation must remain active")
	}
	if nodeStatus, nodeEligibility, err := readPrunedIntentNodeStatus(ctx, profile.Address, nomadToken, storedState.NodeID); err != nil || nodeStatus != "ready" || nodeEligibility != "ineligible" {
		t.Fatal("captured worker is not positively ready and quarantined")
	}
	if err := verifyFixtureProcessIdentity(input.AgentctlPID, input.AgentctlStart, input.FixtureRoot, storedState.JobID); err != nil {
		t.Fatalf("captured runtime leader PID/start identity could not be verified: %v", err)
	}
	target, err := ResolveSSHTarget(SSHConnConfig{
		Host: profile.SSHHost, Port: profile.SSHPort, User: profile.SSHUser,
		IdentitySource: SSHIdentitySourceFile, IdentityFile: filepath.Join(input.FixtureRoot, "ssh", "client_ed25519"),
		PinnedFingerprint: profile.SSHHostFingerprint,
	})
	if err != nil {
		t.Fatal("resolve the exact pinned fixture SSH identity")
	}
	sshClient, err := DialSSH(ctx, target)
	if err != nil {
		t.Fatal("connect to the exact fixture worker")
	}
	defer func() { _ = sshClient.Close() }()
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, storedState); err != nil {
		t.Fatal("worker-local fence does not match the exact allocation")
	}
	if os.Getenv("FF_PRUNED_INTENT_REPAIR_PREFLIGHT_ONLY") == "yes" {
		t.Log("read-only row, credential-reference, Nomad job/allocation, node, process and worker-fence checks passed")
		return
	}

	// Normalize only the two proven JSON-[]byte mistakes. Every other metadata
	// byte is preserved. The update is CAS-bound to the entire original row.
	metadata["fixture_operator_pruned_restore"], _ = json.Marshal(marker)
	metadata[MetadataKeyAuthTokenSecret], _ = json.Marshal(authRef)
	encoded, err := json.Marshal(metadata)
	if err != nil {
		t.Fatal("encode corrected fixture metadata")
	}
	if err := repairPrunedIntentMetadataCAS(ctx, db, row, input, string(encoded)); err != nil {
		t.Fatalf("exact cleanup metadata CAS failed: %v", err)
	}

	repaired, err := readFixtureOperatorRow(ctx, db, input.TaskID)
	if err != nil || repaired.ID != row.ID || repaired.SessionID != row.SessionID || repaired.ExecutionID != row.ExecutionID || repaired.ExecutorID != row.ExecutorID || repaired.Status != row.Status {
		t.Fatal("corrected cleanup row identity changed")
	}
	var repairedMetadata map[string]json.RawMessage
	if err := json.Unmarshal([]byte(repaired.Metadata), &repairedMetadata); err != nil {
		t.Fatal("verify corrected metadata")
	}
	var checkMarker map[string]any
	if json.Unmarshal(repairedMetadata["fixture_operator_pruned_restore"], &checkMarker) != nil || verifyFixtureCleanupClaim(input, checkMarker) != nil {
		t.Fatal("corrected cleanup marker did not round-trip as a JSON object")
	}
	var checkAuthRef string
	if json.Unmarshal(repairedMetadata[MetadataKeyAuthTokenSecret], &checkAuthRef) != nil || checkAuthRef != wantAuthRef {
		t.Fatal("corrected auth reference did not round-trip as a JSON string")
	}
	if err := assertPrunedRepairSnapshotRow(ctx, input, row, repaired); err != nil {
		t.Fatalf("private pre-repair snapshot did not preserve prior row: %v", err)
	}
	manifest, sessionConfig := buildFixturePrunedCleanupIdentity(t, input, repaired, storedState)
	writeFixtureJSONFile(t, sessionConfig, map[string]any{
		"fixture_only": true, "fixture_root": input.FixtureRoot,
		"session": map[string]any{"task_id": input.TaskID, "session_id": input.SessionID, "environment_id": input.EnvironmentID,
			"environment_generation": input.Generation, "job_id": storedState.JobID, "allocation_id": storedState.AllocationID,
			"node_id": storedState.NodeID, "workspace_path": storedState.WorkspacePath,
			"agentctl_pid": input.AgentctlPID, "agentctl_runtime_path": input.AgentctlRuntimePath},
	})
	writeFixtureJSONFile(t, manifest, fixtureIdentityManifestForPruned(input, repaired, storedState, sessionConfig))
	t.Setenv("FF_CURRENT_FIXTURE_TEARDOWN", "yes")
	t.Setenv("FF_ALLOW_FIXTURE_OPERATOR_RECOVERY", "yes")
	t.Setenv("FF_FIXTURE_IDENTITY_MANIFEST", manifest)
	TestFixtureOperatorTeardownCurrent(t)
}
