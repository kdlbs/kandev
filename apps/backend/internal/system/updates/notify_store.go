package updates

import (
	"context"
	"encoding/json"
	"fmt"

	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

const notifySettingsKey = "update_notifications"

// NotifyStore persists NotifySettings on top of the generic install-wide
// settings table (see internal/system/settings), following the same
// shape as metrics.Store.
type NotifyStore struct {
	settings *systemsettings.Store
}

func NewNotifyStore(settings *systemsettings.Store) *NotifyStore {
	return &NotifyStore{settings: settings}
}

func (s *NotifyStore) GetSettings(ctx context.Context) (NotifySettings, error) {
	raw, found, err := s.settings.Get(ctx, notifySettingsKey)
	if err != nil {
		return NotifySettings{}, err
	}
	if !found {
		return DefaultNotifySettings(), nil
	}
	var settings NotifySettings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return NotifySettings{}, err
	}
	return NormalizeNotifySettings(settings)
}

func (s *NotifyStore) SaveSettings(ctx context.Context, settings NotifySettings) (NotifySettings, error) {
	normalized, err := NormalizeNotifySettings(settings)
	if err != nil {
		return NotifySettings{}, err
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return NotifySettings{}, err
	}
	if err := s.settings.Save(ctx, notifySettingsKey, data); err != nil {
		return NotifySettings{}, fmt.Errorf("save update notification settings: %w", err)
	}
	return normalized, nil
}
