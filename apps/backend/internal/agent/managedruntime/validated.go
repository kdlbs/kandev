package managedruntime

import (
	"context"
	"fmt"
	"strings"
)

const validatedKeyPrefix = "managed_runtime.validated."

// ValidatedVersionStore records the exact managed package version that the
// most recent successful activation probed. It is independent of the operator
// selection, so returning to the Kandev default never persists a selection.
type ValidatedVersionStore interface {
	GetValidated(context.Context, string, string) (Selection, bool, error)
	SaveValidated(context.Context, string, string, string) error
}

var _ ValidatedVersionStore = (*Store)(nil)

// GetValidated returns the validation record only when it belongs to the
// package currently trusted for the agent.
func (s *Store) GetValidated(ctx context.Context, agentID, packageName string) (Selection, bool, error) {
	if s == nil || s.settings == nil {
		return Selection{}, false, errSettingsMissing
	}
	if agentID == "" || packageName == "" {
		return Selection{}, false, fmt.Errorf("%w: agent and package are required", ErrInvalidSelection)
	}
	return s.readVersionRecord(ctx, validatedKey(agentID), packageName)
}

// SaveValidated persists the exact version a successful activation probed.
func (s *Store) SaveValidated(ctx context.Context, agentID, packageName, version string) error {
	if s == nil || s.settings == nil {
		return errSettingsMissing
	}
	return s.writeVersionRecord(ctx, agentID, packageName, version, validatedKey(agentID))
}

func validatedKey(agentID string) string {
	return validatedKeyPrefix + strings.TrimSpace(agentID)
}
