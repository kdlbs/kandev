package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/secrets"
	_ "github.com/mattn/go-sqlite3"
)

// TestFixtureOperatorTeardownCurrent is a fixture-only manual recovery path
// for a captured, fixture-owned Nomad runtime. Its eligibility is determined
// by the exact row/session snapshot in the private manifest, not a fixed task,
// status, phase, or PID. It preserves task/session history and never makes the
// row eligible for automatic recovery. Every destructive action is identity-gated.
func TestFixtureOperatorTeardownCurrent(t *testing.T) {
	if os.Getenv("FF_CURRENT_FIXTURE_TEARDOWN") != "yes" {
		t.Skip("fixture teardown requires explicit operator opt-in")
	}
	if os.Getenv("FF_ALLOW_FIXTURE_OPERATOR_RECOVERY") != "yes" {
		t.Fatal("fixture teardown requires explicit fixture-only opt-in")
	}
	manifestPath := requiredFixtureEnv(t, "FF_FIXTURE_IDENTITY_MANIFEST")
	manifest := readFixtureIdentityManifest(t, manifestPath)
	root := manifest.FixtureRoot
	taskID := manifest.ExpectedRow.TaskID
	sessionID := manifest.ExpectedRow.SessionID
	jobID := manifest.ExpectedNomad.JobID
	allocationID := manifest.ExpectedNomad.AllocationID
	nodeID := manifest.ExpectedNomad.NodeID
	verifyFixturePrivateRoot(t, root)
	verifyFixtureOwnedFileUnderRoot(t, root, manifest.DatabasePath, false)
	verifyFixturePrivateFileUnderRoot(t, root, manifest.SessionConfigPath)
	verifyFixtureSessionConfigMatchesManifest(t, manifest)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	dbPath := manifest.DatabasePath
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
	if err != nil || !fixtureRowMatchesManifest(row, manifest.ExpectedRow) {
		t.Fatal("fixture runtime row differs from the captured exact row revision or metadata")
	}
	var sessionState string
	if err := db.GetContext(ctx, &sessionState, `SELECT state FROM task_sessions WHERE task_id = ? AND id = ?`, taskID, sessionID); err != nil || sessionState != manifest.ExpectedSessionState {
		t.Fatal("fixture session state differs from the captured identity manifest")
	}
	metadata, state := fixtureMetadataAndLegacyState(t, row.Metadata)
	if !fixtureNomadStateMatchesManifest(state, manifest.ExpectedNomad, row.ExecutionID) || state.JobID != jobID || state.AllocationID != allocationID || state.NodeID != nodeID {
		t.Fatal("current immutable fixture execution identity changed")
	}
	var envOwner, envType, envStatus string
	var generation int64
	if err := db.QueryRowxContext(ctx, `SELECT id, executor_type, status, ownership_generation FROM task_environments WHERE task_id = ?`, taskID).
		Scan(&envOwner, &envType, &envStatus, &generation); err != nil || envOwner != state.EnvironmentID || envType != "nomad" || generation != state.Generation {
		t.Fatal("current fixture environment owner/generation changed")
	}
	if _, exists := metadata["fixture_operator_current_teardown"]; exists {
		t.Fatal("fixture teardown marker already exists; preserve prior evidence")
	}
	cleanupClaimID := ""
	var prunedInput prunedIntentCleanupInput
	if raw := metadata["fixture_operator_pruned_restore"]; len(raw) > 0 {
		var marker map[string]any
		if err := json.Unmarshal(raw, &marker); err != nil {
			t.Fatal("decode fixture cleanup claim")
		}
		prunedInput = prunedIntentCleanupInput{
			TaskID: taskID, SessionID: sessionID, ExecutionID: state.ExecutionID,
			ExecutorID: row.ExecutorID, ExecutorProfileID: fixtureRawString(t, metadata, "executor_profile_id"),
			EnvironmentID: state.EnvironmentID, Generation: state.Generation,
			ExpectedJobID: state.JobID, ExpectedAllocationID: state.AllocationID, NodeID: state.NodeID,
		}
		if err := verifyFixtureCleanupClaim(prunedInput, marker); err != nil {
			t.Fatalf("fixture cleanup claim did not bind the current row and execution: %v", err)
		}
		if marker["node_ineligible"] != true {
			t.Fatal("fixture cleanup claim lacks node quarantine evidence")
		}
		cleanupClaimID, _ = marker["cleanup_claim_id"].(string)
	}
	profile, err := nomadProfileFromMetadata(metadataAsAny(metadata))
	if err != nil || profile.NodeID != nodeID || profile.Address == "" || profile.NomadTokenSecretID == "" {
		t.Fatal("exact fixture Nomad profile is unavailable")
	}
	authRef := fixtureRawString(t, metadata, MetadataKeyAuthTokenSecret)
	if authRef == "" {
		t.Fatal("original agentctl auth token reference is unavailable; preserve fixture")
	}
	authToken, err := secretStore.Reveal(ctx, authRef)
	if err != nil || authToken == "" {
		t.Fatal("reveal exact fixture auth token reference")
	}
	nomadToken, err := secretStore.Reveal(ctx, profile.NomadTokenSecretID)
	if err != nil || nomadToken == "" {
		t.Fatal("reveal exact fixture Nomad API token reference")
	}
	client, err := nomad.NewClient(profile.Address, nomadToken)
	if err != nil {
		t.Fatal("configure loopback fixture Nomad client")
	}
	defer client.CloseIdleConnections()
	job, err := client.GetJob(ctx, state.Namespace, state.JobID)
	if err != nil || !jobMatchesState(job, state) {
		t.Fatal("exact immutable fixture Nomad job is unavailable")
	}
	allocs, err := client.ListAllocations(ctx, state.Namespace, state.JobID)
	if err != nil || len(allocs) == 0 {
		t.Fatal("fixture allocation history is unavailable")
	}
	seenExpected := false
	for _, summary := range allocs {
		full, getErr := client.GetAllocation(ctx, state.Namespace, summary.ID)
		if getErr != nil || full.ID != summary.ID || full.JobID != state.JobID || full.Namespace != state.Namespace || full.NodeID != state.NodeID {
			t.Fatal("fixture allocation history has an unexpected owner; preserve it")
		}
		if full.ID == state.AllocationID && full.CreateIndex == state.AllocationCreateIndex {
			seenExpected = true
			if full.ClientStatus != "running" || full.TaskStates["agentctl"].State != "running" {
				t.Fatal("expected current allocation is not running")
			}
		}
	}
	if !seenExpected {
		t.Fatal("original full allocation identity is absent")
	}
	if cleanupClaimID != "" {
		if err := quarantinePrunedIntentNode(ctx, profile.Address, nomadToken, state.NodeID, state.JobID); err != nil {
			t.Fatal("dedicated fixture worker lost its ineligible quarantine")
		}
	}
	if status, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID); err != nil || status != "ready" {
		t.Fatal("fixture worker is not positively ready")
	}
	if err := verifyFixtureCurrentProcessIdentity(state, manifest.ExpectedProcess); err != nil {
		t.Fatal("exact current agentctl process identity did not match")
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
		t.Fatal("exact worker allocation fence does not match")
	}
	forward, err := StartPortForward(sshClient, state.ControlPort, newTestLogger())
	if err != nil {
		t.Fatal("open exact worker control forward")
	}
	defer func() { _ = forward.Close() }()
	if err := waitAgentctlHealthy(ctx, forward.LocalPort(), sshAgentctlHealthTimeout); err != nil {
		t.Fatal("exact agentctl did not pass control-health readiness")
	}
	control := agentctl.NewControlClient("127.0.0.1", forward.LocalPort(), newTestLogger(), agentctl.WithControlAuthToken(authToken))
	defer control.Close()
	identity, err := control.GetIdentity(ctx)
	if err != nil || identity == nil || identity.ServerIdentity == "" || identity.ServerIdentity != state.ServerIdentity || !containsNomadCapability(identity.Capabilities, nomadDrainCapability) {
		t.Fatal("authenticated server identity does not match retained runtime")
	}
	receipt, err := control.SealAndDrainRuntime(ctx, agentctl.RuntimeDrainRequest{
		RequestID:              "manual-current-fixture-drain-" + state.AllocationID,
		ExpectedServerIdentity: identity.ServerIdentity,
		DeadlineMS:             int64(30 * time.Second / time.Millisecond),
	})
	if err != nil || receipt == nil || receipt.ServerIdentity != identity.ServerIdentity || receipt.RequestID != "manual-current-fixture-drain-"+state.AllocationID {
		t.Fatal("authenticated runtime seal-and-drain did not return exact positive evidence")
	}
	receiptBytes, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal("encode validated drain receipt")
	}
	receiptDigestBytes := sha256.Sum256(receiptBytes)
	receiptDigest := hex.EncodeToString(receiptDigestBytes[:])
	if state.DrainReceiptDigest != "" && state.DrainReceiptDigest != receiptDigest {
		t.Fatal("retained drain receipt conflicts with the exact runtime")
	}
	if cleanupClaimID != "" {
		if err := persistFixturePrunedDrainReceipt(ctx, db, row, prunedInput, receiptDigest); err != nil {
			t.Fatal("persist drain receipt against the exact cleanup claim before stopping Nomad")
		}
	}

	if err := deregisterFixtureJobWithoutPurge(ctx, profile.Address, nomadToken, state); err != nil {
		t.Fatal("deregister exact immutable fixture job without purge")
	}
	terminal, err := waitCurrentFixtureJobTerminal(ctx, profile.Address, nomadToken, client, state)
	if err != nil || len(terminal) == 0 {
		t.Fatal("whole-job stop did not yield positively terminal allocation history")
	}
	if status, err := getNomadNodeStatus(ctx, profile.Address, nomadToken, state.NodeID); err != nil || status != "ready" {
		t.Fatal("fixture worker is not positively ready after job stop")
	}
	if err := assertFixturePIDAbsentExact(state, manifest.ExpectedProcess); err != nil {
		t.Fatal("tracked fixture agentctl PID is not positively absent")
	}
	if fixtureJobProcessCount(t, root, state.JobID) != 0 {
		t.Fatal("fixture agentctl or inert ACP process remains for exact job")
	}
	if err := assertFixturePortsClosed(ctx, sshClient, state.ControlPort, state.InstancePort); err != nil {
		t.Fatal("reserved runtime ports remain open")
	}
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("fixture fence changed before exact release")
	}
	if err := releaseExactFixtureFence(ctx, sshClient, profile, state); err != nil {
		t.Fatal("exact compare-and-delete of fixture fence failed")
	}
	var afterState string
	if err := db.GetContext(ctx, &afterState, `SELECT state FROM task_sessions WHERE task_id = ? AND id = ?`, taskID, sessionID); err != nil || afterState != manifest.ExpectedSessionState {
		t.Fatal("fixture task/session state changed from its captured identity")
	}
	writeFixtureEvidence(t, root, map[string]any{
		"fixture_only": true, "task_id": taskID, "session_id": sessionID,
		"execution_id": state.ExecutionID, "environment_id": state.EnvironmentID,
		"generation": state.Generation, "job_id": state.JobID,
		"original_allocation_id": state.AllocationID, "node_id": state.NodeID,
		"server_identity": identity.ServerIdentity, "workspace_path": state.WorkspacePath,
		"cleanup_claim_id":        cleanupClaimID,
		"session_state_preserved": afterState, "drain_receipt_digest": receiptDigest,
		"allocations": terminal, "agentctl_leader_absent": true,
		"agentctl_or_acp_processes_for_job": 0, "reserved_ports_closed": true,
		"worker_ready": true, "fence_released": true,
		"completed_at": time.Now().UTC().Format(time.RFC3339Nano),
	})
	t.Logf("fixture-only current teardown passed: task=%s session=%s job=%s allocation_history=%d preserved_state=%s fence_released=true", taskID, sessionID, state.JobID, len(terminal), afterState)
}

