package storage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

const settingsKey = "storage_maintenance"

type SettingsStore struct {
	settings *systemsettings.Store
}

type SaveConfirmations struct {
	DedicatedDocker bool
	adoptGoCache    bool
}

func NewSettingsStore(settings *systemsettings.Store) *SettingsStore {
	return &SettingsStore{settings: settings}
}

func (s *SettingsStore) GetSettings(ctx context.Context) (StorageMaintenanceSettings, error) {
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
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return StorageMaintenanceSettings{}, err
	}
	current, err := s.GetSettings(ctx)
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

func (s *SettingsStore) AdoptGoCachePath(
	ctx context.Context,
	path string,
) (StorageMaintenanceSettings, error) {
	if path == "" {
		return StorageMaintenanceSettings{}, validationError("go_cache.adopted_path is required")
	}
	settings, err := s.GetSettings(ctx)
	if err != nil {
		if !errors.Is(err, ErrInvalidPersistedSettings) {
			return StorageMaintenanceSettings{}, err
		}
		settings = DefaultSettings()
	}
	settings.GoCache.AdoptedPath = path
	return s.SaveSettingsWithConfirmations(ctx, settings, SaveConfirmations{adoptGoCache: true})
}
