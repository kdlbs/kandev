package lifecycle

import agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"

// ExecutionStoreForTesting returns the manager's execution store.
// Intended for use by test packages that need to inject executions.
func (m *Manager) ExecutionStoreForTesting() *ExecutionStore {
	return m.executionStore
}

// SetAgentCtlClientForTesting injects an agentctl client into a synthetic
// execution used by focused tests in dependent packages.
func (ae *AgentExecution) SetAgentCtlClientForTesting(client *agentctl.Client) {
	ae.agentctl = client
}

// SetMetadataForTesting seeds metadata on a synthetic execution built by a test
// in a dependent package. The metadata map itself is unexported so that all
// production access stays behind the metadataMu helpers.
func (ae *AgentExecution) SetMetadataForTesting(metadata map[string]interface{}) {
	ae.metadataMu.Lock()
	defer ae.metadataMu.Unlock()
	ae.metadata = metadata
}
