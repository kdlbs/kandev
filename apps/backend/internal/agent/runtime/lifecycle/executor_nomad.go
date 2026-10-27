package lifecycle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

var ErrNomadExecutorNotReady = errors.New("nomad executor lifecycle is not ready")

const (
	nomadAllocationPollInterval = 500 * time.Millisecond
	nomadBootstrapTimeout       = 2 * time.Minute
)

type NomadExecutor struct {
	agentctlResolver *AgentctlResolver
	secretStore      interface {
		Reveal(context.Context, string) (string, error)
	}
	logger *logger.Logger

	// Narrow seams make the launch orchestration testable without allowing a
	// test or production caller to replace the fixed job policy.
	newNomadClient func(nomad.ProfileConfig, string) (*nomad.Client, error)
	dialSSH        func(context.Context, *SSHTarget) (*ssh.Client, error)
	mu             sync.Mutex
	sessions       map[string]*nomadAgentctlConnection
	closed         bool

	cleanup            nomadCleanupCoordinator
	cleanupActive      map[string]struct{}
	drainDeadline      time.Duration
	stopConfirmTimeout time.Duration

	// Pool-mode seams; empty values select the production locations.
	hostIdentityPath string
	kandevHomeDir    string
}

func (r *NomadExecutor) setCleanupCoordinator(coordinator nomadCleanupCoordinator) {
	r.mu.Lock()
	r.cleanup = coordinator
	r.mu.Unlock()
}

func (r *NomadExecutor) cleanupCoordinator() nomadCleanupCoordinator {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cleanup
}

func NewNomadExecutor(resolver *AgentctlResolver, secretStore interface {
	Reveal(context.Context, string) (string, error)
}, log *logger.Logger) *NomadExecutor {
	return &NomadExecutor{
		agentctlResolver: resolver,
		secretStore:      secretStore,
		logger:           log,
		newNomadClient:   nomad.NewClientForProfile,
		dialSSH:          DialSSH,
		sessions:         make(map[string]*nomadAgentctlConnection),
	}
}

func (r *NomadExecutor) Name() executor.Name { return executor.NameNomad }

func (r *NomadExecutor) HealthCheck(context.Context) error {
	if r == nil || r.agentctlResolver == nil || r.secretStore == nil || r.logger == nil {
		return ErrNomadExecutorNotReady
	}
	return nil
}

func (r *NomadExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (_ *ExecutorInstance, returnedErr error) {
	resumeExisting, err := r.validateNomadCreateRequest(ctx, req)
	if err != nil {
		return nil, err
	}
	if err := r.reserveNomadOperation(req.InstanceID); err != nil {
		return nil, err
	}
	defer func() {
		if returnedErr != nil {
			r.releaseNomadReservation(req.InstanceID)
		}
	}()
	id, err := resolveNomadLaunchIdentity(req)
	if err != nil {
		return nil, err
	}
	if resumeExisting {
		state, err := validateNomadContinuation(req, id)
		if err != nil {
			return nil, err
		}
		return r.attachRetainedOperation(ctx, req, id.profile, state)
	}
	if id.profile.Mode == nomad.ModePool {
		return r.launchPoolOperation(ctx, req, id)
	}
	return r.launchNewOperation(ctx, req, id)
}

// launchNewOperation records the launch intent, stages the immutable job
// evidence, registers the job, waits for its single allocation, and connects.
func (r *NomadExecutor) launchNewOperation(ctx context.Context, req *ExecutorCreateRequest, id nomadLaunchIdentity) (_ *ExecutorInstance, returnedErr error) {
	profile := id.profile
	state := newNomadIntentState(req, id)
	if err := r.admitNomadLaunch(ctx, req, profile); err != nil {
		return nil, err
	}
	nonce, err := recordNomadLaunchIntent(ctx, req, state)
	if err != nil {
		return nil, err
	}

	sshClient, err := r.dialPinnedNomadWorker(ctx, profile)
	if err != nil {
		return nil, err
	}
	closeSSH := true
	defer func() {
		if closeSSH {
			_ = sshClient.Close()
		}
	}()

	artifact, paths, err := r.checkpointNomadJobEvidence(ctx, req, profile, &state)
	if err != nil {
		return nil, err
	}
	apiTunnel, _, err := openSSHRuntimeAPITunnelForRequest(sshClient, req)
	if err != nil {
		return nil, fmt.Errorf("open Nomad worker Kandev API tunnel: %w", err)
	}
	if apiTunnel != nil {
		defer func() {
			if returnedErr != nil {
				_ = apiTunnel.Close()
			}
		}()
	}
	if err := stageNomadLaunchFiles(ctx, sshClient, req, profile, state, artifact, paths, nonce); err != nil {
		return nil, err
	}

	nomadClient, err := r.registerNomadJob(ctx, req, profile, &state)
	if err != nil {
		return nil, err
	}
	allocation, err := waitForNomadAllocation(ctx, nomadClient, profile.Namespace, state)
	if err != nil {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, &state, err)
	}
	state = allocation
	if err := checkpointNomadState(ctx, req, &state); err != nil {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, &state, fmt.Errorf("checkpoint running Nomad allocation: %w", err))
	}
	bootstrapCtx, cancelBootstrap := context.WithTimeout(ctx, nomadBootstrapTimeout)
	defer cancelBootstrap()
	connection, err := connectNomadAgentctlOverSSHWithTunnel(bootstrapCtx, sshClient, profile, &state, req, nonce, r.logger, apiTunnel)
	if err != nil {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, &state, err)
	}
	closeSSH = false
	if err := r.storeNomadConnection(req.InstanceID, connection, "nomad executor closed while connecting to the retained operation"); err != nil {
		return nil, err
	}
	return connection.instance, nil
}

