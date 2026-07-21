package metrics

import (
	"context"
	"encoding/json"
	"fmt"

	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

const settingsKey = "system_metrics"

type Store struct {
	settings *systemsettings.Store
}

func NewStore(settings *systemsettings.Store) *Store {
	return &Store{settings: settings}
}

func (s *Store) GetSettings(ctx context.Context) (GlobalSettings, error) {
	raw, found, err := s.settings.Get(ctx, settingsKey)
	if err != nil {
		return GlobalSettings{}, err
	}
	if !found {
		return DefaultSettings(), nil
	}
	var settings GlobalSettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return GlobalSettings{}, err
	}
	return NormalizeSettings(settings)
}

func (s *Store) SaveSettings(ctx context.Context, settings GlobalSettings) (GlobalSettings, error) {
	normalized, err := NormalizeSettings(settings)
	if err != nil {
		return GlobalSettings{}, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return GlobalSettings{}, err
	}
	if err := s.settings.Save(ctx, settingsKey, data); err != nil {
		return GlobalSettings{}, fmt.Errorf("save metrics settings: %w", err)
	}
	return normalized, nil
}
