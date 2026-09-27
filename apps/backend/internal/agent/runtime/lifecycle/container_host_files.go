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

// remoteContainerHostFiles supplies no mount at all.
//
// A remote daemon resolves a bind-mount source against the remote host's
// filesystem, so every source here would be a file Kandev had to create there.
// The remote runtime delivers those inputs into the container through the
// Docker Engine API instead (see remoteContainerInputs), which is what lets a
// task run on a host that accepts no filesystem writes.
//
// The provider still exists for one reason: the platform check has to fail
// before a container is created, and this is the only hook that runs that
// early.
type remoteContainerHostFiles struct {
	platform SSHRemotePlatform
}

func newRemoteContainerHostFiles(platform SSHRemotePlatform) remoteContainerHostFiles {
	return remoteContainerHostFiles{platform: platform}
}

// AgentctlBinary reports no mount source, and rejects a remote whose platform
// has no matching helper. Failing here keeps an unsupported remote from
// producing a container whose helper cannot execute.
func (r remoteContainerHostFiles) AgentctlBinary() (string, error) {
	if err := SSHRequireSupportedRemotePlatform(r.platform); err != nil {
		return "", err
	}
	return "", nil
}

// MockAgentBinary reports no mount source. The E2E helper is delivered with
// agentctl.
func (r remoteContainerHostFiles) MockAgentBinary() (string, error) { return "", nil }

// SessionDir reports no mount. The directory is created inside the container by
// the delivered archive, so it is removed when the container is.
func (r remoteContainerHostFiles) SessionDir(agents.Agent, string) (string, string, error) {
	return "", "", nil
}