// abortRegisteredNomadLaunch records the cleanup intent for a launch that
// failed after its job was registered, rolls it back, and returns cause
// unchanged.
func (r *NomadExecutor) abortRegisteredNomadLaunch(ctx context.Context, req *ExecutorCreateRequest, state *nomad.EnvironmentState, cause error) error {
	state.Phase = nomadPhaseCleanupPending
	state.Revision++
	_ = checkpointNomadState(context.Background(), req, state)
	r.rollbackNomadLaunch(ctx, req)
	return cause
}

func (r *NomadExecutor) admitNomadLaunch(ctx context.Context, req *ExecutorCreateRequest, profile nomad.ProfileConfig) error {
	client, err := r.newNomadAdmissionClient(ctx, profile)
	if err != nil {
		return err
	}
	return r.admitNewNomadOperation(ctx, req, profile, client)
}

func (r *NomadExecutor) newNomadAdmissionClient(ctx context.Context, profile nomad.ProfileConfig) (*nomad.Client, error) {
	token, err := r.revealNomadToken(ctx, profile)
	if err != nil {
		return nil, err
	}
	client, err := r.newNomadClient(profile, token)
	if err != nil {
		return nil, fmt.Errorf("configure Nomad API client: %w", err)
	}
	return client, nil
}

func (r *NomadExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, _ bool) error {
	if r == nil || instance == nil || instance.InstanceID == "" {
		return errors.New("nomad stop requires the exact connected instance identity")
	}
	// Only a direct archive, delete or rollback reason destroys the environment,
	// through the positive cleanup workflow. Every other stop, including a
	// forced one and the task-tree handoff that precedes the durable cleanup
	// job, only detaches this backend's transports and preserves the allocation
	// and worker fence for that job.
	if nomadStopDestroys(instance.StopReason) {
		return r.destroyOnStop(ctx, instance)
	}
	r.mu.Lock()
	connection, exists := r.sessions[instance.InstanceID]
	if !exists {
		r.mu.Unlock()
		return nil // duplicate close after a prior successful ordinary Stop
	}
	if connection == nil || !sameNomadConnectedInstance(connection.instance, instance) {
		r.mu.Unlock()
		return errors.New("nomad stop does not match the current connected operation")
	}
	delete(r.sessions, instance.InstanceID)
	r.mu.Unlock()
	connection.closeHostResources()
	return nil
}

