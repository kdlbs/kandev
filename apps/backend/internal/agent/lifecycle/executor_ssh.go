package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agentctl/client"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/secrets"
)

const (
	sshAgentctlHealthTimeout = 20 * time.Second

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
	target    *SSHTarget
	client    *ssh.Client
	forwarder *SSHPortForwarder
	pid       int
	remoteDir string
}

// SSHExecutor implements ExecutorBackend for SSH-reachable Linux hosts.
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

// Close is a no-op: per-session SSH clients are owned by sshSessionState and
// torn down by StopInstance. Kept on the type so the executor satisfies
// io.Closer alongside the other backends.
func (r *SSHExecutor) Close() error { return nil }

// targetFromMetadata builds an SSHTarget from the executor metadata
// propagated via buildLaunchMetadata (req.ExecutorConfig keys merged in).
func (r *SSHExecutor) targetFromMetadata(md map[string]interface{}) (*SSHTarget, error) {
	host := getMetadataString(md, MetadataKeySSHHost)
	hostAlias := getMetadataString(md, "ssh_host_alias")
	if host == "" && hostAlias == "" {
		return nil, errors.New("ssh executor: host (or host_alias) is required in executor config")
	}
	port := 0
	if p := getMetadataString(md, MetadataKeySSHPort); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
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
		return nil, errors.New("ssh executor: host_fingerprint is required — re-run Test Connection in settings to trust the host")
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
func (r *SSHExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
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

	agentctlBin, err := r.prepareRemoteHost(ctx, client, req)
	if err != nil {
		return nil, err
	}

	workdir := r.workdirRoot(req.Metadata)
	taskDir, sessionDir, err := r.prepareRemoteDirs(ctx, client, workdir, req)
	if err != nil {
		return nil, err
	}
	r.maybeUploadCredentials(ctx, client, req)

	port, pid, fwd, err := r.startAndForwardAgentctl(ctx, client, agentctlBin, taskDir, sessionDir, req)
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
	}
	r.mu.Unlock()
	released = true // ownership transferred to session state; released on StopInstance

	return r.buildInstance(req, target, fwd, taskDir, sessionDir, port, pid, workdir), nil
}

// prepareRemoteHost runs the steps that are independent of any particular
// task: detect remote OS/arch and ensure the agentctl binary is on the host.
// Returns the resolved remote agentctl path.
func (r *SSHExecutor) prepareRemoteHost(ctx context.Context, client *ssh.Client, req *ExecutorCreateRequest) (string, error) {
	info, err := detectRemoteInfo(ctx, client)
	if err != nil {
		r.report(req.OnProgress, "Detecting remote OS", PrepareStepFailed, err.Error())
		return "", fmt.Errorf("ssh: detect remote: %w", err)
	}
	if err := requireSupportedArch(info.Arch); err != nil {
		r.report(req.OnProgress, "Detecting remote OS", PrepareStepFailed, err.Error())
		return "", err
	}
	r.report(req.OnProgress, "Detecting remote OS", PrepareStepCompleted, info.UnameAll)

	agentctlBin, err := ensureAgentctlOnHost(ctx, client, r.agentctlResolver, r.logger)
	if err != nil {
		r.report(req.OnProgress, "Uploading agent controller", PrepareStepFailed, err.Error())
		return "", err
	}
	r.report(req.OnProgress, "Uploading agent controller", PrepareStepCompleted, "")
	return agentctlBin, nil
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
	ctx context.Context, client *ssh.Client, agentctlBin, taskDir, sessionDir string, req *ExecutorCreateRequest,
) (int, int, *SSHPortForwarder, error) {
	controlPort, pid, err := startRemoteAgentctl(ctx, client, agentctlBin, taskDir, sessionDir, r.logger)
	if err != nil {
		r.report(req.OnProgress, "Starting agent controller", PrepareStepFailed, err.Error())
		return 0, 0, nil, err
	}
	r.report(req.OnProgress, "Starting agent controller", PrepareStepCompleted,
		fmt.Sprintf("pid=%d control_port=%d", pid, controlPort))

	// Override req.WorkspacePath to the remote task dir so the per-instance
	// server's workspace points at the SSH workspace, not the host one.
	origWorkspace := req.WorkspacePath
	req.WorkspacePath = taskDir
	instancePort, ierr := createRemoteAgentInstance(ctx, client, controlPort, req, r.logger)
	req.WorkspacePath = origWorkspace
	if ierr != nil {
		_ = stopRemoteAgentctl(ctx, client, sessionDir, pid)
		r.report(req.OnProgress, "Creating agent instance", PrepareStepFailed, ierr.Error())
		return 0, 0, nil, ierr
	}
	r.report(req.OnProgress, "Creating agent instance", PrepareStepCompleted,
		fmt.Sprintf("instance_port=%d", instancePort))

	fwd, err := StartPortForward(client, instancePort, r.logger)
	if err != nil {
		_ = stopRemoteAgentctl(ctx, client, sessionDir, pid)
		return 0, 0, nil, fmt.Errorf("ssh: port forward: %w", err)
	}
	if err := waitAgentctlHealthy(ctx, fwd.LocalPort(), sshAgentctlHealthTimeout); err != nil {
		_ = fwd.Close()
		_ = stopRemoteAgentctl(ctx, client, sessionDir, pid)
		return 0, 0, nil, fmt.Errorf("ssh: agentctl health: %w", err)
	}
	r.report(req.OnProgress, "Connecting to agent controller", PrepareStepCompleted,
		fmt.Sprintf("local:%d -> remote:%d", fwd.LocalPort(), instancePort))
	return instancePort, pid, fwd, nil
}

