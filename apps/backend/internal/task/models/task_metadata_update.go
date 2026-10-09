package models

import (
	"maps"
	"strings"
)

// ReservedMetadataKeyPrefixCoordinator prefixes task metadata keys only the
// coordinator service may write, such as the conversation's tool binding. An
// update that omits one keeps it.
const ReservedMetadataKeyPrefixCoordinator = "kandev.coordinator_"

// ProtectedTaskMetadataUpdate replaces caller-owned metadata while retaining
// the current server-owned lifecycle, handoff, causation and coordinator
// reserved records.
func ProtectedTaskMetadataUpdate(existing, requested map[string]interface{}) map[string]interface{} {
	updated := maps.Clone(requested)
	if updated == nil {
		updated = make(map[string]interface{})
	}
	StripOfficeCarrierMetadata(updated)
	RestoreOfficeCarrierMetadata(updated, existing)
	for _, key := range []string{MetaKeyDeferredLaunch, MetaKeyStepHandoffCarry, MetaKeyHandoffSource, MetaKeyHandoffs} {
		if value, ok := existing[key]; ok {
			updated[key] = value
		} else {
			delete(updated, key)
		}
	}
	for key, value := range existing {
		if !strings.HasPrefix(key, ReservedMetadataKeyPrefixCoordinator) {
			continue
		}
		if _, present := updated[key]; !present {
			updated[key] = value
		}
	}
	return updated
}