// nomadStopDestroys reports whether a stop reason runs positive cleanup in the
// stopping call. Cascade and task-tree reasons belong to the durable cleanup
// job, which holds the environment claim.
func nomadStopDestroys(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case StopReasonCascadeArchive, StopReasonCascadeDelete, StopReasonTaskTreeArchived, StopReasonTaskTreeDeleted:
		return false
	}
	return shouldRunExecutorCleanup(reason)
}

// destroyOnStop runs positive cleanup for a destructive stop. A connected
// operation must match the requested instance; a detached one is cleaned from
// its retained inventory row.
func (r *NomadExecutor) destroyOnStop(ctx context.Context, instance *ExecutorInstance) error {
	coordinator := r.cleanupCoordinator()
	if coordinator == nil {
		return ErrNomadExecutorNotReady
	}
	r.mu.Lock()
	connection, exists := r.sessions[instance.InstanceID]
	mismatch := exists && connection != nil && !sameNomadConnectedInstance(connection.instance, instance)
	r.mu.Unlock()
	if mismatch {
		return errors.New("nomad stop does not match the current connected operation")
	}
	return coordinator.DestroyInstance(ctx, instance)
}

// rollbackNomadLaunch cleans up a launch whose job was registered but whose
// connection failed. The original launch error is never replaced.
func (r *NomadExecutor) rollbackNomadLaunch(ctx context.Context, req *ExecutorCreateRequest) {
	coordinator := r.cleanupCoordinator()
	if coordinator == nil {
		return
	}
	if err := coordinator.DestroyLaunch(ctx, req); err != nil {
		r.logger.Warn("nomad launch rollback retained cleanup", zap.String("execution_id", req.InstanceID))
	}
}

func sameNomadConnectedInstance(current, requested *ExecutorInstance) bool {
	if current == nil || requested == nil || current.InstanceID == "" || current.InstanceID != requested.InstanceID ||
		current.TaskID != requested.TaskID || current.SessionID != requested.SessionID ||
		current.RuntimeName != executor.NameNomad {
		return false
	}
	currentState, currentErr := nomad.EnvironmentStateFromMetadata(current.Metadata)
	requestedState, requestedErr := nomad.EnvironmentStateFromMetadata(requested.Metadata)
	return currentErr == nil && requestedErr == nil && sameNomadConnectedState(currentState, requestedState)
}

func sameNomadConnectedState(current, requested nomad.EnvironmentState) bool {
	return current.ExecutionID == requested.ExecutionID &&
		current.EnvironmentID == requested.EnvironmentID &&
		current.Generation == requested.Generation &&
		current.JobID == requested.JobID &&
		current.AllocationID == requested.AllocationID &&
		current.NodeID == requested.NodeID &&
		current.ServerIdentity == requested.ServerIdentity &&
		current.WorkspacePath == requested.WorkspacePath
}

// Close releases only this backend's client-side transports. Nomad owns the
// task/allocation and the worker fence remains authoritative across restarts.
func (r *NomadExecutor) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closed = true
	connections := r.sessions
	r.sessions = make(map[string]*nomadAgentctlConnection)
	r.mu.Unlock()
	for _, connection := range connections {
		if connection != nil {
			connection.closeHostResources()
		}
	}
	return nil
}

// attachRetainedOperation reconnects to one previously checkpointed operation.
// It never stages files, registers or updates a Nomad job, or creates a second
// allocation. The caller must have already validated the owner/session and
// passed the original immutable execution ID.
func (r *NomadExecutor) attachRetainedOperation(
	ctx context.Context,
	req *ExecutorCreateRequest,
	profile nomad.ProfileConfig,
	state nomad.EnvironmentState,
) (*ExecutorInstance, error) {
	client, job, err := r.openRetainedNomadJob(ctx, req, profile, state)
	if err != nil {
		return nil, err
	}
	lookupState, err := retainedNomadLookupState(ctx, req, state, job)
	if err != nil {
		return nil, err
	}
	allocated, err := waitForNomadAllocation(ctx, client, state.Namespace, lookupState)
	if err != nil {
		return nil, fmt.Errorf("reattach retained Nomad allocation: %w", err)
	}
	profile = profile.EffectiveProfile(allocated)
	if err := reconcileRetainedNomadAllocation(ctx, req, profile, state, &allocated); err != nil {
		return nil, err
	}
	connection, err := r.connectRetainedNomadWorker(ctx, req, client, profile, &allocated)
	if err != nil {
		return nil, err
	}
	if err := r.storeNomadConnection(req.InstanceID, connection, "nomad executor closed while reconnecting to the retained operation"); err != nil {
		return nil, err
	}
	return connection.instance, nil
}

