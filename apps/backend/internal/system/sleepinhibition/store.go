package sleepinhibition

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

func NewStore(raw RawStore) *Store { return &Store{raw: raw} }

func (s *Store) Load(ctx context.Context) (Settings, error) {
	raw, found, err := s.raw.Get(ctx, SettingsKey)
	if err != nil {
		return Settings{}, fmt.Errorf("load task sleep inhibition settings: %w", err)
	}
	if !found {
		return Settings{}, nil
	}
	var settings Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return Settings{}, fmt.Errorf("%w: decode JSON: %v", ErrInvalidPersisted, err)
	}
	return settings, nil
}

func (s *Store) Save(ctx context.Context, settings Settings) error {
	raw, err := json.Marshal(settings)
	if err != nil {
		return fmt.Errorf("encode task sleep inhibition settings: %w", err)
	}
	if err := s.raw.Save(ctx, SettingsKey, raw); err != nil {
		return fmt.Errorf("save task sleep inhibition settings: %w", err)
	}
	return nil
}
