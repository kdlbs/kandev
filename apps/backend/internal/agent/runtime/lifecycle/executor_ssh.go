package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

const (
	sshAgentctlHealthTimeout  = 20 * time.Second
	sshAgentctlCleanupTimeout = 20 * time.Second
	sshAgentctlLoopbackHost   = "127.0.0.1"

	sshStatusRunning      = "running"
	sshStatusUnknown      = "unknown"
	sshStatusDisconnected = "disconnected"
	sshStatusAgentctlDown = "agentctl-down"
)

// sshSessionState tracks per-session resources we need to clean up later:
// the SSH target (so we can dial again on Stop), the per-session SSH client
// (no shared pool — the Go SSH client's direct-tcpip mux gets unreliable
// when many short-lived channels race the long-lived stream channels), and
// the local port forwarder.
type sshSessionState struct {
	target         *SSHTarget
	client         *ssh.Client
	forwarder      *SSHPortForwarder
	agentctlClient *agentctl.Client
	pid            int
	remoteDir      string
	authToken      string
	reusingProcess bool
}

// SSHExecutor implements ExecutorBackend for SSH-reachable Linux and macOS hosts.
//
// Each session owns its own *ssh.Client (no shared pool). One SSH connection
// per session keeps teardown simple — closing the executor instance closes
// the client — at the cost of an extra TCP+handshake per session on the same
// host. See docs/specs/ssh-executor/spec.md for the full design.
type SSHExecutor struct {
	agentctlResolver *AgentctlResolver
	secretStore      secrets.SecretStore
	agentList        RemoteAuthAgentLister
	logger           *logger.Logger

	mu       sync.Mutex
	sessions map[string]*sshSessionState // keyed by ExecutorInstance.InstanceID
}

// NewSSHExecutor wires up an SSH executor with shared infrastructure.
// agentList is optional; pass nil if no agent install scripts are needed.
func NewSSHExecutor(
	secretStore secrets.SecretStore,
	agentList RemoteAuthAgentLister,
	resolver *AgentctlResolver,
	log *logger.Logger,
) *SSHExecutor {
	return &SSHExecutor{
		agentctlResolver: resolver,
		secretStore:      secretStore,
		agentList:        agentList,
		logger:           log.WithFields(zap.String("runtime", "ssh")),
		sessions:         make(map[string]*sshSessionState),
	}
}

func (r *SSHExecutor) Name() executor.Name { return executor.NameSSH }

func (r *SSHExecutor) HealthCheck(_ context.Context) error {
	// SSH targets are configured per-executor — the runtime itself is always
	// available. Per-host reachability is verified by the test-connection
	// endpoint and surfaced in the executor status panel.
	return nil
}

// Close terminates every still-tracked SSH session. Normal teardown happens
// session-by-session via StopInstance; Close is the shutdown safety net for
// sessions whose StopInstance didn't run (e.g. a hard kandev exit).
func (r *SSHExecutor) Close() error {
	r.mu.Lock()
	states := make([]*sshSessionState, 0, len(r.sessions))
	for id, s := range r.sessions {
		states = append(states, s)
		delete(r.sessions, id)
	}
	r.mu.Unlock()
	for _, s := range states {
		if s.forwarder != nil {
			_ = s.forwarder.Close()
		}
		if s.client != nil {
			_ = s.client.Close()
		}
	}
	return nil
}

// targetFromMetadata builds an SSHTarget from the executor metadata
// propagated via buildLaunchMetadata (req.ExecutorConfig keys merged in).
func (r *SSHExecutor) targetFromMetadata(md map[string]interface{}) (*SSHTarget, error) {
	host := getMetadataString(md, MetadataKeySSHHost)
	hostAlias := getMetadataString(md, MetadataKeySSHHostAlias)
	if host == "" && hostAlias == "" {
		return nil, errors.New("ssh executor: host (or host_alias) is required in executor config")
	}
	port := 0
	if p := getMetadataString(md, MetadataKeySSHPort); p != "" {
		// Reject malformed / out-of-range ports loudly instead of silently
		// falling back to 22 — that fallback would mask an obviously bad
		// config (e.g. ssh_port="abc" or ssh_port="0") and route every
		// launch through a port the user never meant to use.
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("ssh executor: invalid ssh_port %q (must be 1-65535)", p)
		}
		port = n
	}
	identitySource := SSHIdentitySource(getMetadataString(md, MetadataKeySSHIdentitySource))
	identityFile := getMetadataString(md, MetadataKeySSHIdentityFile)
	cfg := SSHConnConfig{
		HostAlias:         hostAlias,
		Host:              host,
		Port:              port,
		User:              getMetadataString(md, MetadataKeySSHUser),
		IdentitySource:    identitySource,
		IdentityFile:      identityFile,
		ProxyJump:         getMetadataString(md, MetadataKeySSHProxyJump),
		PinnedFingerprint: getMetadataString(md, MetadataKeySSHHostFingerprint),
	}
	if cfg.PinnedFingerprint == "" {
		return nil, errors.New("ssh executor: host_fingerprint is required — open the SSH executor connection settings, run Test connection, and trust the host")
	}
	return ResolveSSHTarget(cfg)
}

