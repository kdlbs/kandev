package lifecycle

// RequiresCloneURL implements orchestrator/executor.ExecutorTypeCapabilities.
func (m *Manager) RequiresCloneURL(executorType string) bool {
	backend, err := m.getExecutorBackend(executorType)
	if err != nil {
		return false
	}
	return backend.RequiresCloneURL()
}

// ShouldApplyPreferredShell implements orchestrator/executor.ExecutorTypeCapabilities.
func (m *Manager) ShouldApplyPreferredShell(executorType string) bool {
	backend, err := m.getExecutorBackend(executorType)
	if err != nil {
		return false
	}
	return backend.ShouldApplyPreferredShell()
}
