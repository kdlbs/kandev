package managedruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

var (
	ErrInvalidSelection = errors.New("invalid managed runtime selection")
	errSettingsMissing  = errors.New("managed runtime settings store is unavailable")
)

const selectionKeyPrefix = "managed_runtime.active."

// SettingsStore is the small persistence seam required by the install-wide
// managed-runtime selection store.
type SettingsStore interface {
	Get(context.Context, string) ([]byte, bool, error)
	Save(context.Context, string, []byte) error
	Delete(context.Context, string) error
}

// SelectionStore is the runtime-facing seam for reading and persisting an
// active managed-runtime selection.
type SelectionStore interface {
	Get(context.Context, string, string) (Selection, bool, error)
	Save(context.Context, string, string, string) error
	Delete(context.Context, string, string) error
}

// SelectionReader is the read-only seam used by runtime command builders.
type SelectionReader interface {
	Get(context.Context, string, string) (Selection, bool, error)
}

// Selection is the trusted package identity and exact active version for one
// built-in agent.
type Selection struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

type Store struct {
	settings SettingsStore
}

// NewStore creates the install-wide active-version store.
func NewStore(settings SettingsStore) *Store {
	return &Store{settings: settings}
}

// Get returns the selection only when it belongs to the package currently
// trusted for the agent. A package replacement therefore starts unselected.
func (s *Store) Get(ctx context.Context, agentID, packageName string) (Selection, bool, error) {
	if s == nil || s.settings == nil {
		return Selection{}, false, errSettingsMissing
	}
	if agentID == "" || packageName == "" {
		return Selection{}, false, fmt.Errorf("%w: agent and package are required", ErrInvalidSelection)
	}
	raw, found, err := s.settings.Get(ctx, selectionKey(agentID))
	if err != nil || !found {
		return Selection{}, found, err
	}
	var selection Selection
	if err := json.Unmarshal(raw, &selection); err != nil {
		return Selection{}, false, fmt.Errorf("%w: decode: %v", ErrInvalidSelection, err)
	}
	if selection.Package != packageName {
		return Selection{}, false, nil
	}
	if selection.Version == "" {
		return Selection{}, false, fmt.Errorf("%w: version is empty", ErrInvalidSelection)
	}
	if _, err := ParseStableVersion(selection.Version); err != nil {
		return Selection{}, false, fmt.Errorf("%w: %v", ErrInvalidSelection, err)
	}
	return selection, true, nil
}

// Save persists a validated exact version for one trusted package.
func (s *Store) Save(ctx context.Context, agentID, packageName, version string) error {
	if s == nil || s.settings == nil {
		return errSettingsMissing
	}
	if agentID == "" || packageName == "" {
		return fmt.Errorf("%w: agent and package are required", ErrInvalidSelection)
	}
	if _, err := ParseStableVersion(version); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSelection, err)
	}
	raw, err := json.Marshal(Selection{Package: packageName, Version: version})
	if err != nil {
		return fmt.Errorf("marshal managed runtime selection: %w", err)
	}
	return s.settings.Save(ctx, selectionKey(agentID), raw)
}

// Delete removes the operator selection for one trusted package. The built-in
// default remains outside the settings store and becomes effective after this
// operation succeeds.
func (s *Store) Delete(ctx context.Context, agentID, packageName string) error {
	if s == nil || s.settings == nil {
		return errSettingsMissing
	}
	if agentID == "" || packageName == "" {
		return fmt.Errorf("%w: agent and package are required", ErrInvalidSelection)
	}
	return s.settings.Delete(ctx, selectionKey(agentID))
}

func selectionKey(agentID string) string {
	return selectionKeyPrefix + strings.TrimSpace(agentID)
}
