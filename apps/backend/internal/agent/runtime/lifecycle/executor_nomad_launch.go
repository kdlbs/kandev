package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/nomad"
)

// nomadLaunchIdentity is the authoritative, immutable identity derived from the
// profile snapshot and request before any external side effect.
type nomadLaunchIdentity struct {
	profile       nomad.ProfileConfig
	profileID     string
	generation    int64
	workspacePath string
	inputDigest   string
	jobID         string
}

// validateNomadCreateRequest checks the request shape and reports whether it
// continues a retained operation.
func (r *NomadExecutor) validateNomadCreateRequest(ctx context.Context, req *ExecutorCreateRequest) (bool, error) {
	if err := r.HealthCheck(ctx); err != nil {
		return false, err
	}
	if req == nil || req.InstanceID == "" || req.TaskID == "" || req.SessionID == "" || req.TaskEnvironmentID == "" || req.Metadata == nil {
		return false, errors.New("nomad launch requires an execution, task, session, environment, and authoritative metadata")
	}
	if req.CheckpointRuntimeInventory == nil {
		return false, errors.New("nomad launch requires durable inventory checkpointing")
	}
	resumeExisting := req.PreviousExecutionID != ""
	if resumeExisting && req.PreviousExecutionID != req.InstanceID {
		return false, errors.New("nomad continuation must use the retained operation identity")
	}
	return resumeExisting, nil
}

// reserveNomadOperation reserves the operation before any external side effect.
func (r *NomadExecutor) reserveNomadOperation(instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return errors.New("nomad executor is closed")
	}
	if r.sessions == nil {
		r.sessions = make(map[string]*nomadAgentctlConnection)
	}
	if _, alreadyConnected := r.sessions[instanceID]; alreadyConnected {
		return errors.New("nomad execution is already connected; duplicate create is not admitted")
	}
	r.sessions[instanceID] = nil
	return nil
}

// releaseNomadReservation drops a reservation that never became a connection.
func (r *NomadExecutor) releaseNomadReservation(instanceID string) {
	r.mu.Lock()
	if r.sessions[instanceID] == nil {
		delete(r.sessions, instanceID)
	}
	r.mu.Unlock()
}

// storeNomadConnection publishes a connected operation unless the executor closed.
func (r *NomadExecutor) storeNomadConnection(instanceID string, connection *nomadAgentctlConnection, closedMessage string) error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		connection.closeHostResources()
		return errors.New(closedMessage)
	}
	r.sessions[instanceID] = connection
	r.mu.Unlock()
	return nil
}

func resolveNomadLaunchIdentity(req *ExecutorCreateRequest) (nomadLaunchIdentity, error) {
	var id nomadLaunchIdentity
	profile, err := nomadProfileFromMetadata(req.Metadata)
	if err != nil {
		return id, err
	}
	profileID := getMetadataString(req.Metadata, MetadataKeyExecutorProfileID)
	if profileID == "" {
		return id, errors.New("nomad launch requires authoritative executor profile identity")
	}
	generation, err := strconv.ParseInt(getMetadataString(req.Metadata, "nomad_environment_generation"), 10, 64)
	if err != nil || generation < 1 {
		return id, errors.New("nomad launch requires a positive task environment ownership generation")
	}
	workspacePath, err := nomadIdentityWorkspacePath(profile, req.TaskEnvironmentID, generation)
	if err != nil {
		return id, fmt.Errorf("derive immutable Nomad environment workspace: %w", err)
	}
	inputDigest, err := nomadLaunchDigest(profile, req, req.WorkspacePath, nomadDigestWorkspacePath(profile, req.TaskEnvironmentID, generation, workspacePath))
	if err != nil {
		return id, fmt.Errorf("nomad launch identity: %w", err)
	}
	jobID, err := nomad.StableJobID(nomad.JobIdentity{
		EnvironmentID: req.TaskEnvironmentID, Generation: generation,
		OperationID: req.InstanceID, InstallationID: profileID,
	})
	if err != nil {
		return id, err
	}
	return nomadLaunchIdentity{
		profile: profile, profileID: profileID, generation: generation,
		workspacePath: workspacePath, inputDigest: inputDigest, jobID: jobID,
	}, nil
}

// nomadIdentityWorkspacePath is the environment workspace of a legacy profile.
// A pool environment has none until a node is chosen, so it returns empty.
func nomadIdentityWorkspacePath(profile nomad.ProfileConfig, environmentID string, generation int64) (string, error) {
	if profile.Mode == nomad.ModePool {
		return "", nil
	}
	return nomad.EnvironmentWorkspacePath(profile.WorkspaceRoot, environmentID, generation)
}