type fixtureIdentityManifest struct {
	SchemaVersion        int                          `json:"schema_version"`
	FixtureRoot          string                       `json:"fixture_root"`
	DatabasePath         string                       `json:"database_path"`
	SessionConfigPath    string                       `json:"session_config_path"`
	ExpectedSessionState string                       `json:"expected_session_state"`
	ExpectedRow          fixtureIdentityExpectedRow   `json:"expected_row"`
	ExpectedNomad        fixtureIdentityExpectedNomad `json:"expected_nomad"`
	ExpectedProcess      fixtureIdentityExpectedProc  `json:"expected_process"`
}

type fixtureIdentityExpectedRow struct {
	ID             string `json:"id"`
	TaskID         string `json:"task_id"`
	SessionID      string `json:"session_id"`
	ExecutionID    string `json:"execution_id"`
	ExecutorID     string `json:"executor_id"`
	Runtime        string `json:"runtime"`
	Status         string `json:"status"`
	UpdatedAt      string `json:"updated_at"`
	MetadataSHA256 string `json:"metadata_sha256"`
}

type fixtureIdentityExpectedNomad struct {
	Phase                 string `json:"phase"`
	Revision              uint64 `json:"revision"`
	EnvironmentID         string `json:"environment_id"`
	Generation            int64  `json:"generation"`
	OperationID           string `json:"operation_id"`
	JobID                 string `json:"job_id"`
	AllocationID          string `json:"allocation_id"`
	AllocationCreateIndex uint64 `json:"allocation_create_index"`
	NodeID                string `json:"node_id"`
	WorkspacePath         string `json:"workspace_path"`
	ServerIdentity        string `json:"server_identity"`
}