// workdirRoot returns the per-profile or per-executor workdir root (defaults
// to ~/.kandev). Per-profile wins over per-executor.
func (r *SSHExecutor) workdirRoot(md map[string]interface{}) string {
	if w := getMetadataString(md, MetadataKeySSHWorkdirRoot); w != "" {
		return w
	}
	return sshDefaultWorkdir
}

// CreateInstance opens (or reuses) an SSH connection to the configured host,
// ensures agentctl is uploaded, provisions the per-task remote dir, launches
// a per-session agentctl process, and sets up a local port forward so the
// kandev backend can speak to it as if it were on localhost.
//
// If ResumeRemoteInstance already attached this InstanceID (e.g. after a
// backend restart), reuse the resumed SSH client + forwarder + remote pid
// instead of starting a second remote agentctl on top of the live one.
func (r *SSHExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	r.mu.Lock()
	resumed, ok := r.sessions[req.InstanceID]
	r.mu.Unlock()
	if ok {
		return r.buildResumedInstance(req, resumed), nil
	}

	target, err := r.targetFromMetadata(req.Metadata)
	if err != nil {
		return nil, err
	}
	client, err := dialSSH(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("ssh: connect to %s@%s: %w", target.User, target.Host, err)
	}
	released := false
	defer func() {
		if !released {
			_ = client.Close()
		}
	}()
	r.report(req.OnProgress, "Connecting to SSH host", PrepareStepCompleted, "")

	agentctlBin, platform, err := r.prepareRemoteHost(ctx, client, req)
	if err != nil {
		return nil, err
	}

	if err := r.preflightAgentBinary(ctx, client, req, platform); err != nil {
		return nil, err
	}

	workdir := r.workdirRoot(req.Metadata)
	taskDir, sessionDir, err := r.prepareRemoteDirs(ctx, client, workdir, req)
	if err != nil {
		return nil, err
	}
	r.maybeUploadCredentials(ctx, client, req, platform)

	port, pid, fwd, authToken, err := r.startAndForwardAgentctl(ctx, client, agentctlBin, taskDir, sessionDir, req, platform)
	if err != nil {
		return nil, err
	}

	r.mu.Lock()
	r.sessions[req.InstanceID] = &sshSessionState{
		target:    target,
		client:    client,
		forwarder: fwd,
		pid:       pid,
		remoteDir: sessionDir,
		authToken: authToken,
	}
	r.mu.Unlock()
	released = true // ownership transferred to session state; released on StopInstance

	return r.buildInstance(req, target, fwd, taskDir, sessionDir, port, pid, workdir, authToken), nil
}

// prepareRemoteHost runs the steps that are independent of any particular
// task: detect remote OS/arch and ensure the agentctl binary is on the host.
// Returns the resolved remote agentctl path.
func (r *SSHExecutor) prepareRemoteHost(
	ctx context.Context,
	client *ssh.Client,
	req *ExecutorCreateRequest,
) (string, SSHRemotePlatform, error) {
	info, err := detectRemoteInfo(ctx, client)
	if err != nil {
		r.report(req.OnProgress, "Detecting remote OS", PrepareStepFailed, err.Error())
		return "", SSHRemotePlatform{}, fmt.Errorf("ssh: detect remote: %w", err)
	}
	if err := requireSupportedRemotePlatform(info.Platform); err != nil {
		r.report(req.OnProgress, "Detecting remote OS", PrepareStepFailed, err.Error())
		return "", info.Platform, err
	}
	r.report(req.OnProgress, "Detecting remote OS", PrepareStepCompleted, info.UnameAll)

	agentctlBin, err := ensureAgentctlOnHost(ctx, client, r.agentctlResolver, info.Platform, r.logger)
	if err != nil {
		r.report(req.OnProgress, "Uploading agent controller", PrepareStepFailed, err.Error())
		return "", info.Platform, err
	}
	r.report(req.OnProgress, "Uploading agent controller", PrepareStepCompleted, "")
	return agentctlBin, info.Platform, nil
}

