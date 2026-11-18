package process

import (
	"context"
	"errors"
)

// providerReplacementSource fences a stopped provider against later full stops,
// configuration changes, and replacement process starts on the same manager.
type providerReplacementSource struct {
	managerID         string
	processGeneration uint64
	configRevision    uint64
	stopGeneration    uint64
}

// The caller must settle provider work and close provider effect admission
// before invoking this lifecycle primitive. Workspace admission stays open.
func (m *Manager) stopProviderForReplacement(ctx context.Context, generation uint64) (providerReplacementSource, error) {
	m.startOperationMu.Lock()
	defer m.startOperationMu.Unlock()
	m.startMu.Lock()
	source := providerReplacementSource{managerID: m.ProcessManagerID(), processGeneration: generation, configRevision: m.configRevision, stopGeneration: m.stopGeneration}
	valid := generation != 0 && m.ProcessGeneration() == generation && m.Status() == StatusRunning
	if valid && m.workspaceEnvironment == nil {
		m.workspaceEnvironment = &providerWorkspaceEnvironment{env: append([]string(nil), m.cfg.AgentEnv...)}
	}
	m.startMu.Unlock()
	if !valid {
		return providerReplacementSource{}, errors.New("provider replacement source is no longer running")
	}
	if err := m.stopWithWorkspace(ctx, false); err != nil {
		return providerReplacementSource{}, err
	}
	return source, nil
}

func (m *Manager) startProviderForReplacement(ctx context.Context, source providerReplacementSource) (uint64, error) {
	operationCtx, release, err := m.BeginOwnedOperation(ctx)
	if err != nil {
		return 0, err
	}
	defer release()
	m.startOperationMu.Lock()
	defer m.startOperationMu.Unlock()
	if source.managerID != m.ProcessManagerID() || source.processGeneration == 0 || m.ProcessGeneration() != source.processGeneration {
		return 0, errors.New("provider replacement generation changed")
	}
	return m.startGeneration(operationCtx, nil, nil, &source.configRevision, &source.stopGeneration, nil, false)
}

func (m *Manager) stopWorkspaceProcessesIfRequested(ctx context.Context, workspace bool) error {
	if !workspace {
		return nil
	}
	return m.stopShellAndProcesses(ctx)
}
