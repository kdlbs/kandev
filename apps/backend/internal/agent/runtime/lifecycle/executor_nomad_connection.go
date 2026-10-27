package lifecycle

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/nomad"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
)

const nomadDrainCapability = "runtime-seal-drain.v1"

type nomadAgentctlConnection struct {
	instance          *ExecutorInstance
	sshClient         *ssh.Client
	transport         nomadWorkerTransport
	controlForwarder  nomadPortForward
	instanceForwarder nomadPortForward
	runtimeAPITunnel  *sshRuntimeAPITunnel
	controlClient     *agentctl.ControlClient
	closeOnce         sync.Once
}

func (c *nomadAgentctlConnection) closeHostResources() {
	if c == nil {
		return
	}
	c.closeOnce.Do(func() {
		if c.instanceForwarder != nil {
			_ = c.instanceForwarder.Close()
		}
		if c.controlForwarder != nil {
			_ = c.controlForwarder.Close()
		}
		if c.runtimeAPITunnel != nil {
			_ = c.runtimeAPITunnel.Close()
		}
		if c.controlClient != nil {
			c.controlClient.Close()
		}
		if c.transport != nil {
			_ = c.transport.Close()
		} else if c.sshClient != nil {
			_ = c.sshClient.Close()
		}
	})
}

// connectNomadAgentctlOverSSH connects to the one Nomad-owned controller. It
// only opens SSH transports and the agentctl instance; it never starts or
// stops a process through the SSH executor's launch/teardown scripts.
func connectNomadAgentctlOverSSH(
	ctx context.Context,
	client *ssh.Client,
	profile nomad.ProfileConfig,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
	bootstrapNonce string,
	log *logger.Logger,
) (*nomadAgentctlConnection, error) {
	return connectNomadAgentctlOverSSHWithTunnel(ctx, client, profile, state, req, bootstrapNonce, log, nil)
}

func connectNomadAgentctlOverSSHWithTunnel(
	ctx context.Context,
	client *ssh.Client,
	profile nomad.ProfileConfig,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
	bootstrapNonce string,
	log *logger.Logger,
	preparedTunnel *sshRuntimeAPITunnel,
) (*nomadAgentctlConnection, error) {
	if client == nil {
		return nil, errors.New("connect Nomad agentctl: pinned SSH client is required")
	}
	conn := &nomadAgentctlConnection{sshClient: client, transport: nomadSSHTransport{client: client}}
	return conn.connect(ctx, profile, state, req, bootstrapNonce, log, preparedTunnel)
}

// connectNomadAgentctlOverTransport is the pool attach: the transport is SSH
// for a remote node and the Kandev host itself for a local one.
func connectNomadAgentctlOverTransport(
	ctx context.Context,
	transport nomadWorkerTransport,
	profile nomad.ProfileConfig,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
	bootstrapNonce string,
	log *logger.Logger,
	preparedTunnel *sshRuntimeAPITunnel,
) (*nomadAgentctlConnection, error) {
	if transport == nil {
		return nil, errors.New("connect Nomad agentctl: a worker transport is required")
	}
	conn := &nomadAgentctlConnection{transport: transport}
	if sshT, ok := transport.(nomadSSHTransport); ok {
		conn.sshClient = sshT.client
	}
	return conn.connect(ctx, profile, state, req, bootstrapNonce, log, preparedTunnel)
}

func (conn *nomadAgentctlConnection) connect(
	ctx context.Context,
	profile nomad.ProfileConfig,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
	bootstrapNonce string,
	log *logger.Logger,
	preparedTunnel *sshRuntimeAPITunnel,
) (*nomadAgentctlConnection, error) {
	if state == nil || req == nil || log == nil || (bootstrapNonce == "" && req.AuthToken == "") {
		return nil, errors.New("connect Nomad agentctl: pinned SSH client, state, request, logger, and persisted auth or bootstrap credential are required")
	}
	phaseAllowed := state.Phase == nomadPhaseAllocated || (req.RequireExistingAgentctlInstance && state.Phase == nomadPhaseReady)
	if err := state.Validate(); err != nil || !phaseAllowed || state.NodeID != profile.NodeID || state.ControlPort < 1 || state.InstancePort < 1 {
		return nil, errors.New("connect Nomad agentctl: exact allocated worker identity and ports are required")
	}
	if err := conn.establish(ctx, state, req, bootstrapNonce, log, preparedTunnel); err != nil {
		conn.closeHostResources()
		return nil, err
	}
	return conn, nil
}