// prepareRemoteDirs makes <workdir>/tasks/<taskDir> and <taskDir>/.kandev/sessions/<sid>.
func (r *SSHExecutor) prepareRemoteDirs(ctx context.Context, client *ssh.Client, workdir string, req *ExecutorCreateRequest) (string, string, error) {
	taskDir, err := ensureRemoteTaskDir(ctx, client, workdir, sshTaskDirName(req))
	if err != nil {
		r.report(req.OnProgress, "Preparing task directory", PrepareStepFailed, err.Error())
		return "", "", err
	}
	sessionDir, err := ensureRemoteSessionDir(ctx, client, taskDir, req.SessionID)
	if err != nil {
		r.report(req.OnProgress, "Preparing task directory", PrepareStepFailed, err.Error())
		return "", "", err
	}
	r.report(req.OnProgress, "Preparing task directory", PrepareStepCompleted, taskDir)
	return taskDir, sessionDir, nil
}

// startAndForwardAgentctl spawns the per-session agentctl on the remote,
// creates a per-instance sub-server inside it, and stands up the SSH local
// port forward to the *instance* port (not the control port). Returns the
// instance port, the agentctl PID, and the forwarder. Cleans up everything
// it created on any failure.
//
// Lifecycle mirrors what executor_sprites.go does inside its sprite:
//  1. agentctl --workdir <taskDir> binds the control API on a random port.
//  2. POST /api/v1/instances on that control port creates a session-scoped
//     sub-server with its own port (allocated from agentctl's instance pool).
//  3. The SSH executor's clients (agent stream, workspace, etc.) all talk to
//     that sub-server, so the local port forward points there.
func (r *SSHExecutor) startAndForwardAgentctl(
	ctx context.Context,
	client *ssh.Client,
	agentctlBin, taskDir, sessionDir string,
	req *ExecutorCreateRequest,
	platform SSHRemotePlatform,
) (int, int, *SSHPortForwarder, string, error) {
	nonce, err := generateBootstrapNonce()
	if err != nil {
		return 0, 0, nil, "", fmt.Errorf("ssh: generate bootstrap nonce: %w", err)
	}
	env := sshAgentctlLaunchEnv(req.Env, nonce)
	shell := sshShellForRemote(req.Metadata, platform)
	controlPort, pid, err := startRemoteAgentctl(ctx, client, shell, agentctlBin, taskDir, sessionDir, env, r.logger)
	if err != nil {
		r.report(req.OnProgress, "Starting agent controller", PrepareStepFailed, err.Error())
		return 0, 0, nil, "", err
	}
	r.report(req.OnProgress, "Starting agent controller", PrepareStepCompleted,
		fmt.Sprintf("pid=%d control_port=%d", pid, controlPort))

	// The per-instance server's workspace is the remote task dir, not the
	// host-side req.WorkspacePath (which is meaningless on the remote).
	// Passed explicitly so we don't briefly mutate the caller's request.
	authToken, ierr := remoteControlHandshake(ctx, client, controlPort, nonce)
	if ierr != nil {
		_ = stopRemoteAgentctl(ctx, client, sessionDir, pid)
		return 0, 0, nil, "", ierr
	}
	instancePort, ierr := createRemoteAgentInstance(ctx, client, controlPort, taskDir, req, authToken, r.logger)
	if ierr != nil {
		_ = stopRemoteAgentctl(ctx, client, sessionDir, pid)
		r.report(req.OnProgress, "Creating agent instance", PrepareStepFailed, ierr.Error())
		return 0, 0, nil, "", ierr
	}
	r.report(req.OnProgress, "Creating agent instance", PrepareStepCompleted,
		fmt.Sprintf("instance_port=%d", instancePort))

	fwd, err := StartPortForward(client, instancePort, r.logger)
	if err != nil {
		_ = stopRemoteAgentctl(ctx, client, sessionDir, pid)
		return 0, 0, nil, "", fmt.Errorf("ssh: port forward: %w", err)
	}
	if err := waitAgentctlHealthy(ctx, fwd.LocalPort(), sshAgentctlHealthTimeout); err != nil {
		_ = fwd.Close()
		_ = stopRemoteAgentctl(ctx, client, sessionDir, pid)
		return 0, 0, nil, "", fmt.Errorf("ssh: agentctl health: %w", err)
	}
	r.report(req.OnProgress, "Connecting to agent controller", PrepareStepCompleted,
		fmt.Sprintf("local:%d -> remote:%d", fwd.LocalPort(), instancePort))
	return instancePort, pid, fwd, authToken, nil
}

