package service

import (
	"errors"
	"fmt"
	"strings"
)

// ReservedMetadataKeyPrefixCoordinator prefixes task metadata keys only the
// coordinator service may write, such as the conversation's tool binding.
const ReservedMetadataKeyPrefixCoordinator = "kandev.coordinator_"

// ErrReservedMetadata reports a create or update request whose metadata names a
// reserved key without AllowReservedMetadata.
var ErrReservedMetadata = errors.New("task metadata key is reserved")

// refuseReservedMetadata returns ErrReservedMetadata when metadata carries a
// key with the reserved prefix and the caller is not the coordinator service.
func refuseReservedMetadata(metadata map[string]interface{}, allowed bool) error {
	if allowed {
		return nil
	}
	for key := range metadata {
		if strings.HasPrefix(key, ReservedMetadataKeyPrefixCoordinator) {
			return fmt.Errorf("%w: %s", ErrReservedMetadata, key)
		}
	}
	return nil
}

// restoreReservedMetadata copies every reserved key of existing that the
// updated map omits, so an update that does not name the binding keeps it.
func restoreReservedMetadata(updated, existing map[string]interface{}) {
	for key, value := range existing {
		if !strings.HasPrefix(key, ReservedMetadataKeyPrefixCoordinator) {
			continue
		}
		if _, present := updated[key]; !present {
			updated[key] = value
		}
	}
}