type fixtureIdentityExpectedProc struct {
	AgentctlPID         int    `json:"agentctl_pid"`
	AgentctlStart       string `json:"agentctl_start"`
	AgentctlRuntimePath string `json:"agentctl_runtime_path"`
}

func readFixtureIdentityManifest(t *testing.T, path string) fixtureIdentityManifest {
	t.Helper()
	if !filepath.IsAbs(path) {
		t.Fatal("fixture identity manifest path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		t.Fatal("fixture identity manifest must be a private non-symlink file")
	}
	owner, ok := info.Sys().(*syscall.Stat_t)
	if !ok || owner.Uid != uint32(os.Getuid()) {
		t.Fatal("fixture identity manifest must be owned by the current user")
	}
	var manifest fixtureIdentityManifest
	contents, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(contents, &manifest) != nil || manifest.SchemaVersion != 1 {
		t.Fatal("fixture identity manifest is invalid")
	}
	if manifest.FixtureRoot == "" || manifest.DatabasePath == "" || manifest.SessionConfigPath == "" || manifest.ExpectedSessionState == "" {
		t.Fatal("fixture identity manifest is incomplete")
	}
	manifestRoot := filepath.Clean(manifest.FixtureRoot)
	relativeManifest, relErr := filepath.Rel(manifestRoot, filepath.Clean(path))
	if !filepath.IsAbs(manifestRoot) || relErr != nil || relativeManifest == ".." || strings.HasPrefix(relativeManifest, ".."+string(os.PathSeparator)) {
		t.Fatal("fixture identity manifest is outside its fixture root")
	}
	row := manifest.ExpectedRow
	if row.ID == "" || row.TaskID == "" || row.SessionID == "" || row.ExecutionID == "" || row.ExecutorID == "" || row.Runtime != "nomad" || row.Status == "" || row.UpdatedAt == "" || len(row.MetadataSHA256) != sha256.Size*2 {
		t.Fatal("fixture identity manifest row snapshot is incomplete")
	}
	if _, err := hex.DecodeString(row.MetadataSHA256); err != nil {
		t.Fatal("fixture identity manifest metadata digest is invalid")
	}
	n := manifest.ExpectedNomad
	if n.Phase == "" || n.Revision < 1 || n.EnvironmentID == "" || n.Generation < 1 || n.OperationID == "" || n.JobID == "" || n.AllocationID == "" || n.AllocationCreateIndex == 0 || n.NodeID == "" || n.WorkspacePath == "" || n.ServerIdentity == "" {
		t.Fatal("fixture identity manifest Nomad snapshot is incomplete")
	}
	p := manifest.ExpectedProcess
	if p.AgentctlPID <= 1 || p.AgentctlStart == "" || p.AgentctlRuntimePath == "" {
		t.Fatal("fixture identity manifest process identity is incomplete")
	}
	if n.OperationID != row.ExecutionID {
		t.Fatal("fixture identity manifest operation mapping is inconsistent")
	}
	return manifest
}

func verifyFixtureSessionConfigMatchesManifest(t *testing.T, manifest fixtureIdentityManifest) {
	t.Helper()
	data, err := os.ReadFile(manifest.SessionConfigPath)
	if err != nil {
		t.Fatal("read fixture session identity config")
	}
	var config struct {
		FixtureOnly bool   `json:"fixture_only"`
		FixtureRoot string `json:"fixture_root"`
		Session     struct {
			TaskID              string `json:"task_id"`
			SessionID           string `json:"session_id"`
			EnvironmentID       string `json:"environment_id"`
			EnvironmentGen      int64  `json:"environment_generation"`
			JobID               string `json:"job_id"`
			AllocationID        string `json:"allocation_id"`
			NodeID              string `json:"node_id"`
			WorkspacePath       string `json:"workspace_path"`
			AgentctlPID         int    `json:"agentctl_pid"`
			AgentctlRuntimePath string `json:"agentctl_runtime_path"`
		} `json:"session"`
	}
	if json.Unmarshal(data, &config) != nil || !config.FixtureOnly || filepath.Clean(config.FixtureRoot) != filepath.Clean(manifest.FixtureRoot) {
		t.Fatal("fixture session config is invalid or belongs to another root")
	}
	row, n, p := manifest.ExpectedRow, manifest.ExpectedNomad, manifest.ExpectedProcess
	if config.Session.TaskID != row.TaskID || config.Session.SessionID != row.SessionID ||
		config.Session.EnvironmentID != n.EnvironmentID || config.Session.EnvironmentGen != n.Generation ||
		config.Session.JobID != n.JobID || config.Session.AllocationID != n.AllocationID ||
		config.Session.NodeID != n.NodeID || config.Session.WorkspacePath != n.WorkspacePath ||
		config.Session.AgentctlPID != p.AgentctlPID || config.Session.AgentctlRuntimePath != p.AgentctlRuntimePath {
		t.Fatal("fixture session config does not match the captured immutable identity")
	}
}

func verifyFixturePrivateFileUnderRoot(t *testing.T, root, path string) {
	verifyFixtureOwnedFileUnderRoot(t, root, path, true)
}

func verifyFixtureOwnedFileUnderRoot(t *testing.T, root, path string, requirePrivateMode bool) {
	t.Helper()
	if !filepath.IsAbs(path) {
		t.Fatal("fixture input path must be absolute")
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 || (requirePrivateMode && info.Mode().Perm()&0o077 != 0) {
		t.Fatal("fixture input must be an owned non-symlink file with required permissions")
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok || stat.Uid != uint32(os.Getuid()) {
		t.Fatal("fixture input must be owned by the current user")
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || (resolved != root && !strings.HasPrefix(resolved, filepath.Clean(root)+string(os.PathSeparator))) {
		t.Fatal("fixture input must remain under fixture_root")
	}
}

func fixtureRowMatchesManifest(row fixtureOperatorRow, expected fixtureIdentityExpectedRow) bool {
	digest := sha256.Sum256([]byte(row.Metadata))
	return row.ID == expected.ID && row.TaskID == expected.TaskID && row.SessionID == expected.SessionID &&
		row.ExecutionID == expected.ExecutionID && row.ExecutorID == expected.ExecutorID && row.Runtime == expected.Runtime &&
		row.Status == expected.Status && row.UpdatedAt == expected.UpdatedAt && hex.EncodeToString(digest[:]) == expected.MetadataSHA256
}

func fixtureNomadStateMatchesManifest(state nomad.EnvironmentState, expected fixtureIdentityExpectedNomad, executionID string) bool {
	return state.Phase == expected.Phase && state.Revision == expected.Revision && state.EnvironmentID == expected.EnvironmentID &&
		state.Generation == expected.Generation && state.OperationID == expected.OperationID && state.OperationID == executionID &&
		state.JobID == expected.JobID && state.AllocationID == expected.AllocationID && state.AllocationCreateIndex == expected.AllocationCreateIndex &&
		state.NodeID == expected.NodeID && state.WorkspacePath == expected.WorkspacePath && state.ServerIdentity == expected.ServerIdentity
}

func verifyFixtureCurrentProcessIdentity(state nomad.EnvironmentState, expected fixtureIdentityExpectedProc) error {
	identity, found, err := fixtureProcessIdentity(expected.AgentctlPID)
	if err != nil || !found || identity.Start != expected.AgentctlStart || !strings.HasPrefix(identity.Command, expected.AgentctlRuntimePath) || !strings.Contains(identity.Command, state.JobID) || !strings.Contains(identity.Command, state.WorkspacePath) {
		return errors.New("fixture leader identity mismatch")
	}
	return nil
}

func assertFixturePIDAbsentExact(state nomad.EnvironmentState, expected fixtureIdentityExpectedProc) error {
	identity, found, err := fixtureProcessIdentity(expected.AgentctlPID)
	if err != nil {
		return err
	}
	if found && identity.Start == expected.AgentctlStart && strings.HasPrefix(identity.Command, expected.AgentctlRuntimePath) && strings.Contains(identity.Command, state.JobID) && strings.Contains(identity.Command, state.WorkspacePath) {
		return errors.New("exact original fixture leader remains alive")
	}
	return nil
}

func TestFixtureIdentityManifestMatchesCurrentSnapshot(t *testing.T) {
	manifestPath := strings.TrimSpace(os.Getenv("FF_FIXTURE_IDENTITY_MANIFEST"))
	if manifestPath == "" {
		t.Skip("fixture identity manifest is not configured")
	}
	manifest := readFixtureIdentityManifest(t, manifestPath)
	verifyFixturePrivateRoot(t, manifest.FixtureRoot)
	verifyFixtureOwnedFileUnderRoot(t, manifest.FixtureRoot, manifest.DatabasePath, false)
	verifyFixturePrivateFileUnderRoot(t, manifest.FixtureRoot, manifest.SessionConfigPath)
	verifyFixtureSessionConfigMatchesManifest(t, manifest)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	db, err := sqlx.Open("sqlite3", "file:"+manifest.DatabasePath+"?mode=ro&_busy_timeout=10000")
	if err != nil {
		t.Fatal("open captured fixture snapshot read-only")
	}
	defer func() { _ = db.Close() }()
	db.SetMaxOpenConns(1)
	row, err := readFixtureOperatorRow(ctx, db, manifest.ExpectedRow.TaskID)
	if err != nil || !fixtureRowMatchesManifest(row, manifest.ExpectedRow) {
		t.Fatal("current fixture row differs from captured exact revision")
	}
	_, state := fixtureMetadataAndLegacyState(t, row.Metadata)
	if !fixtureNomadStateMatchesManifest(state, manifest.ExpectedNomad, row.ExecutionID) {
		t.Fatal("current Nomad state differs from captured exact revision")
	}
	var sessionState string
	if err := db.GetContext(ctx, &sessionState, `SELECT state FROM task_sessions WHERE task_id = ? AND id = ?`, row.TaskID, row.SessionID); err != nil || sessionState != manifest.ExpectedSessionState {
		t.Fatal("current session state differs from captured identity")
	}
	t.Log("captured fixture identity, row metadata digest, and Nomad revision are current")
}

func TestFixtureTeardownSnapshotRejectsRowAndNomadRevisionDrift(t *testing.T) {
	metadata := `{"nomad_environment_state":{"phase":"ready"}}`
	digest := sha256.Sum256([]byte(metadata))
	expectedRow := fixtureIdentityExpectedRow{
		ID: "session", TaskID: "task", SessionID: "session", ExecutionID: "execution",
		ExecutorID: "executor", Runtime: "nomad", Status: "running", UpdatedAt: "row-revision-7",
		MetadataSHA256: hex.EncodeToString(digest[:]),
	}
	row := fixtureOperatorRow{
		ID: expectedRow.ID, TaskID: expectedRow.TaskID, SessionID: expectedRow.SessionID,
		ExecutionID: expectedRow.ExecutionID, ExecutorID: expectedRow.ExecutorID,
		Runtime: expectedRow.Runtime, Status: expectedRow.Status,
		UpdatedAt: expectedRow.UpdatedAt, Metadata: metadata,
	}
	if !fixtureRowMatchesManifest(row, expectedRow) {
		t.Fatal("unchanged captured row must match")
	}
	changed := row
	changed.UpdatedAt = "row-revision-8"
	if fixtureRowMatchesManifest(changed, expectedRow) {
		t.Fatal("changed row revision must fail closed")
	}
	changed = row
	changed.Metadata += " "
	if fixtureRowMatchesManifest(changed, expectedRow) {
		t.Fatal("changed metadata bytes must fail closed")
	}
	expectedNomad := fixtureIdentityExpectedNomad{
		Phase: "ready", Revision: 5, EnvironmentID: "environment", Generation: 1,
		OperationID: "execution", JobID: "job", AllocationID: "allocation",
		AllocationCreateIndex: 11, NodeID: "node", WorkspacePath: "/fixture/workspace",
		ServerIdentity: "server",
	}
	state := nomad.EnvironmentState{
		Phase: expectedNomad.Phase, Revision: expectedNomad.Revision,
		EnvironmentID: expectedNomad.EnvironmentID, Generation: expectedNomad.Generation,
		OperationID: expectedNomad.OperationID, JobID: expectedNomad.JobID,
		AllocationID: expectedNomad.AllocationID, AllocationCreateIndex: expectedNomad.AllocationCreateIndex,
		NodeID: expectedNomad.NodeID, WorkspacePath: expectedNomad.WorkspacePath,
		ServerIdentity: expectedNomad.ServerIdentity,
	}
	if !fixtureNomadStateMatchesManifest(state, expectedNomad, "execution") {
		t.Fatal("unchanged captured Nomad revision must match")
	}
	state.Revision++
	if fixtureNomadStateMatchesManifest(state, expectedNomad, "execution") {
		t.Fatal("changed Nomad state revision must fail closed")
	}
}

func waitCurrentFixtureJobTerminal(ctx context.Context, address, token string, client *nomad.Client, state nomad.EnvironmentState) ([]map[string]any, error) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	deadline := time.NewTimer(60 * time.Second)
	defer deadline.Stop()
	for {
		var job struct {
			ID        string `json:"ID"`
			Namespace string `json:"Namespace"`
			Status    string `json:"Status"`
			Stop      bool   `json:"Stop"`
		}
		jobErr := fixtureNomadGet(ctx, address, token, "/v1/job/"+url.PathEscape(state.JobID)+"?namespace="+url.QueryEscape(state.Namespace), &job)
		allocations, listErr := client.ListAllocations(ctx, state.Namespace, state.JobID)
		allTerminal, foundOriginal := len(allocations) > 0, false
		var evidence []map[string]any
		for _, summary := range allocations {
			full, err := client.GetAllocation(ctx, state.Namespace, summary.ID)
			if err != nil || full.ID != summary.ID || full.JobID != state.JobID || full.Namespace != state.Namespace || full.NodeID != state.NodeID || full.CreateIndex != summary.CreateIndex {
				allTerminal = false
				break
			}
			if full.ID == state.AllocationID && full.CreateIndex == state.AllocationCreateIndex {
				foundOriginal = true
			}
			if (full.ClientStatus != "complete" && full.ClientStatus != "failed") || full.TaskStates["agentctl"].State != "dead" {
				allTerminal = false
			}
			evidence = append(evidence, map[string]any{"id": full.ID, "create_index": full.CreateIndex, "client_status": full.ClientStatus, "desired_status": full.DesiredStatus, "agentctl_state": full.TaskStates["agentctl"].State, "node_id": full.NodeID})
		}
		if jobErr == nil && job.ID == state.JobID && job.Namespace == state.Namespace && job.Status == "dead" && job.Stop && listErr == nil && allTerminal && foundOriginal {
			return evidence, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("fixture job or allocations failed to become terminal")
		case <-ticker.C:
		}
	}
}