func sshAgentctlLaunchEnv(base map[string]string, nonce string) map[string]string {
	env := make(map[string]string, len(base)+2)
	for key, value := range base {
		env[key] = value
	}
	// The handshake returns the bearer token to the backend; never pass it to
	// the launched agent process. SSH reaches both listeners through explicit
	// loopback direct-tcpip forwards, so it need not expose them remotely.
	delete(env, "AGENTCTL_AUTH_TOKEN")
	env["AGENTCTL_BOOTSTRAP_NONCE"] = nonce
	env["AGENTCTL_LISTEN_HOST"] = sshAgentctlLoopbackHost
	return env
}

func (r *SSHExecutor) buildInstance(
	req *ExecutorCreateRequest,
	target *SSHTarget,
	fwd *SSHPortForwarder,
	taskDir, sessionDir string,
	port, pid int,
	workdir, authToken string,
) *ExecutorInstance {
	return &ExecutorInstance{
		InstanceID:  req.InstanceID,
		TaskID:      req.TaskID,
		SessionID:   req.SessionID,
		RuntimeName: r.Name(),
		Client: agentctl.NewClient(sshAgentctlLoopbackHost, fwd.LocalPort(), r.logger,
			agentctl.WithExecutionID(req.InstanceID),
			agentctl.WithSessionID(req.SessionID), agentctl.WithAuthToken(authToken)),
		WorkspacePath: taskDir,
		AuthToken:     authToken,
		Metadata: map[string]interface{}{
			MetadataKeySSHHost:               target.Host,
			MetadataKeySSHPort:               strconv.Itoa(target.Port),
			MetadataKeySSHUser:               target.User,
			MetadataKeySSHHostFingerprint:    target.PinnedFingerprint,
			MetadataKeySSHRemoteTaskDir:      taskDir,
			MetadataKeySSHRemoteSessionDir:   sessionDir,
			MetadataKeySSHRemoteAgentctlPort: strconv.Itoa(port),
			MetadataKeySSHRemoteAgentctlPID:  strconv.Itoa(pid),
			MetadataKeySSHLocalForwardPort:   strconv.Itoa(fwd.LocalPort()),
			MetadataKeySSHWorkdirRoot:        workdir,
			MetadataKeyIsRemote:              true,
		},
	}
}

// buildResumedInstance constructs an ExecutorInstance from already-attached
// session state set up by ResumeRemoteInstance. No new dials / uploads / agent
// launches — the remote agentctl is still running from the previous session.
// Metadata mirrors the original launch's values so downstream consumers
// don't see a partial view.
func (r *SSHExecutor) buildResumedInstance(req *ExecutorCreateRequest, state *sshSessionState) *ExecutorInstance {
	port, _ := strconv.Atoi(getMetadataString(req.Metadata, MetadataKeySSHRemoteAgentctlPort))
	taskDir := getMetadataString(req.Metadata, MetadataKeySSHRemoteTaskDir)
	workdir := r.workdirRoot(req.Metadata)
	client := state.agentctlClient
	if client == nil {
		client = agentctl.NewClient(sshAgentctlLoopbackHost, state.forwarder.LocalPort(), r.logger,
			agentctl.WithExecutionID(resumedSSHAgentctlInstanceID(req)),
			agentctl.WithSessionID(req.SessionID), agentctl.WithAuthToken(state.authToken))
	}
	return &ExecutorInstance{
		InstanceID:    req.InstanceID,
		TaskID:        req.TaskID,
		SessionID:     req.SessionID,
		RuntimeName:   r.Name(),
		Client:        client,
		WorkspacePath: taskDir,
		AuthToken:     state.authToken,
		Metadata: map[string]interface{}{
			MetadataKeySSHHost:               state.target.Host,
			MetadataKeySSHPort:               strconv.Itoa(state.target.Port),
			MetadataKeySSHUser:               state.target.User,
			MetadataKeySSHHostFingerprint:    state.target.PinnedFingerprint,
			MetadataKeySSHRemoteTaskDir:      taskDir,
			MetadataKeySSHRemoteSessionDir:   state.remoteDir,
			MetadataKeySSHRemoteAgentctlPort: strconv.Itoa(port),
			MetadataKeySSHRemoteAgentctlPID:  strconv.Itoa(state.pid),
			MetadataKeySSHLocalForwardPort:   strconv.Itoa(state.forwarder.LocalPort()),
			MetadataKeySSHWorkdirRoot:        workdir,
			MetadataKeyIsRemote:              true,
			"reuse_existing_process":         state.reusingProcess,
		},
	}
}

