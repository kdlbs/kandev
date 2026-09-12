package retention

import (
	"context"
	"encoding/json"
	"fmt"

	systemsettings "github.com/kandev/kandev/internal/system/settings"
)

// settingsKey is the internal/system/settings.Store key holding the
// retention policy document. The live key/value table is "settings",
// reached through settings.Store; system_settings is a legacy SQLite-only
// table read once for migration and is absent on PostgreSQL.
const settingsKey = "office_run_retention"

// SettingsStore persists and reads the retention policy document.
type SettingsStore struct {
	store *systemsettings.Store
}

// NewSettingsStore wraps the shared key/value settings store.
func NewSettingsStore(store *systemsettings.Store) *SettingsStore {
	return &SettingsStore{store: store}
}

// GetSettings reads the retention policy for reporting and startup use. An
// unreadable or unparseable document falls back to DefaultSettings and
// wraps ErrInvalidPersistedSettings so the caller can raise a health issue;
// it never fails outright (AC-OFFICE-RUN-HISTORY-RETENTION-004.4).
func (s *SettingsStore) GetSettings(ctx context.Context) (Settings, error) {
	raw, found, err := s.store.Get(ctx, settingsKey)
	if err != nil {
		return DefaultSettings(), fmt.Errorf("%w: %w", ErrInvalidPersistedSettings, err)
	}
	if !found {
		return DefaultSettings(), nil
	}
	return decodeAndNormalize(raw, DefaultSettings())
}

// GetSettingsForSweep reads the retention policy on the writer pool at
// sweep start, per AC-OFFICE-RUN-HISTORY-RETENTION-004.5: a sweep must read
// the stored settings at its start rather than a value cached from a
// notification, so a backend that did not serve the write still sweeps
// under the new policy. Unlike GetSettings, a read or parse failure here
// returns ErrInvalidPersistedSettings with no usable Settings value — the
// caller must skip the sweep rather than fall back to the (possibly
// shorter) default window and delete history the operator configured the
// system to keep. A document that was never saved is the legitimate empty
// state, not a failure, and yields the defaults.
func (s *SettingsStore) GetSettingsForSweep(ctx context.Context) (Settings, error) {
	raw, found, err := s.store.GetConsistent(ctx, settingsKey)
	if err != nil {
		return Settings{}, fmt.Errorf("%w: %w", ErrInvalidPersistedSettings, err)
	}
	if !found {
		return DefaultSettings(), nil
	}
	return decodeAndNormalize(raw, Settings{})
}

func decodeAndNormalize(raw []byte, fallback Settings) (Settings, error) {
	var doc Settings
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fallback, fmt.Errorf("%w: decode JSON: %w", ErrInvalidPersistedSettings, err)
	}
	normalized, err := NormalizeSettings(doc)
	if err != nil {
		return fallback, fmt.Errorf("%w: %w", ErrInvalidPersistedSettings, err)
	}
	return normalized, nil
}

// SaveSettings normalizes and persists a full settings document, replacing
// whatever was stored (AC-OFFICE-RUN-HISTORY-RETENTION-004.9). A rejected
// write leaves the stored document unchanged.
func (s *SettingsStore) SaveSettings(ctx context.Context, in Settings) (Settings, error) {
	normalized, err := NormalizeSettings(in)
	if err != nil {
		return Settings{}, err
	}
	raw, err := json.Marshal(normalized)
	if err != nil {
		return Settings{}, fmt.Errorf("encode retention settings: %w", err)
	}
	if err := s.store.Save(ctx, settingsKey, raw); err != nil {
		return Settings{}, err
	}
	return normalized, nil
}
