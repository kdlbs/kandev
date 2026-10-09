package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"path"
	"strconv"
	"sync"
	"time"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	"github.com/kandev/kandev/internal/agent/docker"
	"github.com/kandev/kandev/internal/agent/executor"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// RemoteDockerExecutor runs a task's container on a Docker daemon reached over
// SSH.
//
// It is the Docker executor's container model on the SSH executor's transport:
// the daemon connection, the mount sources, and the published-port endpoints
// all resolve on the remote host, and nothing on the backend's filesystem is
// mounted into the container.
type RemoteDockerExecutor struct {
	logger           *logger.Logger
	agentctlResolver *AgentctlResolver

	// connect resolves a request to a live remote session. These operations
	// are fields so the lifecycle can be tested without a daemon or an SSH
	// host.
	connect func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error)
	// reconnect adopts a container the request already names, so a resume
	// reattaches to the preserved workspace instead of launching a new one.
	reconnect func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error)
	// launch provisions a fresh container.
	launch func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error)
	// watchTransport starts the keepalive watchdog for a live session.
	watchTransport func(string, *remoteDockerSession)

	mu       sync.Mutex
	sessions map[string]*remoteDockerSession
	// targets holds each launched instance's SSH target from launch until its
	// stop. A session can be dropped before then, when the keepalive watchdog
	// declares the transport lost, and teardown still has to reach the daemon.
	targets map[string]map[string]interface{}
}

// remoteDockerTargetKeys are the metadata keys remoteDockerTarget reads.
var remoteDockerTargetKeys = []string{
	MetadataKeySSHHost,
	MetadataKeySSHHostAlias,
	MetadataKeySSHPort,
	MetadataKeySSHUser,
	MetadataKeySSHIdentitySource,
	MetadataKeySSHIdentityFile,
	MetadataKeySSHProxyJump,
	MetadataKeySSHHostFingerprint,
}

// remoteDockerSession is one executor profile's live connection to a remote
// daemon, plus the per-session resources that ride it.
type remoteDockerSession struct {
	sshClient    *ssh.Client
	dockerClient *docker.Client
	containerMgr *ContainerManager
	endpoints    containerEndpointResolver
	// inputs delivers the container's agentctl helper, session directory, and
	// seeded credentials through the Engine API. Nothing reaches the remote
	// host's filesystem.
	inputs   *remoteContainerInputs
	platform SSHRemotePlatform

	watchdogMu sync.Mutex
	watchdog   *sshKeepaliveWatchdog
	closed     bool
}

func (s *remoteDockerSession) close() error {
	return s.closeWithWatchdogLoop(true, nil)
}

// closeFromWatchdogLoop is called by the watchdog's loss callback. The
// callback already owns the loop goroutine, so it closes the transport first
// and waits only for the probe goroutine; runLoop closes loopDone when the
// callback returns.
func (s *remoteDockerSession) closeFromWatchdogLoop(watchdog *sshKeepaliveWatchdog) error {
	return s.closeWithWatchdogLoop(false, watchdog)
}