func (r *SSHExecutor) buildInstance(
	req *ExecutorCreateRequest,
	target *SSHTarget,
	fwd *SSHPortForwarder,
	taskDir, sessionDir string,
	port, pid int,
	workdir string,
) *ExecutorInstance {
	return &ExecutorInstance{
		InstanceID:  req.InstanceID,
		TaskID:      req.TaskID,
		SessionID:   req.SessionID,
		RuntimeName: string(r.Name()),
		Client: agentctl.NewClient("127.0.0.1", fwd.LocalPort(), r.logger,
			agentctl.WithExecutionID(req.InstanceID),
			agentctl.WithSessionID(req.SessionID)),
		WorkspacePath: taskDir,
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

// StopInstance kills the per-session agentctl on the remote, closes the local
// port forward, and releases this session's hold on the pooled SSH connection.
// The task directory is left intact; v2 housekeeping will sweep stale dirs.
func (r *SSHExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, _ bool) error {
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
	// Use the same SSH client we used for CreateInstance — if it's still
	// alive we can kill the remote agentctl gracefully; otherwise just drop
	// the connection on the floor.
	if state.client != nil {
		_ = stopRemoteAgentctl(ctx, state.client, state.remoteDir, state.pid)
		_ = state.client.Close()
	}
	return nil
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

	target, err := r.targetFromMetadata(req.Metadata)
	if err != nil {
		return err
	}
	client, err := dialSSH(ctx, target)
	if err != nil {
		return fmt.Errorf("ssh resume: connect: %w", err)
	}

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

	r.mu.Lock()
	r.sessions[req.InstanceID] = &sshSessionState{
		target:    target,
		client:    client,
		forwarder: fwd,
		pid:       pid,
		remoteDir: sessionDir,
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
		RuntimeName:   string(r.Name()),
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
// user's home (resolved at runtime via a tiny `pwd` probe), and the file
// uploader writes via SFTP.
//
// v1 ships without remote-credentials upload — the field is accepted on the
// executor profile (parity with Sprites/Docker) but the upload pipeline lives
// in the Sprites executor and is not yet ported over. A non-empty
// remote_credentials log is the only side effect so users notice they
// configured a feature that isn't active yet.
func (r *SSHExecutor) maybeUploadCredentials(_ context.Context, _ *ssh.Client, req *ExecutorCreateRequest) {
	if getMetadataString(req.Metadata, "remote_credentials") == "" {
		return
	}
	r.logger.Warn(
		"ssh executor: remote_credentials is configured but credential upload is not yet implemented for the SSH runtime; see docs/specs/ssh-executor/spec.md",
		zap.String("task_id", req.TaskID),
		zap.String("session_id", req.SessionID),
	)
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
