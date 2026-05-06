package lifecycle

// ExecutionStoreForTesting returns the manager's execution store.
// Intended for use by test packages that need to inject executions.
func (m *Manager) ExecutionStoreForTesting() *ExecutionStore {
	return m.executionStore
}