func (s *remoteDockerSession) closeWithWatchdogLoop(
	awaitLoop bool, expectedWatchdog *sshKeepaliveWatchdog,
) error {
	var firstErr error
	watchdog := s.takeWatchdog(expectedWatchdog)
	if watchdog != nil && awaitLoop {
		watchdog.stopAndAwaitLoop()
	}
	if s.endpoints != nil {
		if err := s.endpoints.Close(); err != nil {
			firstErr = err
		}
	}
	// SSH closes before the Docker client. Each pooled Engine API connection
	// waits for its remote command to exit, and on a dead link that exit only
	// arrives once the SSH connection is gone.
	if s.sshClient != nil {
		if err := s.sshClient.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.dockerClient != nil {
		if err := s.dockerClient.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if watchdog != nil {
		watchdog.awaitProbeExit()
	}
	return firstErr
}

func (s *remoteDockerSession) setWatchdog(watchdog *sshKeepaliveWatchdog) bool {
	s.watchdogMu.Lock()
	defer s.watchdogMu.Unlock()
	if s.closed {
		return false
	}
	s.watchdog = watchdog
	return true
}

func (s *remoteDockerSession) takeWatchdog(expected *sshKeepaliveWatchdog) *sshKeepaliveWatchdog {
	s.watchdogMu.Lock()
	defer s.watchdogMu.Unlock()
	s.closed = true
	if expected != nil && s.watchdog != expected {
		return nil
	}
	watchdog := s.watchdog
	s.watchdog = nil
	return watchdog
}

// NewRemoteDockerExecutor creates the remote Docker runtime. Connections are
// established per launch, against the target named by the executor profile.
func NewRemoteDockerExecutor(log *logger.Logger, resolvers ...*AgentctlResolver) *RemoteDockerExecutor {
	resolver := NewAgentctlResolver(log)
	if len(resolvers) > 0 && resolvers[0] != nil {
		resolver = resolvers[0]
	}
	r := &RemoteDockerExecutor{
		logger:           log.WithFields(zap.String("runtime", "remote_docker")),
		agentctlResolver: resolver,
		sessions:         map[string]*remoteDockerSession{},
		targets:          map[string]map[string]interface{}{},
	}
	r.connect = r.dialRemote
	r.reconnect = r.reconnectToContainer
	r.launch = r.launchFresh
	r.watchTransport = r.startTransportWatchdog
	return r
}

func (r *RemoteDockerExecutor) newRemoteContainerInputs(
	client containerArchiveWriter,
	platform SSHRemotePlatform,
	commandBuilder *CommandBuilder,
) *remoteContainerInputs {
	return newRemoteContainerInputs(client, platform, commandBuilder, r.logger, r.agentctlResolver)
}

func (r *RemoteDockerExecutor) Name() executor.Name {
	return executor.NameRemoteDocker
}

// HealthCheck reports on the runtime as a capability, not on any particular
// daemon: the daemon lives on a host named by an executor profile, and each
// profile is verified by its own connection test and at launch.
func (r *RemoteDockerExecutor) HealthCheck(_ context.Context) error {
	return nil
}

// remoteDockerTarget resolves the SSH target for a launch.
//
// The profile stores an SSH target, never a Docker host URL: rejecting every
// scheme is what keeps an unsecured tcp:// daemon port out of this path.
func remoteDockerTarget(md map[string]interface{}) (*SSHTarget, error) {
	host := getMetadataString(md, MetadataKeySSHHost)
	hostAlias := getMetadataString(md, MetadataKeySSHHostAlias)
	if host == "" && hostAlias == "" {
		return nil, errors.New("remote docker: host (or host_alias) is required in the executor profile")
	}
	for _, candidate := range []string{host, hostAlias} {
		if candidate == "" {
			continue
		}
		if err := docker.ValidateRemoteDaemonAddress(candidate); err != nil {
			return nil, fmt.Errorf("remote docker: %w", err)
		}
	}

	port := 0
	if p := getMetadataString(md, MetadataKeySSHPort); p != "" {
		n, err := strconv.Atoi(p)
		if err != nil || n < 1 || n > 65535 {
			return nil, fmt.Errorf("remote docker: invalid ssh_port %q (must be 1-65535)", p)
		}
		port = n
	}

	cfg := SSHConnConfig{
		HostAlias:         hostAlias,
		Host:              host,
		Port:              port,
		User:              getMetadataString(md, MetadataKeySSHUser),
		IdentitySource:    SSHIdentitySource(getMetadataString(md, MetadataKeySSHIdentitySource)),
		IdentityFile:      getMetadataString(md, MetadataKeySSHIdentityFile),
		ProxyJump:         getMetadataString(md, MetadataKeySSHProxyJump),
		PinnedFingerprint: getMetadataString(md, MetadataKeySSHHostFingerprint),
	}
	if cfg.PinnedFingerprint == "" {
		return nil, errors.New("remote docker: host_fingerprint is required — open the executor connection settings, run Test connection, and trust the host")
	}
	return ResolveSSHTarget(cfg)
}

// dialRemote establishes the SSH connection, probes the remote platform, and
// assembles a Docker client and container manager that resolve everything on
// the remote host.
func (r *RemoteDockerExecutor) dialRemote(ctx context.Context, req *ExecutorCreateRequest) (*remoteDockerSession, error) {
	target, err := remoteDockerTarget(req.Metadata)
	if err != nil {
		return nil, err
	}

	sshClient, err := DialSSH(ctx, target)
	if err != nil {
		return nil, fmt.Errorf("remote docker: connect to %s: %w", target.Host, err)
	}

	info, err := SSHProbeRemote(ctx, sshClient)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("remote docker: probe %s: %w", target.Host, err)
	}
	if err := SSHRequireSupportedRemotePlatform(info.Platform); err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("remote docker: %w", err)
	}

	dockerClient, err := docker.NewRemoteClient(NewSSHDockerDialer(sshClient, r.logger), r.logger)
	if err != nil {
		_ = sshClient.Close()
		return nil, fmt.Errorf("remote docker: create client for %s: %w", target.Host, err)
	}

	// Ping through the transport so a denied socket or a missing CLI is
	// reported here, with its own cause, rather than as an opaque failure
	// part-way through provisioning a container.
	if err := dockerClient.Ping(ctx); err != nil {
		_ = dockerClient.Close()
		_ = sshClient.Close()
		return nil, fmt.Errorf("remote docker: %w", err)
	}

	session := &remoteDockerSession{
		sshClient:    sshClient,
		dockerClient: dockerClient,
		platform:     info.Platform,
	}
	session.endpoints = newRemoteEndpointResolver(
		dockerPublishedPorts{client: dockerClient},
		sshPortForwarder{client: sshClient, logger: r.logger},
	)

	mgr := NewContainerManager(dockerClient, "", r.logger)
	mgr.containerHostFiles = newRemoteContainerHostFiles(info.Platform)
	mgr.endpointResolver = session.endpoints

	session.inputs = r.newRemoteContainerInputs(dockerClient, info.Platform, mgr.commandBuilder)
	session.inputs.resolveMockAgentBinary = mgr.resolveMockAgentBinary
	mgr.seedCreatedContainer = session.inputs.DeliverLaunchInputs
	session.containerMgr = mgr

	return session, nil
}

