package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"

	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

const settingsKey = "storage_maintenance"

type SettingsStore struct {
	settings *systemsettings.Store
	mu       sync.Mutex
}

type SaveConfirmations struct {
	DedicatedDocker bool
	adoptGoCache    bool
}

func NewSaveConfirmations(dedicatedDocker, adoptGoCache bool) SaveConfirmations {
	return SaveConfirmations{DedicatedDocker: dedicatedDocker, adoptGoCache: adoptGoCache}
}

func NewSettingsStore(settings *systemsettings.Store) *SettingsStore {
	return &SettingsStore{settings: settings}
}

func (s *SettingsStore) GetSettings(ctx context.Context) (StorageMaintenanceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.getSettings(ctx)
}

func (s *SettingsStore) getSettings(ctx context.Context) (StorageMaintenanceSettings, error) {
	raw, found, err := s.settings.Get(ctx, settingsKey)
	if err != nil || !found {
		return DefaultSettings(), err
	}
	settings := DefaultSettings()
	if err := json.Unmarshal(raw, &settings); err != nil {
		return DefaultSettings(), fmt.Errorf("%w: decode JSON: %w", ErrInvalidPersistedSettings, err)
	}
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return DefaultSettings(), fmt.Errorf("%w: %w", ErrInvalidPersistedSettings, err)
	}
	return normalized, nil
}

func (s *SettingsStore) SaveSettings(
	ctx context.Context,
	settings StorageMaintenanceSettings,
) (StorageMaintenanceSettings, error) {
	return s.SaveSettingsWithConfirmations(ctx, settings, SaveConfirmations{})
}

func (s *SettingsStore) SaveSettingsWithConfirmations(
	ctx context.Context,
	settings StorageMaintenanceSettings,
	confirmations SaveConfirmations,
) (StorageMaintenanceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveSettingsWithConfirmations(ctx, settings, confirmations)
}

func (s *SettingsStore) saveSettingsWithConfirmations(
	ctx context.Context,
	settings StorageMaintenanceSettings,
	confirmations SaveConfirmations,
) (StorageMaintenanceSettings, error) {
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return StorageMaintenanceSettings{}, err
	}
	current, err := s.getSettings(ctx)
	if err != nil {
		if !errors.Is(err, ErrInvalidPersistedSettings) {
			return StorageMaintenanceSettings{}, err
		}
		current = DefaultSettings()
	}
	if normalized.GoCache.AdoptedPath != current.GoCache.AdoptedPath && !confirmations.adoptGoCache {
		return StorageMaintenanceSettings{}, ErrAdoptionRequired
	}
	if normalized.Docker.DedicatedDaemonAcknowledged &&
		!current.Docker.DedicatedDaemonAcknowledged && !confirmations.DedicatedDocker {
		return StorageMaintenanceSettings{}, ErrDedicatedDockerConfirmation
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if err := s.settings.Save(ctx, settingsKey, raw); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	return normalized, nil
}

// PatchSettingsWithConfirmations merges a bounded JSON patch while holding the
// storage ownership lock. The current document, confirmation checks, result
// validation, and write therefore share one atomic boundary with HTTP saves.
func (s *SettingsStore) PatchSettingsWithConfirmations(
	ctx context.Context,
	changes map[string]json.RawMessage,
	confirmations SaveConfirmations,
) (StorageMaintenanceSettings, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.getSettings(ctx)
	if err != nil {
		if !errors.Is(err, ErrInvalidPersistedSettings) {
			return StorageMaintenanceSettings{}, err
		}
		current = DefaultSettings()
	}
	payload, err := json.Marshal(current)
	if err != nil {
		return StorageMaintenanceSettings{}, err
	}
	var document map[string]any
	if err := json.Unmarshal(payload, &document); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	for path, raw := range changes {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return StorageMaintenanceSettings{}, fmt.Errorf("invalid storage setting %q: %w", path, err)
		}
		if err := setNestedJSONValue(document, path, value); err != nil {
			return StorageMaintenanceSettings{}, err
		}
	}
	nextPayload, err := json.Marshal(document)
	if err != nil {
		return StorageMaintenanceSettings{}, err
	}
	var next StorageMaintenanceSettings
	if err := json.Unmarshal(nextPayload, &next); err != nil {
		return StorageMaintenanceSettings{}, fmt.Errorf("invalid storage settings: %w", err)
	}
	return s.saveSettingsWithCurrent(ctx, next, current, confirmations)
}

func (s *SettingsStore) saveSettingsWithCurrent(
	ctx context.Context,
	settings StorageMaintenanceSettings,
	current StorageMaintenanceSettings,
	confirmations SaveConfirmations,
) (StorageMaintenanceSettings, error) {
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if normalized.GoCache.AdoptedPath != current.GoCache.AdoptedPath && !confirmations.adoptGoCache {
		return StorageMaintenanceSettings{}, ErrAdoptionRequired
	}
	if normalized.Docker.DedicatedDaemonAcknowledged &&
		!current.Docker.DedicatedDaemonAcknowledged && !confirmations.DedicatedDocker {
		return StorageMaintenanceSettings{}, ErrDedicatedDockerConfirmation
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return StorageMaintenanceSettings{}, err
	}
	if err := s.settings.Save(ctx, settingsKey, raw); err != nil {
		return StorageMaintenanceSettings{}, err
	}
	return normalized, nil
}

func (s *SettingsStore) AdoptGoCachePath(
	ctx context.Context,
	path string,
) (StorageMaintenanceSettings, error) {
	if path == "" {
		return StorageMaintenanceSettings{}, validationError("go_cache.adopted_path is required")
	}
	return s.PatchSettingsWithConfirmations(ctx, map[string]json.RawMessage{
		"go_cache.adopted_path": json.RawMessage(strconv.Quote(path)),
	}, SaveConfirmations{adoptGoCache: true})
}

func setNestedJSONValue(document map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	if len(parts) == 0 || parts[0] == "" {
		return fmt.Errorf("storage setting path %q is invalid", path)
	}
	current := document
	for _, part := range parts[:len(parts)-1] {
		nested, ok := current[part].(map[string]any)
		if !ok {
			return fmt.Errorf("storage setting path %q is invalid", path)
		}
		current = nested
	}
	current[parts[len(parts)-1]] = value
	return nil
}