// nomadDigestWorkspacePath is the workspace identity folded into the launch
// digest. A pool environment uses a symbolic path because the real one depends
// on the node Nomad will choose.
func nomadDigestWorkspacePath(profile nomad.ProfileConfig, environmentID string, generation int64, workspacePath string) string {
	if profile.Mode == nomad.ModePool {
		return fmt.Sprintf("pool/%s/g%d", environmentID, generation)
	}
	return workspacePath
}

// validateNomadContinuation proves the retained typed inventory is the exact
// operation, owner, input and phase the request continues.
func validateNomadContinuation(req *ExecutorCreateRequest, id nomadLaunchIdentity) (nomad.EnvironmentState, error) {
	state, err := nomad.EnvironmentStateFromMetadata(req.Metadata)
	if err != nil {
		return state, fmt.Errorf("nomad continuation requires retained typed inventory: %w", err)
	}
	if !nomadContinuationIdentityMatches(state, req, id) || !nomadContinuationEvidencePresent(state) ||
		!nomadContinuationPhaseAllowed(state, req) {
		return state, errors.New("nomad continuation does not match the exact retained operation, owner, input, or phase")
	}
	if _, err := nomad.NativeJobFromState(state); err != nil {
		return state, fmt.Errorf("nomad continuation immutable job evidence is invalid: %w", err)
	}
	inputDigest, err := nomadLaunchDigest(id.profile, req, state.WorkspaceInputPath,
		nomadDigestWorkspacePath(id.profile, req.TaskEnvironmentID, id.generation, id.workspacePath))
	if err != nil || state.InputDigest != inputDigest {
		return state, errors.New("nomad continuation does not match the exact retained operation, owner, input, or phase")
	}
	return state, nil
}

func nomadContinuationIdentityMatches(state nomad.EnvironmentState, req *ExecutorCreateRequest, id nomadLaunchIdentity) bool {
	common := state.WorkspaceInputCaptured && state.ExecutionID == req.InstanceID && state.OperationID == req.InstanceID &&
		state.EnvironmentID == req.TaskEnvironmentID && state.InputDigest != "" && state.InstallationID == id.profileID
	if id.profile.Mode == nomad.ModePool {
		return common && state.PlacementMode == nomad.ModePool && nomadPoolSnapshotMatches(state, id.profile)
	}
	return common && state.NodeID == id.profile.NodeID &&
		state.WorkspaceRoot == id.profile.WorkspaceRoot && state.WorkspacePath == id.workspacePath
}

func nomadContinuationEvidencePresent(state nomad.EnvironmentState) bool {
	if state.PlacementMode == nomad.ModePool && state.NodeID == "" {
		return state.ArtifactSHA256 != "" && state.NativeJobSpecSHA256 != ""
	}
	return state.AgentctlPath != "" && state.BootstrapPath != "" && state.NativeJobSpecSHA256 != ""
}

func nomadContinuationPhaseAllowed(state nomad.EnvironmentState, req *ExecutorCreateRequest) bool {
	switch state.Phase {
	case nomadPhaseIntent, nomadPhaseRegistered, nomadPhaseAllocated:
		return true
	case nomadPhaseReady:
		return req.RequireExistingAgentctlInstance
	default:
		return false
	}
}

func newNomadIntentState(req *ExecutorCreateRequest, id nomadLaunchIdentity) nomad.EnvironmentState {
	profile := id.profile
	if profile.Mode == nomad.ModePool {
		return newNomadPoolIntentState(req, id)
	}
	return nomad.EnvironmentState{
		Version: 1, Revision: 1, EnvironmentID: req.TaskEnvironmentID, Generation: id.generation,
		OperationID: req.InstanceID, InstallationID: id.profileID, ExecutionID: req.InstanceID,
		InputDigest: id.inputDigest, JobID: id.jobID, Namespace: profile.Namespace, Region: profile.Region,
		Datacenter: profile.Datacenter, NodeID: profile.NodeID, Platform: profile.Platform,
		SSHHost: profile.SSHHost, SSHUser: profile.SSHUser, SSHPort: profile.SSHPort,
		SSHHostFingerprint: profile.SSHHostFingerprint, WorkspaceRoot: profile.WorkspaceRoot, WorkspacePath: id.workspacePath,
		WorkspaceInputPath: req.WorkspacePath, WorkspaceInputCaptured: true,
		AllocationRoot: profile.AllocationRoot, CPUMHz: profile.CPUMHz, MemoryMB: profile.MemoryMB,
		DisconnectLostAfterSeconds: int64(profile.DisconnectLostAfter.Seconds()),
		Phase:                      nomadPhaseIntent,
	}
}

