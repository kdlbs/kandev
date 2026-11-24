package lifecycle

import settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"

const MetadataKeyResourceCircuitIdentity = "resource_circuit_identity"

type resourceCircuitIdentity struct {
	ExecutionID string `json:"execution_id"`
	ProfileID   string `json:"profile_id"`
	Key         string `json:"key"`
}

// SetResourceCircuitKeyResolver is wired before startup. It receives the same
// profile snapshot used to construct the execution environment and command.
func (r *StoreProfileResolver) SetResourceCircuitKeyResolver(resolve func(*settingsmodels.AgentProfile) string) {
	r.resourceCircuitKey = resolve
}

func (r *StoreProfileResolver) resolveResourceCircuitKey(profile *settingsmodels.AgentProfile) string {
	if r.resourceCircuitKey == nil {
		return ""
	}
	return r.resourceCircuitKey(profile)
}

func (m *Manager) SetResourceCircuitKeyResolver(resolve func(*settingsmodels.AgentProfile) string) {
	if resolver, ok := m.profileResolver.(interface {
		SetResourceCircuitKeyResolver(func(*settingsmodels.AgentProfile) string)
	}); ok {
		resolver.SetResourceCircuitKeyResolver(resolve)
	}
}

func captureResourceCircuitIdentity(metadata map[string]interface{}, executionID string, profile *AgentProfileInfo) {
	if profile == nil {
		return
	}
	metadata[MetadataKeyResourceCircuitIdentity] = resourceCircuitIdentity{
		ExecutionID: executionID, ProfileID: profile.ProfileID, Key: profile.ResourceCircuitKey,
	}
}

// CapturedResourceCircuitKey returns only the resource belonging to this
// execution/profile pair. Missing retained metadata remains unknown.
func CapturedResourceCircuitKey(metadata map[string]interface{}, executionID, profileID string) string {
	return capturedResourceCircuitKey(metadata[MetadataKeyResourceCircuitIdentity], executionID, profileID)
}

func capturedResourceCircuitKey(value interface{}, executionID, profileID string) string {
	var identity resourceCircuitIdentity
	switch value := value.(type) {
	case resourceCircuitIdentity:
		identity = value
	case map[string]interface{}:
		identity.ExecutionID, _ = value["execution_id"].(string)
		identity.ProfileID, _ = value["profile_id"].(string)
		identity.Key, _ = value["key"].(string)
	}
	if executionID == "" || profileID == "" || identity.ExecutionID != executionID || identity.ProfileID != profileID {
		return ""
	}
	return identity.Key
}

func (e *AgentExecution) resourceCircuitKey() string {
	value, _ := e.metadataValue(MetadataKeyResourceCircuitIdentity)
	return capturedResourceCircuitKey(value, e.ID, e.AgentProfileID)
}
