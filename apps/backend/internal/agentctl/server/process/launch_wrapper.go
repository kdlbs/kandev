package process

import (
	"errors"
	"slices"

	"github.com/kandev/kandev/internal/agent/managedruntime"
	"github.com/kandev/kandev/internal/common/launchwrapper"
)

func configuredLaunchWrapper(wrappers []launchwrapper.Spec, argsPresent bool) (launchwrapper.Spec, error) {
	if len(wrappers) == 0 || wrappers[0].IsZero() {
		return launchwrapper.Spec{}, nil
	}
	spec := wrappers[0]
	if err := spec.Validate(); err != nil {
		return launchwrapper.Spec{}, err
	}
	if !argsPresent {
		return launchwrapper.Spec{}, launchwrapper.ErrInvalid
	}
	return spec, nil
}

func (m *Manager) prepareSessionArgs(args []string) ([]string, error) {
	if m.cfg.LaunchWrapperRequired && m.cfg.LaunchWrapper.IsZero() {
		return nil, errors.New("launch wrapper configuration is missing. Session recovery is required")
	}
	if !m.cfg.LaunchWrapper.IsZero() {
		return launchwrapper.Prepare(m.cfg.LaunchWrapper, args, launchwrapper.Context{WorkDir: m.cfg.WorkDir, Env: m.cfg.AgentEnv})
	}
	prepared := slices.Clone(args)
	if err := managedruntime.PrepareNPMProjectPrefix(prepared); err != nil {
		return nil, errors.New("managed npm project prefix could not be prepared")
	}
	return prepared, nil
}

func (m *Manager) prepareOneShotArgs() error {
	if m.adapterCfg == nil || m.adapterCfg.OneShotConfig == nil {
		return nil
	}
	oneShot := m.adapterCfg.OneShotConfig
	initial, continuation := oneShot.InitialArgs, oneShot.ContinueArgs
	if !m.cfg.LaunchWrapper.IsZero() {
		initial, continuation = m.cfg.AgentArgs, m.cfg.ContinueArgs
	}
	preparedInitial, err := m.prepareSessionArgs(initial)
	if err != nil {
		return err
	}
	preparedContinue, err := m.prepareSessionArgs(continuation)
	if err != nil {
		return err
	}
	oneShot.InitialArgs, oneShot.ContinueArgs = preparedInitial, preparedContinue
	return nil
}

// LaunchWrapperSnapshot returns only the retained, unresolved profile wrapper.
func (m *Manager) LaunchWrapperSnapshot() (launchwrapper.Spec, error) {
	m.startMu.Lock()
	defer m.startMu.Unlock()
	spec := m.cfg.LaunchWrapper
	if m.cfg.LaunchWrapperRequired && spec.IsZero() {
		return launchwrapper.Spec{}, launchwrapper.ErrInvalid
	}
	if !spec.IsZero() {
		if err := spec.Validate(); err != nil {
			return launchwrapper.Spec{}, err
		}
	}
	spec.Argv = slices.Clone(spec.Argv)
	return spec, nil
}