// establish opens the SSH transports and verifies the exact agentctl instance.
// The caller closes the host resources when it returns an error.
func (conn *nomadAgentctlConnection) establish(
	ctx context.Context,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
	bootstrapNonce string,
	log *logger.Logger,
	preparedTunnel *sshRuntimeAPITunnel,
) error {
	client := conn.sshClient
	if preparedTunnel != nil {
		conn.runtimeAPITunnel = preparedTunnel
	} else if client != nil {
		tunnel, _, err := openSSHRuntimeAPITunnelForRequest(client, req)
		if err != nil {
			return fmt.Errorf("connect Nomad agentctl: open reverse Kandev API tunnel: %w", err)
		}
		conn.runtimeAPITunnel = tunnel
	}
	controlForwarder, err := conn.transport.Forward(state.ControlPort, log)
	if err != nil {
		return fmt.Errorf("connect Nomad agentctl: forward control port: %w", err)
	}
	conn.controlForwarder = controlForwarder
	control := agentctl.NewControlClient(sshAgentctlLoopbackHost, controlForwarder.LocalPort(), log,
		agentctl.WithControlAuthToken(req.AuthToken))
	conn.controlClient = control
	if req.AuthToken == "" {
		if err := bootstrapNomadAgentctlAuth(ctx, control, controlForwarder.LocalPort(), state, req, bootstrapNonce); err != nil {
			return err
		}
	}
	identity, err := verifyNomadAgentctlIdentity(ctx, control, state)
	if err != nil {
		return err
	}
	created, createdNow, err := ensureNomadAgentctlInstance(ctx, control, state, req)
	if err != nil {
		return err
	}
	if err := conn.attachNomadAgentctlInstance(ctx, created, createdNow, identity, state, req, log); err != nil {
		return err
	}
	conn.instance = &ExecutorInstance{
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RuntimeName: agentruntime.RuntimeNomad,
		Client: agentctl.NewClient(sshAgentctlLoopbackHost, conn.instanceForwarder.LocalPort(), log,
			agentctl.WithExecutionID(req.InstanceID), agentctl.WithSessionID(req.SessionID),
			agentctl.WithAuthToken(control.AuthToken())),
		WorkspacePath:  state.WorkspacePath,
		Metadata:       req.Metadata,
		AuthToken:      control.AuthToken(),
		BootstrapNonce: bootstrapNonce,
	}
	if state.PlacementMode == nomad.ModePool && state.NodeAttach == nomad.AttachLocal {
		conn.instance.LocalWorktreeBranch = nomad.PoolWorktreeBranch(state.EnvironmentID)
	}
	log.Info("connected to Nomad-owned agentctl", zap.String("server_identity", identity.ServerIdentity))
	return nil
}

// bootstrapNomadAgentctlAuth spends the one-use bootstrap nonce and durably
// records the resulting agentctl auth token before instance creation.
func bootstrapNomadAgentctlAuth(
	ctx context.Context,
	control *agentctl.ControlClient,
	localPort int,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
	bootstrapNonce string,
) error {
	// Nomad reports the raw_exec task as running before agentctl has
	// necessarily bound its control listener. Wait for the actual HTTP
	// health endpoint before spending the one-use bootstrap nonce.
	if err := waitAgentctlHealthy(ctx, localPort, sshAgentctlHealthTimeout); err != nil {
		return errors.New("connect Nomad agentctl: control server unavailable before bootstrap handshake")
	}
	token, err := control.Handshake(ctx, bootstrapNonce)
	if err != nil {
		return categorizeNomadBootstrapHandshakeError(err)
	}
	if req.PersistNomadRuntimeSecret == nil {
		return errors.New("connect Nomad agentctl: durable auth-token persistence is unavailable")
	}
	if err := req.PersistNomadRuntimeSecret(ctx, MetadataKeyAuthTokenSecret, token); err != nil {
		return fmt.Errorf("connect Nomad agentctl: persist auth token before instance creation: %w", err)
	}
	req.AuthToken = token
	state.Revision++
	if err := checkpointNomadState(ctx, req, state); err != nil {
		return fmt.Errorf("connect Nomad agentctl: checkpoint auth token before instance creation: %w", err)
	}
	return nil
}

