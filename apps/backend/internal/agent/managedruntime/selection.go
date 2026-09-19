package managedruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

var (
	ErrInvalidSelection = errors.New("invalid managed runtime selection")
	errSettingsMissing  = errors.New("managed runtime settings store is unavailable")
)

const (
	selectionKeyPrefix         = "managed_runtime.active."
	defaultGenerationKeyPrefix = "managed_runtime.default."
)

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

// DefaultGeneration identifies the trusted package and exact default version
// shipped for one built-in managed agent.
type DefaultGeneration struct {
	AgentID string
	Package string
	Version string
}

// DefaultGenerationChange describes a generation that was newly applied.
type DefaultGenerationChange struct {
	Previous DefaultGeneration
	Current  DefaultGeneration
}

// DefaultGenerationChangeHandler observes successful generation changes.
type DefaultGenerationChangeHandler func(DefaultGenerationChange)

type appliedDefaultGeneration struct {
	Package string `json:"package"`
	Version string `json:"version"`
}

type Store struct {
	settings                   SettingsStore
	onDefaultGenerationChanged DefaultGenerationChangeHandler
}

// NewStore creates the install-wide active-version store.
func NewStore(settings SettingsStore) *Store {
	return &Store{settings: settings}
}

// SetDefaultGenerationChangeHandler wires the startup observer for applied
// generations. It is called only after the marker has been stored successfully.
func (s *Store) SetDefaultGenerationChangeHandler(handler DefaultGenerationChangeHandler) {
	if s == nil {
		return
	}
	s.onDefaultGenerationChanged = handler
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

// ReconcileDefaults activates the current trusted default generation for each
// managed agent. A changed or missing marker clears the active selection before
// saving the new marker, so an interrupted reconciliation can be safely retried.
func (s *Store) ReconcileDefaults(ctx context.Context, generations []DefaultGeneration) error {
	if s == nil || s.settings == nil {
		return errSettingsMissing
	}

	ordered := append([]DefaultGeneration(nil), generations...)
	sort.Slice(ordered, func(i, j int) bool {
		return ordered[i].AgentID < ordered[j].AgentID
	})
	for _, generation := range ordered {
		if err := validateDefaultGeneration(generation); err != nil {
			return fmt.Errorf("validate managed runtime default for %q: %w", generation.AgentID, err)
		}

		markerKey := defaultGenerationKey(generation.AgentID)
		rawMarker, found, err := s.settings.Get(ctx, markerKey)
		if err != nil {
			return fmt.Errorf("read managed runtime default for %q: %w", generation.AgentID, err)
		}
		previous, markerValid := decodeDefaultGeneration(rawMarker, generation.AgentID)
		if found && markerValid && matchesDefaultGeneration(rawMarker, generation) {
			continue
		}

		activeSelectionKey := selectionKey(generation.AgentID)
		_, selectionFound, err := s.settings.Get(ctx, activeSelectionKey)
		if err != nil {
			return fmt.Errorf("read managed runtime selection for %q: %w", generation.AgentID, err)
		}
		if selectionFound {
			if err := s.settings.Delete(ctx, activeSelectionKey); err != nil {
				return fmt.Errorf("delete managed runtime selection for %q: %w", generation.AgentID, err)
			}
		}

		rawGeneration, err := json.Marshal(appliedDefaultGeneration{
			Package: generation.Package,
			Version: generation.Version,
		})
		if err != nil {
			return fmt.Errorf("encode managed runtime default for %q: %w", generation.AgentID, err)
		}
		if err := s.settings.Save(ctx, markerKey, rawGeneration); err != nil {
			return fmt.Errorf("save managed runtime default for %q: %w", generation.AgentID, err)
		}
		if s.onDefaultGenerationChanged != nil {
			s.onDefaultGenerationChanged(DefaultGenerationChange{
				Previous: previous,
				Current:  generation,
			})
		}
	}
	return nil
}

func validateDefaultGeneration(generation DefaultGeneration) error {
	if strings.TrimSpace(generation.AgentID) == "" || strings.TrimSpace(generation.Package) == "" {
		return fmt.Errorf("agent and package are required")
	}
	if _, err := ParseStableVersion(generation.Version); err != nil {
		return fmt.Errorf("default version: %w", err)
	}
	return nil
}

func matchesDefaultGeneration(raw []byte, generation DefaultGeneration) bool {
	applied, ok := decodeDefaultGeneration(raw, generation.AgentID)
	return ok && applied.Package == generation.Package && applied.Version == generation.Version
}

func decodeDefaultGeneration(raw []byte, agentID string) (DefaultGeneration, bool) {
	var applied appliedDefaultGeneration
	if err := json.Unmarshal(raw, &applied); err != nil {
		return DefaultGeneration{}, false
	}
	return DefaultGeneration{
		AgentID: agentID,
		Package: applied.Package,
		Version: applied.Version,
	}, true
}

func selectionKey(agentID string) string {
	return selectionKeyPrefix + strings.TrimSpace(agentID)
}

func defaultGenerationKey(agentID string) string {
	return defaultGenerationKeyPrefix + strings.TrimSpace(agentID)
}
