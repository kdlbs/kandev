package process

import (
	"fmt"

	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/launchwrapper"
	"go.uber.org/zap"
)

func (m *Manager) configure(command string, agentArgs []string, agentArgsPresent bool, env map[string]string, continueCommand string, continueArgs []string, continueArgsPresent, replaceEnv bool, delivery *types.ProjectMCPDelivery, installContext *types.ManagedRuntimeInstallContext, wrapper ...launchwrapper.Spec) error {
	providerRelease, err := m.BeginProviderOperation("")
	if err != nil {
		return err
	}
	defer providerRelease()
	m.startMu.Lock()
	defer m.startMu.Unlock()
	return m.configureLocked(command, agentArgs, agentArgsPresent, env, continueCommand, continueArgs, continueArgsPresent, replaceEnv, delivery, installContext, wrapper...)
}

func (m *Manager) configureLocked(command string, agentArgs []string, agentArgsPresent bool, env map[string]string, continueCommand string, continueArgs []string, continueArgsPresent, replaceEnv bool, delivery *types.ProjectMCPDelivery, installContext *types.ManagedRuntimeInstallContext, wrapper ...launchwrapper.Spec) error {
	if m.Status() == StatusRunning || m.Status() == StatusStarting {
		return ErrConfigureWhileRunning
	}

	spec, err := configuredLaunchWrapper(wrapper, agentArgsPresent)
	if err != nil {
		return err
	}
	args := agentArgs
	if !agentArgsPresent {
		args = config.ParseCommand(command)
	}
	if err := config.ValidateCommandArgs(args); err != nil {
		return err
	}
	continueArgs, err = configuredContinueArgs(continueArgs, continueArgsPresent, continueCommand)
	if err != nil {
		return err
	}

	// Compose the environment before changing any other configuration so a
	// malformed indexed Git block leaves the instance fully unchanged.
	mergedEnv, err := composeConfiguredAgentEnvironmentWithManagedGitTools(m.cfg.AgentEnv, env, replaceEnv, m.managedGitTools)
	if err != nil {
		return fmt.Errorf("compose configured agent environment: %w", err)
	}
	// Delivery evidence is validated before any accepted state changes. An
	// invalid explicit record fails configuration instead of replacing it.
	acceptedDelivery, err := m.acceptProjectMCPDelivery(delivery)
	if err != nil {
		return err
	}

	if !sameManagedCommand(m.cfg.AgentArgs, args) || !sameManagedCommand(m.cfg.LaunchWrapper.Argv, spec.Argv) {
		m.invalidateManagedStartupEvidence()
		m.managedRuntimeCommand = nil
	}
	m.cfg.AgentCommand = command
	m.cfg.AgentCommandConfigured = true
	m.cfg.AgentArgs = args
	m.cfg.LaunchWrapper = launchwrapper.Profile(spec.Argv)
	m.cfg.LaunchWrapperRequired = !spec.IsZero()
	m.cfg.ProjectMCPDelivery = acceptedDelivery

	// Store continue command for one-shot adapters
	if continueArgsPresent {
		m.cfg.ContinueCommand = continueCommand
		m.cfg.ContinueArgs = continueArgs
	} else if continueCommand != "" {
		m.cfg.ContinueCommand = continueCommand
		m.cfg.ContinueArgs = continueArgs
	}

	m.cfg.AgentEnv = mergedEnv
	m.cfg.ManagedRuntimeInstallContext = installContext.Clone()
	if m.adapterCfg != nil && m.adapterCfg.OneShotConfig != nil {
		m.adapterCfg.OneShotConfig.Env = append([]string(nil), m.cfg.AgentEnv...)
	}
	m.publishWorkspaceEnvironmentLocked()

	m.configRevision++
	m.logger.Info("agent configured", zap.Int("env_count", len(env)), zap.Bool("wrapped", !spec.IsZero()))

	return nil
}