// StopInstance closes the local port forward and SSH connection for every
// stop. It preserves the remote agentctl only for graceful backend shutdown,
// so a restart can reconnect using the persisted remote PID, port, and token.
// User stops, rollbacks, force, failed-agent, terminal, and stale-replacement
// stops all kill agentctl.
// The task directory is always left intact; v2 housekeeping will sweep stale
// dirs.
func (r *SSHExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	if instance == nil {
		return nil
	}
	r.mu.Lock()
	state := r.sessions[instance.InstanceID]
	delete(r.sessions, instance.InstanceID)
	r.mu.Unlock()
	if state == nil {
		r.logger.Debug("stop: no tracked SSH session state for instance",
			zap.String("instance_id", instance.InstanceID))
		return nil
	}
	if state.forwarder != nil {
		_ = state.forwarder.Close()
	}
	if !sshShouldStopRemoteAgentctl(instance, force) {
		r.logger.Info("preserving SSH agentctl after agent stop",
			zap.String("instance_id", instance.InstanceID),
			zap.String("stop_reason", instance.StopReason))
		if state.client != nil {
			_ = state.client.Close()
		}
		return nil
	}

	// Use the same SSH client we used for CreateInstance — if it's still
	// alive we can kill the remote agentctl gracefully; otherwise just drop
	// the connection on the floor.
	if state.client != nil {
		cleanupCtx, cancel := sshRemoteCleanupContext(ctx)
		_ = stopRemoteAgentctl(cleanupCtx, state.client, state.remoteDir, state.pid)
		cancel()
		_ = state.client.Close()
	}
	return nil
}

func sshShouldStopRemoteAgentctl(instance *ExecutorInstance, force bool) bool {
	if instance == nil {
		return false
	}
	return force || instance.AgentStopFailed || instance.StopReason != StopReasonBackendShutdown
}

func sshRemoteCleanupContext(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), sshAgentctlCleanupTimeout)
}

// RecoverInstances re-opens SSH connections for sessions that were live before
// a backend restart. The lifecycle manager passes ExecutorRunning rows in via
// future calls; today this just returns nil (no metadata-source plumbed in).
// Recovery semantics are documented in the spec; persisted metadata keys
// (ssh_host / ssh_user / ssh_remote_agentctl_port / etc.) are honored by
// ResumeRemoteInstance below.
func (r *SSHExecutor) RecoverInstances(_ context.Context) ([]*ExecutorInstance, error) {
	return nil, nil
}

