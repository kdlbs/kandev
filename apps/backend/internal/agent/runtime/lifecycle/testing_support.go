package lifecycle

import (
	"maps"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

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
//
// The argument is copied rather than adopted, so a caller that keeps reading or
// writing the map it passed in cannot reach the execution's store without going
// through those helpers.
func (ae *AgentExecution) SetMetadataForTesting(metadata map[string]interface{}) {
	ae.metadataMu.Lock()
	defer ae.metadataMu.Unlock()
	if metadata == nil {
		ae.metadata = nil
		return
	}
	ae.metadata = make(map[string]interface{}, len(metadata))
	maps.Copy(ae.metadata, metadata)
}