func verifyNomadAgentctlIdentity(ctx context.Context, control *agentctl.ControlClient, state *nomad.EnvironmentState) (*agentctl.IdentityInfo, error) {
	identity, err := control.GetIdentity(ctx)
	if err != nil || identity == nil || strings.TrimSpace(identity.ServerIdentity) == "" || !containsNomadCapability(identity.Capabilities, nomadDrainCapability) {
		return nil, errors.New("connect Nomad agentctl: exact server identity or required drain capability is unavailable")
	}
	if state.ServerIdentity != "" && identity.ServerIdentity != state.ServerIdentity {
		return nil, errors.New("connect Nomad agentctl: server identity differs from the retained operation")
	}
	return identity, nil
}

// ensureNomadAgentctlInstance finds the retained instance or creates it, and
// proves it matches the Nomad allocation. Only an instance created by this exact
// request may be rolled back; a mismatching pre-existing instance is
// foreign/uncertain state.
func ensureNomadAgentctlInstance(
	ctx context.Context,
	control *agentctl.ControlClient,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
) (*agentctl.InstanceInfo, bool, error) {
	created, err := control.GetInstance(ctx, req.InstanceID)
	createdNow := false
	if errors.Is(err, agentctl.ErrInstanceNotFound) {
		created, err = createNomadAgentctlInstance(ctx, control, state, req)
		if err != nil {
			return nil, false, err
		}
		createdNow = true
	} else if err != nil {
		return nil, false, fmt.Errorf("connect Nomad agentctl: inspect retained instance: %w", err)
	}
	if created == nil || created.ID != req.InstanceID || created.Port != state.InstancePort ||
		created.TaskID != req.TaskID || created.SessionID != req.SessionID || created.WorkspacePath != state.WorkspacePath {
		if created != nil && created.ID != "" && createdNow {
			deleteNomadAgentctlInstance(control, created.ID)
		}
		return nil, false, errors.New("connect Nomad agentctl: retained instance owner, workspace, identity, or reserved port does not match Nomad allocation")
	}
	return created, createdNow, nil
}

func createNomadAgentctlInstance(
	ctx context.Context,
	control *agentctl.ControlClient,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
) (*agentctl.InstanceInfo, error) {
	if req.RequireExistingAgentctlInstance {
		return nil, errors.New("connect Nomad agentctl: retained instance is absent; refusing to recreate during recovery")
	}
	createReq := buildSSHCreateInstanceRequest(req, state.WorkspacePath, "")
	response, createErr := control.CreateInstance(ctx, &createReq)
	if createErr != nil {
		// The response may have been lost after instance creation. Leave it
		// for the next exact-operation retry to discover with GET.
		return nil, fmt.Errorf("connect Nomad agentctl: create instance failed: %w", createErr)
	}
	if response == nil {
		return nil, errors.New("connect Nomad agentctl: create instance returned no identity")
	}
	return &agentctl.InstanceInfo{
		ID: response.ID, Port: response.Port, TaskID: req.TaskID, SessionID: req.SessionID,
		WorkspacePath: state.WorkspacePath,
	}, nil
}

