package queuesettings

import (
	"context"
	"encoding/json"
	"fmt"
)

type RawStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Save(ctx context.Context, key string, value []byte) error
}

type Store struct {
	raw RawStore
}

func NewStore(raw RawStore) *Store {
	return &Store{raw: raw}
}

// storedSettings mirrors Settings for JSON decoding but keeps both merge
// settings as pointers. Missing keys in records written by older versions
// therefore default on, while explicit false values remain false.
type storedSettings struct {
	MaxPerSession    int   `json:"max_per_session"`
	MergeEnabled     *bool `json:"merge_enabled"`
	AutoMergeEnabled *bool `json:"auto_merge_enabled"`
}

// toSettings normalizes a decoded record, defaulting omitted merge settings on.
func (s storedSettings) toSettings() Settings {
	mergeEnabled := true
	if s.MergeEnabled != nil {
		mergeEnabled = *s.MergeEnabled
	}
	autoMergeEnabled := true
	if s.AutoMergeEnabled != nil {
		autoMergeEnabled = *s.AutoMergeEnabled
	}
	return Settings{
		MaxPerSession: s.MaxPerSession, MergeEnabled: mergeEnabled, AutoMergeEnabled: autoMergeEnabled,
	}
}

func (s *Store) Load(ctx context.Context) (*Settings, error) {
	raw, found, err := s.raw.Get(ctx, SettingsKey)
	if err != nil {
		return nil, fmt.Errorf("load message queue settings: %w", err)
	}
	if !found {
		return nil, nil
	}
	var stored storedSettings
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, fmt.Errorf("%w: decode JSON: %v", ErrInvalidPersisted, err)
	}
	settings := stored.toSettings()
	if err := Validate(settings); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPersisted, err)
	}
	return &settings, nil
}

func (s *Store) Save(ctx context.Context, settings Settings) error {
	if err := Validate(settings); err != nil {
		return err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode message queue settings: %w", err)
	}
	if err := s.raw.Save(ctx, SettingsKey, raw); err != nil {
		return fmt.Errorf("save message queue settings: %w", err)
	}
	return nil
}