// sshPortForwarder adapts the SSH executor's forwarder to the endpoint
// resolver's narrower need.
type sshPortForwarder struct {
	client *ssh.Client
	logger *logger.Logger
}

func (s sshPortForwarder) Forward(remotePort int) (int, func() error, error) {
	fwd, err := StartPortForward(s.client, remotePort, s.logger)
	if err != nil {
		return 0, nil, err
	}
	return fwd.LocalPort(), fwd.Close, nil
}

func (r *RemoteDockerExecutor) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	session, err := r.connect(ctx, req)
	if err != nil {
		return nil, err
	}

	// A launch that reuses a live instance ID finds its session here.
	// Overwriting it would strand that session's SSH client, Docker client,
	// forwards, and watchdog.
	r.mu.Lock()
	replaced := r.sessions[req.InstanceID]
	r.sessions[req.InstanceID] = session
	r.mu.Unlock()
	if replaced != nil && replaced != session {
		if err := replaced.close(); err != nil {
			r.logger.Warn("failed to close the replaced remote docker session",
				zap.String("instance_id", req.InstanceID), zap.Error(err))
		}
	}

	r.watchTransport(req.InstanceID, session)

	// A resume names the container it left behind. Reattaching keeps the
	// workspace the user expects; launching a second container would abandon
	// it on the remote host.
	instance, reconnectErr := r.reconnect(ctx, session, req)
	if reconnectErr != nil {
		r.releaseSession(req.InstanceID)
		if req.WorkspaceReuseRequired {
			return nil, fmt.Errorf("%w: existing remote Docker workspace could not be attached: %w",
				models.ErrWorkspaceReuseUnsafe, reconnectErr)
		}
		return nil, reconnectErr
	}
	if instance != nil {
		r.rememberTarget(req)
		return instance, nil
	}
	if req.WorkspaceReuseRequired {
		r.releaseSession(req.InstanceID)
		return nil, fmt.Errorf("%w: existing remote Docker workspace could not be attached",
			models.ErrWorkspaceReuseUnsafe)
	}

	instance, err = r.launch(ctx, session, req)
	if err != nil {
		r.releaseSession(req.InstanceID)
		return nil, err
	}
	r.rememberTarget(req)
	return instance, nil
}