// openRetainedNomadJob reads the retained job and proves it is the immutable
// operation before any reconnect side effect.
func (r *NomadExecutor) openRetainedNomadJob(
	ctx context.Context,
	req *ExecutorCreateRequest,
	profile nomad.ProfileConfig,
	state nomad.EnvironmentState,
) (*nomad.Client, *nomad.JobRecord, error) {
	token, err := r.revealNomadToken(ctx, profile)
	if err != nil {
		return nil, nil, err
	}
	client, err := r.newNomadClient(profile, token)
	if err != nil {
		return nil, nil, fmt.Errorf("configure Nomad API client for retained operation: %w", err)
	}
	job, err := client.GetJob(ctx, state.Namespace, state.JobID)
	if err != nil || !jobMatchesState(job, state) {
		return nil, nil, errors.New("retained Nomad job is missing or does not match the immutable operation")
	}
	if req.AuthToken == "" && req.BootstrapNonce == "" {
		return nil, nil, errors.New("retained Nomad operation has neither its original bootstrap nonce nor agentctl token reference")
	}
	return client, job, nil
}

// retainedNomadLookupState derives the state used to look the allocation up.
func retainedNomadLookupState(ctx context.Context, req *ExecutorCreateRequest, state nomad.EnvironmentState, job *nomad.JobRecord) (nomad.EnvironmentState, error) {
	lookupState := state
	// The durable intent is checkpointed before register, so a crash after
	// Nomad commits the job but before the registered checkpoint leaves these
	// observation indexes empty. Bind the indexes only after the full immutable
	// job response has passed jobMatchesState above.
	if lookupState.JobCreateIndex == 0 && lookupState.JobDefinitionIndex == 0 {
		if job.CreateIndex == 0 || job.JobModifyIndex == 0 {
			return lookupState, errors.New("retained Nomad job omitted immutable creation or definition index")
		}
		lookupState.JobCreateIndex = job.CreateIndex
		lookupState.JobDefinitionIndex = job.JobModifyIndex
	}
	if lookupState.Phase == nomadPhaseIntent {
		// The exact stable job was found above, so the persisted intent is now
		// known to have crossed the registration boundary. Reconcile its
		// allocation without re-registering or staging a second operation.
		lookupState.Phase = nomadPhaseRegistered
		lookupState.Revision++
		if err := checkpointNomadState(ctx, req, &lookupState); err != nil {
			return lookupState, fmt.Errorf("checkpoint reconciled Nomad registration: %w", err)
		}
	}
	if state.AllocationID != "" {
		// BindSingleRunningAllocation intentionally accepts only a new
		// registered record. Reconcile an existing allocation by looking up its
		// exact ID, then compare the full identity and immutable ports below.
		lookupState.Phase = nomadPhaseRegistered
		lookupState.AllocationID = ""
		lookupState.AllocationCreateIndex = 0
		lookupState.ControlPort = 0
		lookupState.InstancePort = 0
		lookupState.ServerIdentity = ""
	}
	return lookupState, nil
}

