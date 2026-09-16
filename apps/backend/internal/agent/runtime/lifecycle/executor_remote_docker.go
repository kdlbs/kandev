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
	logger *logger.Logger

	// connect resolves a request to a live remote session. These operations
	// are fields so the lifecycle can be tested without a daemon or an SSH
	// host.
	connect func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error)
	// reconnect adopts a container the request already names, so a resume
	// reattaches to the preserved workspace instead of launching a new one.
	reconnect func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, bool)
	// launch provisions a fresh container.
	launch func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error)
	// watchTransport starts the keepalive watchdog for a live session.
	watchTransport func(string, *remoteDockerSession)

	mu       sync.Mutex
	sessions map[string]*remoteDockerSession
}

// remoteDockerSession is one executor profile's live connection to a remote
// daemon, plus the per-session resources that ride it.
type remoteDockerSession struct {
	sshClient     *ssh.Client
	dockerClient  *docker.Client
	containerMgr  *ContainerManager
	endpoints     containerEndpointResolver
	hostFileStore *sshHostFileStore
	platform      SSHRemotePlatform
	watchdog      *sshKeepaliveWatchdog
}

func (s *remoteDockerSession) close() error {
	var firstErr error
	if s.watchdog != nil {
		s.watchdog.stopAndAwaitLoop()
		s.watchdog.awaitProbeExit()
		s.watchdog = nil
	}
	if s.endpoints != nil {
		if err := s.endpoints.Close(); err != nil {
			firstErr = err
		}
	}
	if s.dockerClient != nil {
		if err := s.dockerClient.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if s.sshClient != nil {
		if err := s.sshClient.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NewRemoteDockerExecutor creates the remote Docker runtime. Connections are
// established per launch, against the target named by the executor profile.
func NewRemoteDockerExecutor(log *logger.Logger) *RemoteDockerExecutor {
	r := &RemoteDockerExecutor{
		logger:   log.WithFields(zap.String("runtime", "remote_docker")),
		sessions: map[string]*remoteDockerSession{},
	}
	r.connect = r.dialRemote
	r.reconnect = r.reconnectToContainer
	r.launch = r.launchFresh
	r.watchTransport = r.startTransportWatchdog
	return r
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

	session.hostFileStore = newSSHHostFileStore(sshClient, NewAgentctlResolver(r.logger), r.logger)

	mgr := NewContainerManager(dockerClient, "", "", r.logger)
	remoteFiles := newRemoteContainerHostFiles(session.hostFileStore, info.Platform, mgr.commandBuilder)
	remoteFiles.resolveMockAgentBinary = mgr.resolveMockAgentBinary
	mgr.containerHostFiles = remoteFiles
	mgr.endpointResolver = session.endpoints
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

	r.mu.Lock()
	r.sessions[req.InstanceID] = session
	r.mu.Unlock()

	r.watchTransport(req.InstanceID, session)

	// A resume names the container it left behind. Reattaching keeps the
	// workspace the user expects; launching a second container would abandon
	// it on the remote host.
	if instance, ok := r.reconnect(ctx, session, req); ok {
		return instance, nil
	}

	if err := r.seedRemoteSessionDir(ctx, session, req); err != nil {
		r.releaseSession(req.InstanceID)
		return nil, err
	}

	instance, err := r.launch(ctx, session, req)
	if err != nil {
		r.releaseSession(req.InstanceID)
		return nil, err
	}
	return instance, nil
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
) (*ExecutorInstance, bool) {
	if getMetadataString(req.Metadata, MetadataKeyContainerID) == "" && req.PreviousExecutionID == "" {
		return nil, false
	}

	delegate := &DockerExecutor{logger: r.logger, endpoints: session.endpoints}
	instance, err := delegate.reconnectToContainer(ctx, session.dockerClient, req)
	if err != nil {
		r.logger.Info("remote docker: could not reconnect, launching fresh",
			zap.String("instance_id", req.InstanceID), zap.Error(err))
		return nil, false
	}
	instance.RuntimeName = r.Name()
	return instance, true
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
	session.watchdog = startSSHKeepaliveWatchdog(session.sshClient, interval, deadline, time.Now(), nil,
		func(reason string, silence time.Duration) {
			r.logger.Warn("remote docker: transport lost",
				zap.String("instance_id", instanceID),
				zap.String("reason", reason),
				zap.Duration("silence", silence))
			r.releaseSession(instanceID)
		},
	)
}

// seedRemoteSessionDir copies the agent's credentials and selected config
// bundles into the directory the container mounts. Without it the container
// mounts an empty directory and the agent starts with no login.
func (r *RemoteDockerExecutor) seedRemoteSessionDir(
	ctx context.Context, session *remoteDockerSession, req *ExecutorCreateRequest,
) error {
	if req.AgentConfig == nil || session.hostFileStore == nil {
		return nil
	}
	remoteDir, err := session.hostFileStore.EnsureSessionDir(req.InstanceID)
	if err != nil {
		return err
	}
	// A credential failure is reported but does not stop the launch, matching
	// the local Docker path: some agents authenticate from the environment or
	// their in-container setup script instead.
	if seedErr := seedRemoteAgentSessionDir(
		ctx,
		session.sshClient,
		req.AgentConfig,
		remoteDir,
		selectedPortableConfigBundleIDs(req.Metadata),
		r.logger,
		func(warnings []PortableConfigWarning) {
			reportPortableConfigWarnings(req.OnProgress, warnings)
		},
	); seedErr != nil {
		r.logger.Warn("remote docker: failed to seed agent session dir (continuing)",
			zap.String("instance_id", req.InstanceID),
			zap.Error(seedErr))
	}
	return nil
}

func (r *RemoteDockerExecutor) releaseSession(instanceID string) {
	r.mu.Lock()
	session := r.sessions[instanceID]
	delete(r.sessions, instanceID)
	r.mu.Unlock()
	if session != nil {
		if err := session.close(); err != nil {
			r.logger.Warn("failed to release remote docker session",
				zap.String("instance_id", instanceID), zap.Error(err))
		}
	}
}

func (r *RemoteDockerExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, force bool) error {
	if instance == nil {
		return nil
	}
	// An ordinary stop preserves the container, so it must also preserve the
	// connection that reaches it. Releasing here strands the container: the
	// later archive or delete would have no way to remove it.
	teardown := force || instance.AgentStopFailed || shouldTeardownDockerContainer(instance.StopReason)
	if teardown {
		defer r.releaseSession(instance.InstanceID)
	}

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
		// The connection is gone, so the container cannot be reached. It
		// stays on the remote host; say so rather than reporting success.
		return fmt.Errorf("remote docker: no live connection for instance %s; container %s was left running",
			instance.InstanceID, instance.ContainerID)
	}

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