// ResumeRemoteInstance is called by the lifecycle manager when re-attaching to
// an existing SSH session (e.g. after backend restart). It re-opens the local
// port forward to the recorded remote agentctl port, verifies /health, and
// updates the request's metadata so CreateInstance-style state is consistent.
//
// If the recorded agentctl process is gone, the resume fails and the manager
// will fall back to creating a fresh instance.
func (r *SSHExecutor) ResumeRemoteInstance(ctx context.Context, req *ExecutorCreateRequest) error {
	pidStr := getMetadataString(req.Metadata, MetadataKeySSHRemoteAgentctlPID)
	portStr := getMetadataString(req.Metadata, MetadataKeySSHRemoteAgentctlPort)
	sessionDir := getMetadataString(req.Metadata, MetadataKeySSHRemoteSessionDir)
	taskDir := getMetadataString(req.Metadata, MetadataKeySSHRemoteTaskDir)
	if pidStr == "" || portStr == "" || sessionDir == "" || taskDir == "" {
		return nil // not a resume — proceed with normal create
	}
	if err := requireSSHAgentctlAuthToken(req.AuthToken); err != nil {
		return err
	}

	// Idempotency: if a previous resume attempt for this InstanceID already
	// produced live client+forwarder state, return success without dialing
	// again. Without this, a retry orphans the existing client+forwarder
	// (the new ones overwrite the map entry and the old ones leak until
	// the GC reclaims them, well after the SSH connection has timed out).
	// CreateInstance has the same guard; mirror it here so the two entry
	// points to r.sessions[InstanceID] agree.
	r.mu.Lock()
	if _, ok := r.sessions[req.InstanceID]; ok {
		r.mu.Unlock()
		return nil
	}
	r.mu.Unlock()

	target, err := r.targetFromMetadata(req.Metadata)
	if err != nil {
		return err
	}
	client, err := dialSSH(ctx, target)
	if err != nil {
		return fmt.Errorf("ssh resume: connect: %w", err)
	}

	// SSH liveness is judged on the REMOTE host: MetadataKeySSHRemoteAgentctlPID
	// (mirrored into executors_running.pid) is a pid on the remote box, so it is
	// probed with a remote `kill -0` over this SSH client — never with a local
	// os.FindProcess. The runtime-aware row predicate (RowProcessLiveness in
	// liveness.go) deliberately returns Unknown for SSH rows so nothing applies a
	// host-local check here (#1597 runtime-aware liveness).
	pid, _ := strconv.Atoi(pidStr)
	if !isRemoteAgentctlAlive(ctx, client, pid) {
		_ = client.Close()
		return fmt.Errorf("ssh resume: agentctl pid %d not alive on remote", pid)
	}

	remotePort, _ := strconv.Atoi(portStr)
	fwd, err := StartPortForward(client, remotePort, r.logger)
	if err != nil {
		_ = client.Close()
		return fmt.Errorf("ssh resume: port forward: %w", err)
	}
	if err := waitAgentctlHealthy(ctx, fwd.LocalPort(), sshAgentctlHealthTimeout); err != nil {
		_ = fwd.Close()
		_ = client.Close()
		return fmt.Errorf("ssh resume: agentctl health: %w", err)
	}
	instanceClient, reusingProcess := r.newResumedAgentctlClient(
		ctx, fwd.LocalPort(), req,
	)

	r.mu.Lock()
	r.sessions[req.InstanceID] = &sshSessionState{
		target:         target,
		client:         client,
		forwarder:      fwd,
		agentctlClient: instanceClient,
		pid:            pid,
		remoteDir:      sessionDir,
		authToken:      req.AuthToken,
		reusingProcess: reusingProcess,
	}
	r.mu.Unlock()

	// Refresh transient metadata so downstream consumers see the new local
	// forward port (it changes on every recovery — only the remote port is
	// stable).
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata[MetadataKeySSHLocalForwardPort] = strconv.Itoa(fwd.LocalPort())
	return nil
}

func (r *SSHExecutor) newResumedAgentctlClient(
	ctx context.Context,
	localPort int,
	req *ExecutorCreateRequest,
) (*agentctl.Client, bool) {
	client := agentctl.NewClient(sshAgentctlLoopbackHost, localPort, r.logger,
		agentctl.WithExecutionID(resumedSSHAgentctlInstanceID(req)),
		agentctl.WithSessionID(req.SessionID), agentctl.WithAuthToken(req.AuthToken))
	status, err := client.GetStatus(ctx)
	if err != nil {
		r.logger.Debug("ssh resume could not get agent status; starting a new process",
			zap.String("instance_id", req.InstanceID), zap.Error(err))
		return client, false
	}
	return client, status != nil && status.IsAgentRunning()
}

func resumedSSHAgentctlInstanceID(req *ExecutorCreateRequest) string {
	if req.PreviousExecutionID != "" {
		return req.PreviousExecutionID
	}
	return req.InstanceID
}

func requireSSHAgentctlAuthToken(token string) error {
	if strings.TrimSpace(token) == "" {
		return errors.New("ssh resume: missing agentctl auth token")
	}
	return nil
}