// reconcileRetainedNomadAllocation proves the observed allocation is the
// retained one and checkpoints any identity change.
func reconcileRetainedNomadAllocation(
	ctx context.Context,
	req *ExecutorCreateRequest,
	profile nomad.ProfileConfig,
	prior nomad.EnvironmentState,
	allocated *nomad.EnvironmentState,
) error {
	if allocated.AllocationID == "" || allocated.NodeID != profile.NodeID || allocated.JobID != prior.JobID {
		return errors.New("retained Nomad allocation did not prove the immutable worker identity")
	}
	if prior.AllocationID != "" && (allocated.AllocationID != prior.AllocationID || allocated.AllocationCreateIndex != prior.AllocationCreateIndex) {
		return errors.New("retained Nomad allocation differs from the checkpointed allocation identity")
	}
	if prior.ServerIdentity != "" {
		allocated.ServerIdentity = prior.ServerIdentity
	}
	if prior.Phase == nomadPhaseReady {
		allocated.Phase = nomadPhaseReady
	}
	if allocated.AllocationID != prior.AllocationID || allocated.Phase != prior.Phase || allocated.ControlPort != prior.ControlPort || allocated.InstancePort != prior.InstancePort {
		allocated.Revision = prior.Revision + 1
		if err := checkpointNomadState(ctx, req, allocated); err != nil {
			return fmt.Errorf("checkpoint retained Nomad allocation identity: %w", err)
		}
	}
	return nil
}

// connectRetainedNomadWorker dials the pinned worker, verifies its fence, and
// reconnects agentctl.
func (r *NomadExecutor) connectRetainedNomadWorker(
	ctx context.Context,
	req *ExecutorCreateRequest,
	client *nomad.Client,
	profile nomad.ProfileConfig,
	allocated *nomad.EnvironmentState,
) (*nomadAgentctlConnection, error) {
	if allocated.PlacementMode == nomad.ModePool {
		return r.connectRetainedPoolWorker(ctx, req, client, profile, allocated)
	}
	target, err := ResolveSSHTarget(SSHConnConfig{
		HostAlias: profile.SSHHost, Host: profile.SSHHost, Port: profile.SSHPort, User: profile.SSHUser,
		PinnedFingerprint: profile.SSHHostFingerprint,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve pinned retained Nomad worker identity: %w", err)
	}
	sshClient, err := r.dialSSH(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("connect to pinned retained Nomad worker: %w", err)
	}
	if err := verifyNomadWorkerFence(ctx, sshClient, profile, *allocated); err != nil {
		_ = sshClient.Close()
		return nil, err
	}
	tunnel, _, err := openSSHRuntimeAPITunnelForRequest(sshClient, req)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("open retained Nomad worker Kandev API tunnel: %w", err)
	}
	return connectNomadAgentctlOverSSHWithTunnel(ctx, sshClient, profile, allocated, req, req.BootstrapNonce, r.logger, tunnel)
}
func (r *NomadExecutor) RecoverInstances(ctx context.Context, records []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	if err := r.HealthCheck(ctx); err != nil {
		return nil, err
	}
	instances := make([]*ExecutorInstance, 0)
	for _, record := range records {
		if record == nil || record.Runtime != executor.NameNomad || isTerminalExecutorRunningStatus(record.Status) {
			continue
		}
		if handled := r.resumeNomadCleanupRecord(ctx, record); handled {
			continue
		}
		instance, err := r.recoverReadyNomadRecord(ctx, record)
		if err != nil {
			r.logger.Warn("Nomad runtime recovery deferred; retained allocation and inventory were not changed",
				zap.String("session_id", record.SessionID), zap.Error(err))
			continue
		}
		instances = append(instances, instance)
	}
	return instances, nil
}

// resumeNomadCleanupRecord repeats the idempotent tail of a retained cleanup. A
// failure leaves the retained inventory and quarantine untouched for the next pass.
func (r *NomadExecutor) resumeNomadCleanupRecord(ctx context.Context, record *models.ExecutorRunning) bool {
	coordinator := r.cleanupCoordinator()
	if coordinator == nil {
		return false
	}
	handled, err := coordinator.ResumeCleanup(ctx, record)
	if err != nil {
		r.logger.Warn("Nomad cleanup resume deferred; retained inventory and quarantine were not changed",
			zap.String("session_id", record.SessionID))
	}
	return handled
}

