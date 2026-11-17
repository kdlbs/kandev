package process

import (
	"errors"

	"github.com/kandev/kandev/internal/common/launchwrapper"
)

// providerWorkspaceEnvironment retains the instance environment independently
// of later provider configuration. startMu guards publication and reads.
type providerWorkspaceEnvironment struct{ env []string }

func (m *Manager) workspaceEnvLocked() []string {
	if m.workspaceEnvironment != nil {
		return append([]string(nil), m.workspaceEnvironment.env...)
	}
	return append([]string(nil), m.cfg.AgentEnv...)
}

func (m *Manager) workspaceEnvSnapshot() []string {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	return m.workspaceEnvLocked()
}

// Ordinary initial configuration publishes the workspace environment. Once a
// provider replacement retains it, provider changes cannot update its owners.
func (m *Manager) publishWorkspaceEnvironmentLocked() {
	if m.workspaceEnvironment == nil {
		m.setTrackerGitEnvironment(m.cfg.AgentEnv)
	}
}

// configureProviderForReplacement accepts a detached provider command/environment
// only for the exact stopped source. Workspace ownership remains unchanged.
func (m *Manager) configureProviderForReplacement(source providerReplacementSource, args []string, env map[string]string) (providerReplacementSource, error) {
	m.startOperationMu.Lock()
	defer m.startOperationMu.Unlock()
	m.startMu.Lock()
	defer m.startMu.Unlock()
	if source.managerID != m.ProcessManagerID() || source.processGeneration == 0 || source.processGeneration != m.ProcessGeneration() || source.configRevision != m.configRevision || source.stopGeneration != m.stopGeneration || m.Status() != StatusStopped {
		return providerReplacementSource{}, errors.New("provider configuration source changed")
	}
	args = append([]string(nil), args...)
	var wrapper []launchwrapper.Spec
	if !m.cfg.LaunchWrapper.IsZero() {
		wrapper = []launchwrapper.Spec{launchwrapper.Profile(m.cfg.LaunchWrapper.Argv)}
	}
	if err := m.configureLocked("", args, true, env, "", nil, false, true, nil, m.cfg.ManagedRuntimeInstallContext, wrapper...); err != nil {
		return providerReplacementSource{}, err
	}
	source.configRevision = m.configRevision
	return source, nil
}