func (r *RemoteDockerExecutor) rememberTarget(req *ExecutorCreateRequest) {
	target := make(map[string]interface{}, len(remoteDockerTargetKeys))
	for _, key := range remoteDockerTargetKeys {
		if value, ok := req.Metadata[key]; ok {
			target[key] = value
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.targets[req.InstanceID] = target
}

func (r *RemoteDockerExecutor) target(instanceID string) map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.targets[instanceID]
}

func (r *RemoteDockerExecutor) forgetTarget(instanceID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.targets, instanceID)
}

// launchFresh provisions a new container for the request.
func (r *RemoteDockerExecutor) launchFresh(
	ctx context.Context, session *remoteDockerSession, req *ExecutorCreateRequest,
) (*ExecutorInstance, error) {
	return launchDockerContainer(ctx, dockerLaunchTarget{
		dockerClient: session.dockerClient,
		containerMgr: session.containerMgr,
		runtimeName:  r.Name(),
		executorType: string(models.ExecutorTypeRemoteDocker),
		logger:       r.logger,
	}, req)
}

// reconnectToContainer adopts the container the request names, when it is
// still running on the remote daemon.
//
// This delegates to the Docker executor's reconnect rather than reimplementing
// it: that path re-establishes the agentctl control client, re-runs the
// bootstrap handshake when the token is stale, and finds the existing agent
// instance. A simplified version returned an instance with no client, which
// launched fine and then failed on the first prompt.
//
// The delegate carries the session's forwarding endpoint resolver, so the
// endpoints it reports are backend loopback rather than the remote host's.
func (r *RemoteDockerExecutor) reconnectToContainer(
	ctx context.Context, session *remoteDockerSession, req *ExecutorCreateRequest,
) (*ExecutorInstance, error) {
	if getMetadataString(req.Metadata, MetadataKeyContainerID) == "" && req.PreviousExecutionID == "" {
		return nil, nil
	}

	delegate := r.reconnectDelegate(session)
	instance, err := delegate.reconnectToContainer(ctx, session.dockerClient, req)
	if err != nil {
		if errors.Is(err, errContainerEndpointResolution) {
			return nil, err
		}
		r.logger.Info("remote docker: could not reconnect, launching fresh",
			zap.String("instance_id", req.InstanceID), zap.Error(err))
		return nil, nil
	}
	instance.RuntimeName = r.Name()
	return instance, nil
}

// reconnectDelegate builds the Docker executor that adopts a preserved
// container.
//
// It carries the session's forwarding endpoint resolver, so the endpoints it
// reports are backend loopback rather than the remote host's, and the session's
// helper redelivery, so a container preserved across a backend upgrade does not
// resume on the agentctl it was created with.
func (r *RemoteDockerExecutor) reconnectDelegate(session *remoteDockerSession) *DockerExecutor {
	delegate := &DockerExecutor{
		logger:          r.logger,
		endpoints:       session.endpoints,
		brokerPreflight: runBrokerReachabilityViaAgentctl,
	}
	if session.inputs != nil {
		delegate.beforeContainerStart = session.inputs.DeliverHelpers
	}
	return delegate
}

