package service

import (
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
)

// ReservedMetadataKeyPrefixCoordinator prefixes task metadata keys only the
// coordinator service may write, such as the conversation's tool binding.
const ReservedMetadataKeyPrefixCoordinator = models.ReservedMetadataKeyPrefixCoordinator

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
