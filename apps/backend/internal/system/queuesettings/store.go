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

// storedSettings mirrors Settings for JSON decoding but keeps MergeEnabled as
// a pointer so a missing key ("unset" — a record saved before this field
// existed) can be told apart from an explicit `false`. Without this, an
// installation that persisted max_per_session before merge_enabled shipped
// would decode its stale record as merge_enabled=false and silently disable
// merging on upgrade instead of leaving it enabled by default.
type storedSettings struct {
	MaxPerSession int   `json:"max_per_session"`
	MergeEnabled  *bool `json:"merge_enabled"`
}

// toSettings normalizes a decoded record into Settings, defaulting
// MergeEnabled to true when the persisted JSON has no merge_enabled key.
func (s storedSettings) toSettings() Settings {
	mergeEnabled := true
	if s.MergeEnabled != nil {
		mergeEnabled = *s.MergeEnabled
	}
	return Settings{MaxPerSession: s.MaxPerSession, MergeEnabled: mergeEnabled}
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