func (r *NomadExecutor) recoverReadyNomadRecord(ctx context.Context, record *models.ExecutorRunning) (*ExecutorInstance, error) {
	state, profile, err := nomadRecoveryIdentity(record)
	if err != nil {
		return nil, err
	}
	req := nomadRecoveryRequest(record, state)
	if req.Metadata[MetadataKeyExecutorProfileID] == nil {
		return nil, errors.New("nomad recovery profile reference is missing from the retained row")
	}
	instance, err := r.attachRetainedOperation(ctx, req, profile, state)
	if err != nil {
		return nil, fmt.Errorf("reattach exact ready Nomad operation: %w", err)
	}
	if !nomadRecoveredInstanceMatches(instance, record, state) {
		if instance != nil {
			r.discardRecoveredNomadConnection(record.AgentExecutionID)
		}
		return nil, errors.New("nomad recovered connection does not match the retained execution identity")
	}
	return instance, nil
}

// nomadRecoveryIdentity validates the live row and returns its ready state and profile.
func nomadRecoveryIdentity(record *models.ExecutorRunning) (nomad.EnvironmentState, nomad.ProfileConfig, error) {
	var state nomad.EnvironmentState
	var profile nomad.ProfileConfig
	if record == nil || record.SessionID == "" || record.TaskID == "" || record.AgentExecutionID == "" ||
		record.TransientAuthToken == "" || record.Metadata == nil {
		return state, profile, errors.New("nomad recovery requires the exact live row and saved agentctl credential")
	}
	state, err := nomad.EnvironmentStateFromMetadata(record.Metadata)
	if err != nil || !nomadRecoveryStateComplete(state, record) {
		return state, profile, errors.New("nomad recovery requires a complete ready allocation and workspace identity")
	}
	profile, err = nomadEffectiveProfile(record.Metadata, state)
	if err != nil || profile.NodeID != state.NodeID || profile.WorkspaceRoot != state.WorkspaceRoot {
		return state, profile, errors.New("nomad recovery profile differs from the retained worker or workspace identity")
	}
	return state, profile, nil
}

func nomadRecoveryStateComplete(state nomad.EnvironmentState, record *models.ExecutorRunning) bool {
	return state.Phase == nomadPhaseReady && state.ExecutionID == record.AgentExecutionID &&
		state.OperationID == record.AgentExecutionID && state.EnvironmentID != "" && state.Generation >= 1 &&
		state.AllocationID != "" && state.ServerIdentity != "" && state.WorkspacePath != ""
}

func nomadRecoveryRequest(record *models.ExecutorRunning, state nomad.EnvironmentState) *ExecutorCreateRequest {
	return &ExecutorCreateRequest{
		InstanceID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID,
		TaskEnvironmentID: state.EnvironmentID, WorkspacePath: state.WorkspacePath,
		OriginalWorkspacePath: getMetadataString(record.Metadata, MetadataKeyOriginalWorkspacePath),
		AgentProfileID:        record.ExecutionProfileID, PreviousExecutionID: record.AgentExecutionID,
		Metadata: clonePluginExecutorMetadata(record.Metadata), AuthToken: record.TransientAuthToken,
		BootstrapNonce: record.TransientBootstrapNonce, RequireExistingAgentctlInstance: true,
	}
}

func nomadRecoveredInstanceMatches(instance *ExecutorInstance, record *models.ExecutorRunning, state nomad.EnvironmentState) bool {
	return instance != nil && instance.InstanceID == record.AgentExecutionID && instance.TaskID == record.TaskID &&
		instance.SessionID == record.SessionID && instance.RuntimeName == executor.NameNomad &&
		instance.WorkspacePath == state.WorkspacePath && instance.AuthToken == record.TransientAuthToken
}

func (r *NomadExecutor) discardRecoveredNomadConnection(instanceID string) {
	r.mu.Lock()
	if connection := r.sessions[instanceID]; connection != nil {
		connection.closeHostResources()
	}
	delete(r.sessions, instanceID)
	r.mu.Unlock()
}

func (*NomadExecutor) GetInteractiveRunner() *process.InteractiveRunner { return nil }
func (*NomadExecutor) RequiresCloneURL() bool                           { return true }
func (*NomadExecutor) ShouldApplyPreferredShell() bool                  { return false }
func (*NomadExecutor) IsAlwaysResumable() bool                          { return true }