// startTransportWatchdog surfaces a dropped SSH connection as a failure
// instead of a session that looks healthy but answers nothing.
func (r *RemoteDockerExecutor) startTransportWatchdog(instanceID string, session *remoteDockerSession) {
	if session == nil || session.sshClient == nil {
		return
	}
	interval, deadline := sshKeepaliveInterval, sshKeepaliveDeadline
	if !sshKeepaliveTuningValid(interval, deadline) {
		return
	}
	watchdogReady := make(chan *sshKeepaliveWatchdog, 1)
	watchdog := startSSHKeepaliveWatchdog(session.sshClient, interval, deadline, time.Now(), nil,
		func(reason string, silence time.Duration) {
			watchdog := <-watchdogReady
			r.logger.Warn("remote docker: transport lost",
				zap.String("instance_id", instanceID),
				zap.String("reason", reason),
				zap.Duration("silence", silence))
			r.releaseSessionFromWatchdog(instanceID, session, watchdog)
		},
	)
	attached := session.setWatchdog(watchdog)
	watchdogReady <- watchdog
	if !attached {
		watchdog.stopAndAwaitLoop()
		watchdog.awaitProbeExit()
	}
}

func (r *RemoteDockerExecutor) releaseSession(instanceID string) {
	r.releaseSessionIfCurrent(instanceID, nil)
}

func (r *RemoteDockerExecutor) releaseSessionIfCurrent(instanceID string, expected *remoteDockerSession) {
	session := r.takeSession(instanceID, expected)
	if session != nil {
		if err := session.close(); err != nil {
			r.logger.Warn("failed to release remote docker session",
				zap.String("instance_id", instanceID), zap.Error(err))
		}
	}
}

func (r *RemoteDockerExecutor) releaseSessionFromWatchdog(
	instanceID string, expected *remoteDockerSession, watchdog *sshKeepaliveWatchdog,
) {
	session := r.takeSession(instanceID, expected)
	if session != nil {
		if err := session.closeFromWatchdogLoop(watchdog); err != nil {
			r.logger.Warn("failed to release lost remote docker session",
				zap.String("instance_id", instanceID), zap.Error(err))
		}
	}
}

func (r *RemoteDockerExecutor) takeSession(instanceID string, expected *remoteDockerSession) *remoteDockerSession {
	r.mu.Lock()
	defer r.mu.Unlock()
	session := r.sessions[instanceID]
	if expected != nil && session != expected {
		return nil
	}
	delete(r.sessions, instanceID)
	return session
}

// remoteKandevHomeDir is the Kandev root on the remote host.
//
// A launch writes nothing there: a remote container's inputs are delivered
// through the Docker Engine API. It survives because a container provisioned
// before that change bind-mounts a per-instance directory beneath it, holding
// that agent's credential files, and the container's removal does not take a
// bind-mount source with it.
const remoteKandevHomeDir = "~/.kandev"

// remoteSessionDirRemovalTimeout bounds the removal so a wedged host delays a
// teardown rather than blocking it.
const remoteSessionDirRemovalTimeout = 30 * time.Second

// removeLegacySessionDir removes the per-instance agent session directory a
// pre-container-delivery launch created on the remote host.
//
// It runs on the same stop reasons the local Docker executor removes its own
// session directory for. The path is composed here and shell-quoted: this is a
// recursive delete on a machine Kandev does not own, so it must never be steered
// by stored metadata. A failure is reported and does not fail the stop, because
// stop runs inside archive and delete.
func (r *RemoteDockerExecutor) removeLegacySessionDir(ctx context.Context, instance *ExecutorInstance) {
	if !shouldRunExecutorCleanup(instance.StopReason) || instance.InstanceID == "" {
		return
	}

	r.mu.Lock()
	session := r.sessions[instance.InstanceID]
	r.mu.Unlock()
	if session == nil || session.sshClient == nil {
		return
	}

	cleanupCtx, cancel := context.WithTimeout(ctx, remoteSessionDirRemovalTimeout)
	defer cancel()

	root, err := expandRemoteHome(cleanupCtx, session.sshClient, remoteKandevHomeDir)
	if err != nil {
		r.logger.Warn("remote docker: could not resolve the remote Kandev home to clean up",
			zap.String("instance_id", instance.InstanceID), zap.Error(err))
		return
	}
	dir := path.Join(root, "agent-sessions", instance.InstanceID)

	if _, stderr, err := runSSHCommand(cleanupCtx, session.sshClient, "rm -rf "+shellQuote(dir)); err != nil {
		r.logger.Warn("remote docker: failed to remove the remote agent session dir",
			zap.String("instance_id", instance.InstanceID),
			zap.String("path", dir),
			zap.String("stderr", stderr),
			zap.Error(err))
		return
	}
	r.logger.Info("remote docker: removed the remote agent session dir",
		zap.String("instance_id", instance.InstanceID),
		zap.String("path", dir))
}

