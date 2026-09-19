package lifecycle

import (
	"fmt"

	"github.com/kandev/kandev/internal/agent/agents"
)

// ContainerHostFiles resolves the files a container mounts from outside its
// own image: the agentctl helper, the per-instance agent session directory,
// and the E2E mock-agent binary.
//
// These are the only container inputs whose source is a path on a filesystem
// rather than content inside the image, which makes them the part of the
// Docker executor that does not survive moving the daemon to another machine.
// A remote daemon resolves them on the remote host instead.
type ContainerHostFiles interface {
	// AgentctlBinary returns the path the container mounts at
	// /usr/local/bin/agentctl, so user-built images need not bake it in.
	AgentctlBinary() (string, error)

	// MockAgentBinary returns the E2E mock-agent path, or "" when none
	// applies, which is the production case.
	MockAgentBinary() (string, error)

	// SessionDir returns the per-instance agent session directory to mount,
	// or "" when the agent does not use one. Agents without a full
	// template+target pair rely on their in-container setup script instead.
	//
	// It reports an error rather than an empty path when the directory should
	// exist but could not be produced: dropping the mount silently would
	// start an agent without its seeded credentials and no stated cause.
	SessionDir(ag agents.Agent, instanceID string) (source string, target string, err error)
}

// localContainerHostFiles resolves sources on the machine running the backend.
// It is the behavior the Docker executor has always had.
type localContainerHostFiles struct {
	resolveAgentctlBinary  func() (string, error)
	resolveMockAgentBinary func() (string, error)
	commandBuilder         *CommandBuilder
	kandevHomeDir          string
}

func (l localContainerHostFiles) AgentctlBinary() (string, error) {
	if l.resolveAgentctlBinary == nil {
		return "", nil
	}
	path, err := l.resolveAgentctlBinary()
	if err != nil {
		return "", fmt.Errorf("agentctl linux binary not found: %w", err)
	}
	return path, nil
}

func (l localContainerHostFiles) MockAgentBinary() (string, error) {
	if l.resolveMockAgentBinary == nil {
		return "", nil
	}
	path, err := l.resolveMockAgentBinary()
	if err != nil {
		return "", fmt.Errorf("mock-agent binary lookup: %w", err)
	}
	return path, nil
}

func (l localContainerHostFiles) SessionDir(ag agents.Agent, instanceID string) (string, string, error) {
	if l.commandBuilder == nil {
		return "", "", nil
	}
	source := l.commandBuilder.ExpandSessionDir(ag, l.kandevHomeDir, instanceID)
	target := l.commandBuilder.GetSessionDirTarget(ag)
	return source, target, nil
}

// hostFiles returns the manager's configured provider, defaulting to the local
// one built from the manager's own resolvers.
func (cm *ContainerManager) hostFiles() ContainerHostFiles {
	if cm.containerHostFiles != nil {
		return cm.containerHostFiles
	}
	return localContainerHostFiles{
		resolveAgentctlBinary:  cm.resolveAgentctlBinary,
		resolveMockAgentBinary: cm.resolveMockAgentBinary,
		commandBuilder:         cm.commandBuilder,
		kandevHomeDir:          cm.kandevHomeDir,
	}
}

// remoteHostFileStore materializes container inputs on a remote host. The SSH
// implementation lives with the remote Docker runtime; keeping it an interface
// here lets the mount composition be tested without a live connection.
type remoteHostFileStore interface {
	// EnsureAgentctl uploads (or reuses a cached) agentctl helper matching
	// the remote platform and returns its remote path.
	EnsureAgentctl(platform SSHRemotePlatform) (string, error)

	// EnsureSessionDir creates the per-instance session directory on the
	// remote host and returns its remote path.
	EnsureSessionDir(instanceID string) (string, error)

	// EnsureFile uploads a local file to the remote host under name and
	// returns its remote path.
	EnsureFile(name, localPath string) (string, error)
}

// remoteContainerHostFiles resolves every mount source on the remote host.
type remoteContainerHostFiles struct {
	store          remoteHostFileStore
	platform       SSHRemotePlatform
	commandBuilder *CommandBuilder
	// resolveMockAgentBinary is the backend-side E2E mock-agent lookup. It
	// returns "" in production, so the upload below never runs there.
	resolveMockAgentBinary func() (string, error)
}

func newRemoteContainerHostFiles(
	store remoteHostFileStore,
	platform SSHRemotePlatform,
	commandBuilder *CommandBuilder,
) *remoteContainerHostFiles {
	return &remoteContainerHostFiles{store: store, platform: platform, commandBuilder: commandBuilder}
}

func (r *remoteContainerHostFiles) AgentctlBinary() (string, error) {
	// Checked before the upload so an unsupported remote fails with its own
	// cause rather than producing a container whose helper cannot execute.
	if err := SSHRequireSupportedRemotePlatform(r.platform); err != nil {
		return "", err
	}
	path, err := r.store.EnsureAgentctl(r.platform)
	if err != nil {
		return "", fmt.Errorf("remote docker: deliver agentctl: %w", err)
	}
	return path, nil
}

// MockAgentBinary uploads the E2E mock agent when the backend resolves one.
//
// The binary lives in the backend's own build tree, so the remote daemon
// cannot mount that path. It is delivered the same way agentctl is, rather
// than dropped: dropping it produces a container that starts and then fails
// with "the agent could not start", which names nothing useful.
//
// The production resolver returns "", so nothing is uploaded there.
func (r *remoteContainerHostFiles) MockAgentBinary() (string, error) {
	if r.resolveMockAgentBinary == nil {
		return "", nil
	}
	localPath, err := r.resolveMockAgentBinary()
	if err != nil {
		return "", fmt.Errorf("mock-agent binary lookup: %w", err)
	}
	if localPath == "" {
		return "", nil
	}
	remotePath, err := r.store.EnsureFile("mock-agent", localPath)
	if err != nil {
		return "", fmt.Errorf("remote docker: deliver mock-agent: %w", err)
	}
	return remotePath, nil
}

func (r *remoteContainerHostFiles) SessionDir(ag agents.Agent, instanceID string) (string, string, error) {
	if r.commandBuilder == nil {
		return "", "", nil
	}
	target := r.commandBuilder.GetSessionDirTarget(ag)
	if target == "" {
		return "", "", nil
	}
	source, err := r.store.EnsureSessionDir(instanceID)
	if err != nil {
		return "", "", fmt.Errorf("remote docker: create agent session dir: %w", err)
	}
	return source, target, nil
}