func nomadProfileFromMetadata(metadata map[string]interface{}) (nomad.ProfileConfig, error) {
	config := make(map[string]string, 16)
	for _, key := range []string{
		nomadMetadataKeyAddress, "namespace", nomadMetadataKeyRegion, nomadMetadataKeyDatacenter, nomadMetadataKeyNodeID, nomadMetadataKeyPlatform, nomadMetadataKeySSHTarget, "ssh_port",
		"ssh_host_fingerprint", nomadMetadataKeyWorkspaceRoot, nomadMetadataKeyAllocationRoot, nomadMetadataKeyCPUMHz, nomadMetadataKeyMemoryMB, nomadMetadataKeyToken, nomadMetadataKeyLostAfter,
	} {
		value := getMetadataString(metadata, key)
		if value != "" {
			config[key] = value
		}
	}
	for _, key := range nomadPoolMetadataKeys {
		value := getMetadataString(metadata, key)
		if value != "" {
			config[key] = value
		}
	}
	return nomad.ParseProfileConfig(config)
}

func nomadLaunchDigest(profile nomad.ProfileConfig, req *ExecutorCreateRequest, workspaceInputPath, environmentWorkspacePath string) (string, error) {
	if req == nil || environmentWorkspacePath == "" {
		return "", errors.New("nomad launch digest requires request and immutable environment workspace")
	}
	canonical := struct {
		Profile        nomad.ProfileConfig `json:"profile"`
		Execution      string              `json:"execution"`
		Task           string              `json:"task"`
		Session        string              `json:"session"`
		Workspace      string              `json:"workspace"`
		WorkspaceInput string              `json:"workspace_input"`
		WorkspacePath  string              `json:"workspace_path"`
		AgentProfileID string              `json:"agent_profile_id"`
		Protocol       string              `json:"protocol"`
		RepositoryIDs  []string            `json:"repository_ids"`
	}{profile, req.InstanceID, req.TaskID, req.SessionID, req.WorkspaceID, workspaceInputPath, environmentWorkspacePath, req.AgentProfileID, req.Protocol, req.RepositoryIDs}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}

func checkpointNomadState(ctx context.Context, req *ExecutorCreateRequest, state *nomad.EnvironmentState) error {
	if req == nil || state == nil || req.CheckpointRuntimeInventory == nil {
		return errors.New("nomad state checkpoint callback is unavailable")
	}
	if err := nomad.PutEnvironmentState(req.Metadata, *state); err != nil {
		return err
	}
	return req.CheckpointRuntimeInventory(ctx, req.Metadata)
}

func jobMatchesState(job *nomad.JobRecord, state nomad.EnvironmentState) bool {
	if job == nil || job.ModifyIndex == 0 || job.ID != state.JobID || job.Namespace != state.Namespace || job.Region != state.Region ||
		!jobMetaMatchesState(job, state) || !jobIndexesMatchState(job, state) || !nomadContinuationEvidencePresent(state) {
		return false
	}
	expected, err := nomad.NativeJobFromState(state)
	if err != nil {
		return false
	}
	raw := job.Raw
	if len(raw) == 0 {
		raw, err = json.Marshal(job.Spec)
		if err != nil {
			return false
		}
	}
	return nomad.ValidateNativeJobResponse(raw, expected) == nil
}

func jobMetaMatchesState(job *nomad.JobRecord, state nomad.EnvironmentState) bool {
	return job.Meta["kandev.environment_id"] == state.EnvironmentID &&
		job.Meta["kandev.generation"] == strconv.FormatInt(state.Generation, 10) &&
		job.Meta["kandev.operation_id"] == state.OperationID &&
		job.Meta["kandev.installation_id"] == state.InstallationID &&
		job.Meta["kandev.input_digest"] == state.InputDigest && nomadJobNodeMetaMatches(job, state) &&
		job.Meta["kandev.native_job_policy_version"] == strconv.Itoa(state.NativeJobPolicyVersion) &&
		job.Meta["kandev.native_job_spec_sha256"] == state.NativeJobSpecSHA256
}