func (r *RemoteDockerExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	if instance == nil {
		return nil
	}
	// Every stop releases the session once the stop policy has run. Each launch
	// uses a fresh instance ID, so nothing reaches this session afterward: a
	// resume dials its own, and archive or delete remove a preserved container
	// through a fresh connection from the persisted target.
	defer r.releaseSession(instance.InstanceID)
	defer r.forgetTarget(instance.InstanceID)
	teardown := force || instance.AgentStopFailed || shouldTeardownDockerContainer(instance.StopReason)

	r.removeLegacySessionDir(ctx, instance)

	if instance.ContainerID == "" {
		// Nothing was provisioned. Stop runs inside archive and delete, so
		// reporting an error here would fail a teardown that has no work.
		return nil
	}

	if !teardown {
		r.logger.Info("preserving remote docker container after agent stop",
			zap.String("container_id", instance.ContainerID),
			zap.String("instance_id", instance.InstanceID),
			zap.String("stop_reason", instance.StopReason))
		return nil
	}

	r.mu.Lock()
	session := r.sessions[instance.InstanceID]
	r.mu.Unlock()
	if session == nil {
		return r.stopOverNewConnection(ctx, instance, force)
	}

	return stopDockerContainer(ctx, session.dockerClient, session.containerMgr, instance, force, r.logger)
}

// stopOverNewConnection tears down a container whose session was already
// dropped, by connecting again to the target it was launched on.
func (r *RemoteDockerExecutor) stopOverNewConnection(ctx context.Context, instance *ExecutorInstance, force bool) error {
	target := r.target(instance.InstanceID)
	if target == nil {
		// Nothing says where the container is. It stays on the remote host;
		// say so rather than reporting success.
		return fmt.Errorf("remote docker: no live connection for instance %s; container %s was left running",
			instance.InstanceID, instance.ContainerID)
	}
	session, err := r.connect(ctx, &ExecutorCreateRequest{InstanceID: instance.InstanceID, Metadata: target})
	if err != nil {
		return fmt.Errorf("remote docker: reconnect to remove container %s: %w", instance.ContainerID, err)
	}
	defer func() {
		if closeErr := session.close(); closeErr != nil {
			r.logger.Warn("failed to close the teardown connection",
				zap.String("instance_id", instance.InstanceID), zap.Error(closeErr))
		}
	}()
	return stopDockerContainer(ctx, session.dockerClient, session.containerMgr, instance, force, r.logger)
}

// RecoverInstances does not adopt containers after a backend restart. The
// remote container survives, but its SSH connection and port forwards do not,
// and reconnecting is resume's job once a request names the target.
func (r *RemoteDockerExecutor) RecoverInstances(_ context.Context, _ []*models.ExecutorRunning) ([]*ExecutorInstance, error) {
	return nil, nil
}

// GetInteractiveRunner returns nil: passthrough mode runs a process on the
// backend host, which is the opposite of what this runtime is for.
func (r *RemoteDockerExecutor) GetInteractiveRunner() *process.InteractiveRunner {
	return nil
}

func (r *RemoteDockerExecutor) RequiresCloneURL() bool          { return true }
func (r *RemoteDockerExecutor) ShouldApplyPreferredShell() bool { return false }
func (r *RemoteDockerExecutor) IsAlwaysResumable() bool         { return false }

// Close releases every live remote session. Called during shutdown.
func (r *RemoteDockerExecutor) Close() error {
	r.mu.Lock()
	sessions := r.sessions
	r.sessions = map[string]*remoteDockerSession{}
	r.mu.Unlock()

	var firstErr error
	for _, session := range sessions {
		if err := session.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// executorNameRemoteDocker is a test-visible accessor for the runtime's name.
func executorNameRemoteDocker() executor.Name { return executor.NameRemoteDocker }
