package queuesettings

import (
	"context"
	"encoding/json"
	"errors"
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

// storedSettings mirrors Settings for JSON decoding but keeps both merge
// settings as pointers. Missing keys in records written by older versions
// therefore default on, while explicit false values remain false.
type storedSettings struct {
	MaxPerSession     int   `json:"max_per_session"`
	MergeEnabled      *bool `json:"merge_enabled"`
	AutoMergeEnabled  *bool `json:"auto_merge_enabled"`
	AutoMergeRevision int64 `json:"auto_merge_revision"`
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
		MaxPerSession: s.MaxPerSession, MergeEnabled: mergeEnabled,
		AutoMergeEnabled: autoMergeEnabled, AutoMergeRevision: s.AutoMergeRevision,
	}
}

func (s *Store) Load(ctx context.Context) (*Settings, error) {
	raw, found, err := s.raw.Get(ctx, SettingsKey)
	if err != nil {
		return nil, fmt.Errorf("load message queue settings: %w", err)
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
		return nil, fmt.Errorf("load message queue settings: %w", err)
	}
	return decodeSettings(raw, found)
}

func decodeSettings(raw []byte, found bool) (*Settings, error) {
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
	raw, err := encodeSettings(settings)
	if err != nil {
		return err
	}
	if err := s.raw.Save(ctx, SettingsKey, raw); err != nil {
		return fmt.Errorf("save message queue settings: %w", err)
	}
	return nil
}

func encodeSettings(settings Settings) ([]byte, error) {
	if err := Validate(settings); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(storedSettings{
		MaxPerSession: settings.MaxPerSession, MergeEnabled: &settings.MergeEnabled,
		AutoMergeEnabled: &settings.AutoMergeEnabled, AutoMergeRevision: settings.AutoMergeRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("encode message queue settings: %w", err)
	}
	return raw, nil
}

func (s *Store) Update(
	ctx context.Context,
	apply func(current *Settings) (Settings, error),
) (Settings, error) {
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
	current, err := settingsOrUnset(s.Load(ctx))
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
			return Settings{}, fmt.Errorf("load message queue settings: %w", err)
		}
		current, err := settingsOrUnset(decodeSettings(raw, found))
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
			return Settings{}, fmt.Errorf("save message queue settings: %w", err)
		}
		if swapped {
			return updated, nil
		}
		if err := ctx.Err(); err != nil {
			return Settings{}, err
		}
	}
}

func settingsOrUnset(settings *Settings, err error) (*Settings, error) {
	if errors.Is(err, ErrInvalidPersisted) {
		return nil, nil
	}
	return settings, err
}