// GetRemoteStatus reports the SSH host's current reachability for the UI
// status badge. Best-effort and bounded by a short timeout.
func (r *SSHExecutor) GetRemoteStatus(ctx context.Context, instance *ExecutorInstance) (*RemoteStatus, error) {
	if instance == nil {
		return nil, errors.New("instance is nil")
	}
	r.mu.Lock()
	state := r.sessions[instance.InstanceID]
	r.mu.Unlock()
	now := time.Now()
	status := &RemoteStatus{
		RuntimeName:   r.Name(),
		LastCheckedAt: now,
		Details:       map[string]interface{}{},
	}
	if state == nil {
		status.State = sshStatusUnknown
		status.ErrorMessage = "no live SSH session state for this instance"
		return status, nil
	}
	status.RemoteName = state.target.Host
	status.Details["pid"] = state.pid
	status.Details["host"] = state.target.Host
	status.Details["user"] = state.target.User
	status.Details["fingerprint"] = state.target.PinnedFingerprint

	probeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if state.client == nil {
		status.State = sshStatusDisconnected
		return status, nil
	}
	if !isRemoteAgentctlAlive(probeCtx, state.client, state.pid) {
		status.State = sshStatusAgentctlDown
		return status, nil
	}
	status.State = sshStatusRunning
	return status, nil
}

func (r *SSHExecutor) GetInteractiveRunner() *process.InteractiveRunner { return nil }

func (r *SSHExecutor) RequiresCloneURL() bool          { return true }
func (r *SSHExecutor) ShouldApplyPreferredShell() bool { return false }
func (r *SSHExecutor) IsAlwaysResumable() bool         { return true }

// maybeUploadCredentials runs the standard remote-credentials pipeline used by
// Sprites. Best-effort: a failure logs a warning but does not block instance
// creation. SSH-specific tweaks: remote auth target dir defaults to the SSH
// user's $HOME (resolved at runtime via a tiny `printf %s "$HOME"` probe),
// and the file uploader writes via SFTP.
//
// Credential upload only fires when the executor profile selected at least
// one remote-credentials method (or remote_auth_secrets / setup-script env
// vars). A failure here doesn't abort the launch — credentials are
// best-effort and missing ones surface later as the agent's own auth error.
func (r *SSHExecutor) maybeUploadCredentials(
	ctx context.Context,
	client *ssh.Client,
	req *ExecutorCreateRequest,
	platform SSHRemotePlatform,
) {
	if err := r.uploadCredentials(ctx, client, req, platform); err != nil {
		r.logger.Warn(
			"ssh executor: credential upload failed; launch will proceed but agent may not authenticate",
			zap.String("task_id", req.TaskID),
			zap.String("session_id", req.SessionID),
			zap.Error(err),
		)
	}
}

// preflightAgentBinary probes the remote for the agent's required binary
// before we provision the per-session agentctl. Without this, a remote that
// lacks (say) `npx` surfaces the failure deep inside agentctl as an opaque
// `exec: "npx": executable file not found in $PATH` once the agent process
// actually tries to spawn — long after a successful "Starting agent
// controller" step, which reads as "everything is fine".
//
// We extract the binary by calling BuildCommand with Runtime=ssh — that
// returns the exact command the lifecycle will eventually ship to agentctl.
// The first token is the binary; we shell out to POSIX `command -v` on the
// remote and treat empty stdout as "missing". On miss we surface the
// agent's name + InstallScript() so the error is actionable in the UI.
func (r *SSHExecutor) preflightAgentBinary(
	ctx context.Context,
	client *ssh.Client,
	req *ExecutorCreateRequest,
	platform SSHRemotePlatform,
) error {
	if req.AgentConfig == nil {
		return nil
	}
	stepName := "Verifying agent binary"
	shell := sshShellForRemote(req.Metadata, platform)

	// Prefer the agent's standalone CLI when present on the remote: running it
	// directly skips the per-launch `npx` registry round-trip. A hit is recorded
	// in metadata so the command builder emits the native binary; a miss (or a
	// transport hiccup during this optional probe) falls through to verifying
	// the default (npx) command below.
	if r.probeNativeBinary(ctx, client, shell, req, stepName) {
		return nil
	}

	cmd := req.AgentConfig.BuildCommand(agents.CommandOptions{Runtime: agentruntime.RuntimeSSH})
	args := cmd.Args()
	if len(args) == 0 {
		return nil
	}
	binary := args[0]
	out, err := ProbeRemoteBinary(ctx, client, shell, binary)
	if err != nil {
		r.report(req.OnProgress, stepName, PrepareStepFailed, err.Error())
		return fmt.Errorf("ssh: probe %s on remote: %w", binary, err)
	}
	if out == "" {
		msg := formatMissingAgentBinaryError(req.AgentConfig, binary)
		r.report(req.OnProgress, stepName, PrepareStepFailed, msg)
		return errors.New(msg)
	}
	r.report(req.OnProgress, stepName, PrepareStepCompleted, out)
	return nil
}