// recordNomadLaunchIntent durably records the bootstrap nonce and launch intent
// before any dispatch and returns the nonce.
func recordNomadLaunchIntent(ctx context.Context, req *ExecutorCreateRequest, state nomad.EnvironmentState) (string, error) {
	nonce := req.BootstrapNonce
	if nonce == "" {
		var err error
		nonce, err = generateBootstrapNonce()
		if err != nil {
			return "", fmt.Errorf("generate Nomad bootstrap nonce: %w", err)
		}
	}
	if req.PersistNomadRuntimeSecret == nil {
		return "", errors.New("nomad launch requires durable bootstrap secret persistence")
	}
	if err := req.PersistNomadRuntimeSecret(ctx, MetadataKeyBootstrapNonceSecret, nonce); err != nil {
		return "", fmt.Errorf("persist Nomad bootstrap nonce before dispatch: %w", err)
	}
	req.BootstrapNonce = nonce
	if err := nomad.PutEnvironmentState(req.Metadata, state); err != nil {
		return "", err
	}
	if err := req.CheckpointRuntimeInventory(ctx, req.Metadata); err != nil {
		return "", fmt.Errorf("checkpoint Nomad launch intent: %w", err)
	}
	return nonce, nil
}

func (r *NomadExecutor) dialPinnedNomadWorker(ctx context.Context, profile nomad.ProfileConfig) (*ssh.Client, error) {
	target, err := ResolveSSHTarget(SSHConnConfig{
		HostAlias: profile.SSHHost, Host: profile.SSHHost, Port: profile.SSHPort, User: profile.SSHUser,
		PinnedFingerprint: profile.SSHHostFingerprint,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve pinned Nomad worker SSH identity: %w", err)
	}
	sshClient, err := r.dialSSH(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("connect to pinned Nomad worker: %w", err)
	}
	return sshClient, nil
}

// resolveNomadWorkerArtifact returns the verified Darwin arm64 agentctl and its
// SHA-256 digest.
func (r *NomadExecutor) resolveNomadWorkerArtifact(ctx context.Context, req *ExecutorCreateRequest) (string, []byte, error) {
	digest, artifact, _, err := localAgentctlSha256WithContext(ctx, r.agentctlResolver, SSHRemotePlatform{GOOS: "darwin", GOARCH: sshRemoteGOARCHARM64}, req.OnProgress)
	if err != nil {
		return "", nil, fmt.Errorf("resolve verified Darwin arm64 agentctl: %w", err)
	}
	return digest, artifact, nil
}

// checkpointNomadJobEvidence resolves the verified agentctl artifact, pins the
// immutable job digest and worker paths, and checkpoints them before staging.
func (r *NomadExecutor) checkpointNomadJobEvidence(
	ctx context.Context,
	req *ExecutorCreateRequest,
	profile nomad.ProfileConfig,
	state *nomad.EnvironmentState,
) ([]byte, nomadWorkerPaths, error) {
	artifactDigest, artifact, err := r.resolveNomadWorkerArtifact(ctx, req)
	if err != nil {
		return nil, nomadWorkerPaths{}, err
	}
	paths, err := nomadWorkerPathsFor(profile, *state, artifactDigest)
	if err != nil {
		return nil, nomadWorkerPaths{}, err
	}
	job, err := nomad.BuildNativeJob(nomad.NativeJobInput{Profile: profile, State: *state, AgentctlPath: paths.ArtifactPath, BootstrapPath: paths.BootstrapPath})
	if err != nil {
		return nil, nomadWorkerPaths{}, err
	}
	state.NativeJobSpecSHA256, err = nomad.NativeJobSpecDigest(job)
	if err != nil {
		return nil, nomadWorkerPaths{}, err
	}
	state.AgentctlPath = paths.ArtifactPath
	state.BootstrapPath = paths.BootstrapPath
	state.NativeJobPolicyVersion = nomad.NativeJobPolicyVersion
	state.Revision++
	if err := checkpointNomadState(ctx, req, state); err != nil {
		return nil, nomadWorkerPaths{}, fmt.Errorf("checkpoint immutable Nomad job evidence before staging: %w", err)
	}
	return artifact, paths, nil
}

// stageNomadLaunchFiles stages the worker files and proves they landed at the
// pre-dispatch paths.
func stageNomadLaunchFiles(
	ctx context.Context,
	sshClient *ssh.Client,
	req *ExecutorCreateRequest,
	profile nomad.ProfileConfig,
	state nomad.EnvironmentState,
	artifact []byte,
	paths nomadWorkerPaths,
	nonce string,
) error {
	staged, err := stageNomadLaunchFilesOver(ctx, nomadSSHTransport{client: sshClient}, req, profile, state, artifact, nonce)
	if err != nil {
		return err
	}
	if staged != paths {
		return errors.New("staged Nomad worker paths differ from the pre-dispatch immutable job evidence")
	}
	return nil
}

func stageNomadLaunchFilesOver(
	ctx context.Context,
	transport nomadWorkerTransport,
	req *ExecutorCreateRequest,
	profile nomad.ProfileConfig,
	state nomad.EnvironmentState,
	artifact []byte,
	nonce string,
) (nomadWorkerPaths, error) {
	bootstrapEnv := managedGitCredentialBrokerEnv(sshRemoteContributionEnv(req, ""))
	if selectedCheckoutIsPullRequest(req.Metadata) {
		bootstrapEnv = nil
	}
	bootstrapEnv = sshAgentctlLaunchEnv(bootstrapEnv, nonce, req.AgentctlStartupConfig)
	return stageNomadWorkerFilesOver(ctx, transport, profile, state, artifact, bootstrapEnv)
}

// revealNomadToken resolves the Nomad API credential from its secret reference.
func (r *NomadExecutor) revealNomadToken(ctx context.Context, profile nomad.ProfileConfig) (string, error) {
	token, err := r.secretStore.Reveal(ctx, profile.NomadTokenSecretID)
	if err != nil || token == "" {
		return "", errors.New("resolve Nomad API credential from the configured secret reference")
	}
	return token, nil
}

func isNomadRegistrationReconcilable(err error) bool {
	return errors.Is(err, nomad.ErrRegistrationOutcomeUnknown) || errors.Is(err, nomad.ErrJobRegistrationConflict)
}

// registerNomadJob creates the immutable job (create-only), reconciles it to the
// exact full job read, and checkpoints the registered phase.
func (r *NomadExecutor) registerNomadJob(
	ctx context.Context,
	req *ExecutorCreateRequest,
	profile nomad.ProfileConfig,
	state *nomad.EnvironmentState,
) (*nomad.Client, error) {
	token, err := r.revealNomadToken(ctx, profile)
	if err != nil {
		return nil, err
	}
	nomadClient, err := r.newNomadClient(profile, token)
	if err != nil {
		return nil, fmt.Errorf("configure Nomad API client: %w", err)
	}
	job, err := buildNomadJobForState(profile, *state)
	if err != nil {
		return nil, err
	}
	registration, err := nomadClient.RegisterCreateOnly(ctx, job)
	if err != nil && !isNomadRegistrationReconcilable(err) {
		return nil, fmt.Errorf("register immutable Nomad job: %w", err)
	}
	// The PUT was sent: from here on a failure may leave a registered job, so
	// every exit records the cleanup intent and stops it.
	registered, getErr := nomadClient.GetJob(ctx, profile.Namespace, state.JobID)
	if getErr != nil || !jobMatchesState(registered, *state) {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, state, fmt.Errorf("nomad job registration did not reconcile to the exact immutable job %q", state.JobID))
	}
	if !isNomadRegistrationReconcilable(err) && registration.JobModifyIndex != registered.JobModifyIndex {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, state, errors.New("nomad registered definition index differs from the full job read"))
	}
	if registered.CreateIndex == 0 || registered.JobModifyIndex == 0 {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, state, errors.New("nomad job read omitted immutable creation or definition index"))
	}
	state.Phase = nomadPhaseRegistered
	state.JobCreateIndex = registered.CreateIndex
	state.JobDefinitionIndex = registered.JobModifyIndex
	state.Revision++
	if err := checkpointNomadState(ctx, req, state); err != nil {
		return nil, r.abortRegisteredNomadLaunch(ctx, req, state, fmt.Errorf("checkpoint registered Nomad job: %w", err))
	}
	return nomadClient, nil
}