// attachNomadAgentctlInstance forwards the instance port, waits for health, and
// checkpoints the ready phase with the proven server identity. On failure the
// instance is deleted only when this attempt created it; a pre-existing
// instance may still be live and is left for the next attempt.
func (conn *nomadAgentctlConnection) attachNomadAgentctlInstance(
	ctx context.Context,
	created *agentctl.InstanceInfo,
	createdNow bool,
	identity *agentctl.IdentityInfo,
	state *nomad.EnvironmentState,
	req *ExecutorCreateRequest,
	log *logger.Logger,
) error {
	control := conn.controlClient
	rollback := func() {
		if createdNow {
			deleteNomadAgentctlInstance(control, created.ID)
		}
	}
	instanceForwarder, err := conn.transport.Forward(created.Port, log)
	if err != nil {
		rollback()
		return fmt.Errorf("connect Nomad agentctl: forward instance port: %w", err)
	}
	conn.instanceForwarder = instanceForwarder
	if err := waitAgentctlHealthy(ctx, instanceForwarder.LocalPort(), sshAgentctlHealthTimeout); err != nil {
		rollback()
		return fmt.Errorf("connect Nomad agentctl: instance health check failed: %w", err)
	}
	if state.ServerIdentity != identity.ServerIdentity || state.Phase != nomadPhaseReady {
		state.ServerIdentity = identity.ServerIdentity
		state.Phase = nomadPhaseReady
		state.Revision++
		if err := checkpointNomadState(ctx, req, state); err != nil {
			rollback()
			return fmt.Errorf("connect Nomad agentctl: checkpoint server identity: %w", err)
		}
	}
	return nil
}

// categorizeNomadBootstrapHandshakeError deliberately keeps the underlying
// HTTP/client error private: it may contain request details. Only expose a
// small diagnostic class so a consumed nonce or credential is never logged.
func categorizeNomadBootstrapHandshakeError(err error) error {
	if err == nil {
		return nil
	}
	if status, ok := nomadHandshakeHTTPStatus(err.Error()); ok {
		if status == 401 || status == 403 {
			return fmt.Errorf("connect Nomad agentctl: bootstrap credential rejected (HTTP %d)", status)
		}
		return fmt.Errorf("connect Nomad agentctl: bootstrap handshake HTTP response rejected (HTTP %d)", status)
	}
	return errors.New("connect Nomad agentctl: bootstrap handshake transport/unavailable failure")
}

// nomadHandshakeHTTPStatus extracts a three-digit status from the last
// "status NNN" marker of a handshake error message.
func nomadHandshakeHTTPStatus(message string) (int, bool) {
	marker := strings.LastIndex(message, "status ")
	if marker < 0 {
		return 0, false
	}
	rest := message[marker+len("status "):]
	end := 0
	for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
		end++
	}
	if end != 3 {
		return 0, false
	}
	status, parseErr := strconv.Atoi(rest[:end])
	return status, parseErr == nil
}

// errNomadWorkerFenceAbsent reports that the worker holds no owner record at
// all, as opposed to a record that belongs to something else.
var errNomadWorkerFenceAbsent = errors.New("verify Nomad worker fence: no owner record is present")

func verifyNomadWorkerFence(ctx context.Context, client *ssh.Client, profile nomad.ProfileConfig, state nomad.EnvironmentState) error {
	if client == nil {
		return errors.New("verify Nomad worker fence: exact allocation ownership is incomplete")
	}
	return verifyNomadWorkerFenceOver(ctx, nomadSSHTransport{client: client}, profile, state)
}

func verifyNomadWorkerFenceOver(ctx context.Context, transport nomadWorkerTransport, profile nomad.ProfileConfig, state nomad.EnvironmentState) error {
	if transport == nil || state.AllocationID == "" || state.NodeID != profile.NodeID || state.JobID == "" ||
		state.OperationID == "" || state.InputDigest == "" {
		return errors.New("verify Nomad worker fence: exact allocation ownership is incomplete")
	}
	fenceOwnerPath := filepath.Join(nomad.FenceDir(state, profile.AllocationRoot), "owner")
	owner := strings.Join([]string{
		state.InstallationID, state.NodeID, state.EnvironmentID, fmt.Sprint(state.Generation),
		state.OperationID, state.InputDigest, state.JobID,
	}, ":") + "\n" + state.AllocationID
	command := "f=" + shellQuote(fenceOwnerPath) +
		`; if [ -L "$f" ]; then :; elif [ ! -e "$f" ]; then printf absent` +
		`; elif [ -f "$f" ] && [ "$(stat -f %Lp "$f")" = 600 ] && [ "$(cat "$f")" = ` + shellQuote(owner) + ` ]; then printf verified; fi`
	output, _, err := transport.Run(ctx, command)
	if err != nil {
		return errors.New("verify Nomad worker fence: exact private owner record is unavailable")
	}
	switch strings.TrimSpace(output) {
	case "verified":
		return nil
	case "absent":
		return errNomadWorkerFenceAbsent
	}
	return errors.New("verify Nomad worker fence: exact private owner record is unavailable")
}

