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

// protectedTaskMetadataUpdate applies a generic metadata replacement while
// keeping the deferred launch intent owned by the server. The HTTP PATCH
// surface may replace ordinary metadata, but it cannot create, replace, or
// remove the launch record that carries deferred-start ownership.
func protectedTaskMetadataUpdate(existing, requested map[string]interface{}) map[string]interface{} {
	updated := cloneTaskMetadata(requested)
	if updated == nil {
		updated = make(map[string]interface{})
	}
	if deferred, ok := existing[models.MetaKeyDeferredLaunch]; ok {
		updated[models.MetaKeyDeferredLaunch] = deferred
	} else {
		delete(updated, models.MetaKeyDeferredLaunch)
	}
	return updated
}