func jobIndexesMatchState(job *nomad.JobRecord, state nomad.EnvironmentState) bool {
	return (state.JobCreateIndex == 0 || job.CreateIndex == state.JobCreateIndex) &&
		(state.JobDefinitionIndex == 0 || job.JobModifyIndex == state.JobDefinitionIndex)
}

func waitForNomadAllocation(ctx context.Context, client *nomad.Client, namespace string, state nomad.EnvironmentState) (nomad.EnvironmentState, error) {
	ticker := time.NewTicker(nomadAllocationPollInterval)
	defer ticker.Stop()
	for {
		allocations, err := client.ListAllocations(ctx, namespace, state.JobID)
		if err != nil {
			return state, fmt.Errorf("read Nomad allocation state: %w", err)
		}
		if len(allocations) == 1 && allocations[0].ClientStatus == nomadAllocationClientStatusRunning {
			return bindRunningNomadAllocation(ctx, client, namespace, state, allocations[0])
		}
		if len(allocations) > 1 {
			return state, errors.New("nomad returned multiple allocations for a single immutable executor job")
		}
		select {
		case <-ctx.Done():
			return state, fmt.Errorf("wait for exact running Nomad allocation: %w", ctx.Err())
		case <-ticker.C:
		}
	}
}

// bindRunningNomadAllocation proves the running allocation summary and full
// record are the immutable job's allocation, then binds it to the state.
func bindRunningNomadAllocation(
	ctx context.Context,
	client *nomad.Client,
	namespace string,
	state nomad.EnvironmentState,
	stub nomad.AllocationRecord,
) (nomad.EnvironmentState, error) {
	if state.PlacementMode == nomad.ModePool && state.NodeID == "" {
		return bindUnplacedPoolAllocation(ctx, client, namespace, state, stub)
	}
	if stub.ID == "" || stub.CreateIndex == 0 || stub.JobID != state.JobID || stub.NodeID != state.NodeID || stub.Namespace != namespace {
		return state, errors.New("nomad allocation summary identity does not match the immutable job")
	}
	allocation, err := client.GetAllocation(ctx, namespace, stub.ID)
	if err != nil {
		return state, fmt.Errorf("read full Nomad allocation: %w", err)
	}
	if allocation.ID != stub.ID || allocation.CreateIndex != stub.CreateIndex || allocation.JobID != stub.JobID || allocation.NodeID != stub.NodeID || allocation.Namespace != stub.Namespace {
		return state, errors.New("full Nomad allocation identity differs from its immutable job summary")
	}
	if err := validateAllocationPolicy(ctx, client, state, allocation); err != nil {
		return state, err
	}
	return nomad.BindSingleRunningAllocation(state, []nomad.AllocationRecord{*allocation})
}

func validateAllocationPolicy(ctx context.Context, client *nomad.Client, state nomad.EnvironmentState, allocation *nomad.AllocationRecord) error {
	if allocation == nil || len(allocation.Job) == 0 {
		return errors.New("full Nomad allocation omitted its embedded job policy")
	}
	expected, err := nomad.NativeJobFromState(state)
	if err != nil {
		return err
	}
	current, err := client.GetJob(ctx, state.Namespace, state.JobID)
	if err != nil || !jobMatchesState(current, state) {
		return errors.New("current Nomad job definition no longer matches the immutable allocation owner")
	}
	if err := nomad.ValidateNativeJobResponse(allocation.Job, expected); err != nil {
		return fmt.Errorf("nomad allocation embedded job policy is not the exact immutable definition: %w", err)
	}
	var allocatedJob nomad.Job
	if err := json.Unmarshal(allocation.Job, &allocatedJob); err != nil {
		return fmt.Errorf("decode allocation job version: %w", err)
	}
	if allocatedJob.JobModifyIndex != state.JobDefinitionIndex || allocatedJob.Version != current.Spec.Version {
		return fmt.Errorf("nomad allocation references a different job definition version (allocation job modify index %d, expected %d; allocation version %d, current version %d)", allocatedJob.JobModifyIndex, state.JobDefinitionIndex, allocatedJob.Version, current.Spec.Version)
	}
	return nil
}