// probeNativeBinary probes the remote for the agent's standalone CLI (if it
// declares one via NativeBinaryAgent). On a hit it records the binary name in
// req.Metadata under MetadataKeyNativeBinary and returns true. It returns false
// when the agent has no native binary, the binary is absent, or the probe hits
// a transport error — the native binary is an optimisation, so a glitch here
// must not abort the launch or surface a misleading "agent binary" failure.
// The caller falls through to the required default-command probe, which reports
// transport failures with proper context.
func (r *SSHExecutor) probeNativeBinary(ctx context.Context, client *ssh.Client, shell string, req *ExecutorCreateRequest, stepName string) bool {
	nb, ok := req.AgentConfig.(agents.NativeBinaryAgent)
	if !ok {
		return false
	}
	name := nb.NativeBinaryName()
	if name == "" {
		return false
	}
	// Clear any stale/caller-provided value up front so only a positive probe
	// below sets it; a miss or transport error must not leave the command
	// builder preferring the native binary on an npx-only host. delete on a nil
	// map is a no-op.
	delete(req.Metadata, MetadataKeyNativeBinary)
	out, err := ProbeRemoteBinary(ctx, client, shell, name)
	if err != nil {
		r.logger.Warn("native binary probe failed on remote, falling back to default command",
			zap.String("binary", name), zap.Error(err))
		return false
	}
	if out == "" {
		return false
	}
	if req.Metadata == nil {
		req.Metadata = make(map[string]interface{})
	}
	req.Metadata[MetadataKeyNativeBinary] = name
	r.report(req.OnProgress, stepName, PrepareStepCompleted, out)
	return true
}

// sshShellFromMetadata reads the user-selected login shell from the
// per-profile metadata. Empty / unset returns "" so callers can fall
// back to WrapLoginShell's defaultLoginShell — keeping the default in
// one place.
func sshShellFromMetadata(md map[string]interface{}) string {
	return strings.TrimSpace(getMetadataString(md, MetadataKeySSHShell))
}

func sshShellForRemote(md map[string]interface{}, platform SSHRemotePlatform) string {
	if shell := sshShellFromMetadata(md); shell != "" {
		return shell
	}
	return SSHDefaultShellForPlatform(platform)
}

// agentIdentity is the slice of agents.Agent that formatMissingAgentBinaryError
// reads. Narrowing the parameter keeps the helper trivially testable without a
// dependency on the full Agent interface or a stub for every method on it.
type agentIdentity interface {
	ID() string
	Name() string
	InstallScript() string
}

// formatMissingAgentBinaryError builds the user-facing message we surface
// when the agent's first command-line token isn't on the remote's $PATH.
// Pulled out so unit tests can pin the contract without a real SSH client.
func formatMissingAgentBinaryError(ag agentIdentity, binary string) string {
	name := ag.Name()
	if name == "" {
		name = ag.ID()
	}
	msg := fmt.Sprintf("%s requires %q on the remote host, but it was not found in $PATH", name, binary)
	if install := strings.TrimSpace(ag.InstallScript()); install != "" {
		msg += "\n\nInstall hint (run on the remote):\n  " + install
	}
	return msg
}

// sshTaskDirName builds a stable per-task remote directory name. Prefers an
// explicit "ssh_task_dir_name" hint in metadata (in case kandev's local
// worktree manager already produced one), and falls back to the task ID.
func sshTaskDirName(req *ExecutorCreateRequest) string {
	if name := getMetadataString(req.Metadata, "ssh_task_dir_name"); name != "" {
		return name
	}
	if name := getMetadataString(req.Metadata, "task_dir_name"); name != "" {
		return name
	}
	return "task-" + req.TaskID
}

// report emits a single PrepareStep through the OnProgress callback, if any.
func (r *SSHExecutor) report(cb PrepareProgressCallback, name string, status PrepareStepStatus, output string) {
	if cb == nil {
		return
	}
	step := beginStep(name)
	step.Status = status
	step.Output = output
	cb(step, 0, 0)
}
