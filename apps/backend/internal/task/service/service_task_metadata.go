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

// keeping server-managed terminal retention, deferred-launch, step-handoff, task-handoff, and
// task-boundary causation records owned by the server. The HTTP PATCH surface
// may replace ordinary metadata, but it cannot create, replace, or remove
// these records. It cannot set, reset, or lower the causation carrier: any
// office_carrier_* key in the request is stripped, then the task's existing
// carrier is restored in its place.
func protectedTaskMetadataUpdate(existing, requested map[string]interface{}) map[string]interface{} {
	updated := cloneTaskMetadata(requested)
	if updated == nil {
		updated = make(map[string]interface{})
	}
	delete(updated, models.MetaKeyTerminalRetention)
	if held, ok := existing[models.MetaKeyTerminalRetention]; ok {
		updated[models.MetaKeyTerminalRetention] = held
	}
	models.StripOfficeCarrierMetadata(updated)
	models.RestoreOfficeCarrierMetadata(updated, existing)
	if deferred, ok := existing[models.MetaKeyDeferredLaunch]; ok {
		updated[models.MetaKeyDeferredLaunch] = deferred
	} else {
		delete(updated, models.MetaKeyDeferredLaunch)
	}
	if carry, ok := existing[models.MetaKeyStepHandoffCarry]; ok {
		updated[models.MetaKeyStepHandoffCarry] = carry
	} else {
		delete(updated, models.MetaKeyStepHandoffCarry)
	}
	if source, ok := existing[models.MetaKeyHandoffSource]; ok {
		updated[models.MetaKeyHandoffSource] = source
	} else {
		delete(updated, models.MetaKeyHandoffSource)
	}
	if handoffs, ok := existing[models.MetaKeyHandoffs]; ok {
		updated[models.MetaKeyHandoffs] = handoffs
	} else {
		delete(updated, models.MetaKeyHandoffs)
	}
	return updated
}

// protectedTaskMetadataForCreate strips server-managed records from ordinary
// task creation. The handoff path opts in after it has built and authorized the
// provenance payload itself. Office causation carriers always come from the
// separate trusted request field and are never accepted from Metadata.
func protectedTaskMetadataForCreate(metadata map[string]interface{}, trustedHandoff bool) map[string]interface{} {
	created := cloneTaskMetadata(metadata)
	delete(created, models.MetaKeyTerminalRetention)
	models.StripOfficeCarrierMetadata(created)
	delete(created, models.MetaKeyDeferredLaunch)
	delete(created, models.MetaKeyStepHandoffCarry)
	if !trustedHandoff {
		delete(created, models.MetaKeyHandoffSource)
		delete(created, models.MetaKeyHandoffs)
	}
	return created
}
