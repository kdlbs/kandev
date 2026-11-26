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

// refuseReservedMetadata rejects interruption state from every caller and
// other reserved keys from callers without trusted service authority.
func refuseReservedMetadata(metadata map[string]interface{}, allowed bool) error {
	for key := range metadata {
		if key == models.MetaKeyInterruptedSessions || key == models.MetaKeyLegacyInterruptedAt {
			return fmt.Errorf("%w: %s", ErrReservedMetadata, key)
		}
	}
	if allowed {
		return nil
	}
	for key := range metadata {
		if strings.HasPrefix(key, ReservedMetadataKeyPrefixCoordinator) || key == models.MetaKeyLastTurnError {
			return fmt.Errorf("%w: %s", ErrReservedMetadata, key)
		}
	}
	return nil
}