func deleteNomadAgentctlInstance(control *agentctl.ControlClient, instanceID string) {
	if control == nil || instanceID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = control.DeleteInstance(ctx, instanceID)
}

func containsNomadCapability(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type nomadWorkerPaths struct {
	ArtifactPath    string
	BootstrapPath   string
	EnvironmentPath string
}

func nomadWorkerPathsFor(profile nomad.ProfileConfig, state nomad.EnvironmentState, artifactDigest string) (nomadWorkerPaths, error) {
	if state.NodeID != profile.NodeID || state.EnvironmentID == "" || state.JobID == "" || len(artifactDigest) != 64 {
		return nomadWorkerPaths{}, errors.New("nomad worker paths require matching immutable identity and SHA-256 artifact digest")
	}
	for _, r := range artifactDigest {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return nomadWorkerPaths{}, errors.New("nomad artifact digest is invalid")
		}
	}
	root := filepath.Join(profile.AllocationRoot, state.JobID)
	bootstrapBase := filepath.Join(root, "bootstrap-"+artifactDigest)
	return nomadWorkerPaths{
		ArtifactPath:    filepath.Join(root, "agentctl-"+artifactDigest),
		BootstrapPath:   bootstrapBase + ".sh",
		EnvironmentPath: bootstrapBase + ".sh.env",
	}, nil
}

func stageNomadWorkerFilesOver(ctx context.Context, client nomadWorkerTransport, profile nomad.ProfileConfig, state nomad.EnvironmentState, artifact []byte, bootstrapEnv map[string]string) (nomadWorkerPaths, error) {
	digest := sha256.Sum256(artifact)
	digestHex := hex.EncodeToString(digest[:])
	paths, err := nomadWorkerPathsFor(profile, state, digestHex)
	if err != nil {
		return nomadWorkerPaths{}, err
	}
	script, err := nomad.BuildBootstrapScript(state, profile.AllocationRoot)
	if err != nil {
		return nomadWorkerPaths{}, err
	}
	envFile, err := nomad.EncodePrivateEnvironment(bootstrapEnv)
	if err != nil {
		return nomadWorkerPaths{}, err
	}
	dir := filepath.Dir(paths.BootstrapPath)
	if _, _, err := client.Run(ctx, "mkdir -p "+shellQuote(dir)); err != nil {
		return nomadWorkerPaths{}, fmt.Errorf("prepare Nomad worker directory: %w", err)
	}
	if err := client.Upload(ctx, paths.ArtifactPath, artifact, 0o500); err != nil {
		return nomadWorkerPaths{}, fmt.Errorf("stage verified agentctl artifact: %w", err)
	}
	if err := client.Upload(ctx, paths.BootstrapPath, []byte(script), 0o700); err != nil {
		return nomadWorkerPaths{}, fmt.Errorf("stage Nomad bootstrap: %w", err)
	}
	command := "umask 077; cat > " + shellQuote(paths.EnvironmentPath) + " && chmod 0600 " + shellQuote(paths.EnvironmentPath)
	if _, _, err := client.RunStdin(ctx, command, bytes.NewReader(envFile)); err != nil {
		return nomadWorkerPaths{}, fmt.Errorf("stage private Nomad bootstrap environment: %w", err)
	}
	remoteDigest, _, err := client.Run(ctx, "shasum -a 256 "+shellQuote(paths.ArtifactPath))
	if err != nil {
		return nomadWorkerPaths{}, fmt.Errorf("verify staged Nomad artifact: %w", err)
	}
	fields := strings.Fields(remoteDigest)
	if len(fields) == 0 || fields[0] != digestHex {
		return nomadWorkerPaths{}, errors.New("staged Nomad artifact digest does not match the selected local artifact")
	}
	return paths, nil
}
