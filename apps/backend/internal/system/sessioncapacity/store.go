package sessioncapacity

import (
	"context"
	"encoding/json"
	"fmt"
)

type RawStore interface {
	Get(ctx context.Context, key string) ([]byte, bool, error)
	Save(ctx context.Context, key string, value []byte) error
}

type rawCompareAndSwapStore interface {
	GetConsistent(ctx context.Context, key string) ([]byte, bool, error)
	CompareAndSwap(ctx context.Context, key string, expected, value []byte) (bool, error)
}

type Store struct {
	raw RawStore
}

func NewStore(raw RawStore) *Store {
	return &Store{raw: raw}
}

func (s *Store) Load(ctx context.Context) (*Settings, error) {
	raw, found, err := s.raw.Get(ctx, SettingsKey)
	if err != nil {
		return nil, fmt.Errorf("load session capacity settings: %w", err)
	}
	return decodeSettings(raw, found)
}

func (s *Store) LoadConsistent(ctx context.Context) (*Settings, error) {
	cas, ok := s.raw.(rawCompareAndSwapStore)
	if !ok {
		return s.Load(ctx)
	}
	raw, found, err := cas.GetConsistent(ctx, SettingsKey)
	if err != nil {
		return nil, fmt.Errorf("load session capacity settings: %w", err)
	}
	return decodeSettings(raw, found)
}

func decodeSettings(raw []byte, found bool) (*Settings, error) {
	if !found {
		return nil, nil
	}
	var settings Settings
	if err := json.Unmarshal(raw, &settings); err != nil {
		return nil, fmt.Errorf("%w: decode JSON: %v", ErrInvalidPersisted, err)
	}
	if err := Validate(settings); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPersisted, err)
	}
	return &settings, nil
}

func (s *Store) Save(ctx context.Context, settings Settings) error {
	raw, err := encodeSettings(settings)
	if err != nil {
		return err
	}
	if err := s.raw.Save(ctx, SettingsKey, raw); err != nil {
		return fmt.Errorf("save session capacity settings: %w", err)
	}
	return nil
}

func encodeSettings(settings Settings) ([]byte, error) {
	if err := Validate(settings); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(settings)
	if err != nil {
		return nil, fmt.Errorf("encode session capacity settings: %w", err)
	}
	return raw, nil
}

// Update atomically applies apply when the raw store supports consistent reads
// and compare-and-swap. Stores without those methods use their Get/Save pair.
func (s *Store) Update(
	ctx context.Context,
	apply func(current *Settings) (Settings, error),
) (Settings, error) {
	if apply == nil {
		return Settings{}, fmt.Errorf("%w: update function is required", ErrValidation)
	}
	cas, ok := s.raw.(rawCompareAndSwapStore)
	if !ok {
		return s.updateWithoutCompareAndSwap(ctx, apply)
	}
	return s.updateWithCompareAndSwap(ctx, cas, apply)
}

func (s *Store) updateWithoutCompareAndSwap(
	ctx context.Context,
	apply func(current *Settings) (Settings, error),
) (Settings, error) {
	current, err := s.Load(ctx)
	if err != nil {
		return Settings{}, err
	}
	updated, err := apply(current)
	if err != nil {
		return Settings{}, err
	}
	return updated, s.Save(ctx, updated)
}

func (s *Store) updateWithCompareAndSwap(
	ctx context.Context,
	cas rawCompareAndSwapStore,
	apply func(current *Settings) (Settings, error),
) (Settings, error) {
	for {
		raw, found, err := cas.GetConsistent(ctx, SettingsKey)
		if err != nil {
			return Settings{}, fmt.Errorf("load session capacity settings: %w", err)
		}
		current, err := decodeSettings(raw, found)
		if err != nil {
			return Settings{}, err
		}
		updated, err := apply(current)
		if err != nil {
			return Settings{}, err
		}
		next, err := encodeSettings(updated)
		if err != nil {
			return Settings{}, err
		}
		var expected []byte
		if found {
			expected = raw
		}
		swapped, err := cas.CompareAndSwap(ctx, SettingsKey, expected, next)
		if err != nil {
			return Settings{}, fmt.Errorf("save session capacity settings: %w", err)
		}
		if swapped {
			return updated, nil
		}
		if err := ctx.Err(); err != nil {
			return Settings{}, err
		}
	}
}
