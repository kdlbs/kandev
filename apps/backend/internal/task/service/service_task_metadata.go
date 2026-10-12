package service

import (
	"maps"

	"github.com/kandev/kandev/internal/task/models"
)

// cloneTaskMetadata copies the top-level task metadata map so request-owned
// maps cannot be mutated while the service adds or protects server state.
func cloneTaskMetadata(metadata map[string]interface{}) map[string]interface{} {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]interface{}, len(metadata))
	maps.Copy(cloned, metadata)
	return cloned
}

// protectedTaskMetadataUpdate retains the shared task-model ownership rules.
func protectedTaskMetadataUpdate(existing, requested map[string]interface{}) map[string]interface{} {
	return models.ProtectedTaskMetadataUpdate(existing, requested)
}

// protectedTaskMetadataForCreate strips server-managed records from ordinary
// task creation. The handoff path opts in after it has built and authorized the
// provenance payload itself. Office causation carriers always come from the
// separate trusted request field and are never accepted from Metadata.
func protectedTaskMetadataForCreate(metadata map[string]interface{}, trustedHandoff bool) map[string]interface{} {
	created := cloneTaskMetadata(metadata)
	models.StripOfficeCarrierMetadata(created)
	delete(created, models.MetaKeyDeferredLaunch)
	delete(created, models.MetaKeyStepHandoffCarry)
	if !trustedHandoff {
		delete(created, models.MetaKeyHandoffSource)
		delete(created, models.MetaKeyHandoffs)
	}
	return created
}
