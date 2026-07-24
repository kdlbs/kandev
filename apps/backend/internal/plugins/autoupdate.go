package plugins

import (
	"context"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/store"
)

// SetSettings attaches the instance-wide plugin settings store. Called by
// Provide after the store is built; the auto-update accessors treat a nil
// store as "default off". Safe to call once during startup wiring.
func (s *Service) SetSettings(st *settingsStore) {
	s.mu.Lock()
	s.settings = st
	s.mu.Unlock()
}

// AutoUpdateDefault returns the instance-wide auto-update default, applied to
// every plugin that has no per-plugin override. Returns false when no settings
// store is attached (auto-update is strictly opt-in).
func (s *Service) AutoUpdateDefault() (bool, error) {
	s.mu.Lock()
	st := s.settings
	s.mu.Unlock()
	if st == nil {
		return false, nil
	}
	set, err := st.Get()
	if err != nil {
		return false, err
	}
	return set.AutoUpdateDefault, nil
}

// SetAutoUpdateDefault persists the instance-wide auto-update default. A nil
// settings store (marketplace/settings init failed) returns an error so the
// operator is not told a preference was saved when it was not.
func (s *Service) SetAutoUpdateDefault(enabled bool) error {
	s.mu.Lock()
	st := s.settings
	s.mu.Unlock()
	if st == nil {
		return fmt.Errorf("plugins: settings store unavailable")
	}
	return st.SetAutoUpdateDefault(enabled)
}

// SetPluginAutoUpdate sets (or clears) a plugin's per-plugin auto-update
// override and persists it. v is nil to clear the override so the plugin
// inherits the instance-wide default again. Returns store.ErrNotFound if id is
// not installed. It takes the plugin's lifecycle lock so the write is
// serialized against an in-flight Install upgrade (which carries the override
// forward) and against Enable/Disable/Uninstall.
func (s *Service) SetPluginAutoUpdate(id string, v *bool) (*store.Record, error) {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

	s.mu.Lock()
	defer s.mu.Unlock()

	prev, ok := s.registry.Get(id)
	if !ok {
		return nil, store.ErrNotFound
	}
	updated, ok := s.registry.SetAutoUpdate(id, v)
	if !ok {
		return nil, store.ErrNotFound
	}
	if err := s.store.Save(updated); err != nil {
		// Keep the in-memory registry and disk in sync: restore the prior
		// override if persistence failed.
		s.registry.SetAutoUpdate(id, prev.AutoUpdate)
		return nil, fmt.Errorf("plugins: persist auto-update override: %w", err)
	}
	return updated, nil
}

// effectiveAutoUpdate resolves a plugin's effective auto-update setting: the
// per-plugin override when set, otherwise the instance-wide default.
func effectiveAutoUpdate(override *bool, def bool) bool {
	if override != nil {
		return *override
	}
	return def
}

// AutoUpdateOutcome is the result of one auto-update pass.
type AutoUpdateOutcome struct {
	// Updated lists plugins that were upgraded to a newer version this pass.
	Updated []AutoUpdateChange `json:"updated"`
	// Failed lists plugins that had an update available but whose reinstall
	// failed (surfaced for logging/observability; the pass never aborts on a
	// single failure).
	Failed []AutoUpdateFailure `json:"failed"`
}

// AutoUpdateChange records a single successful in-place upgrade.
type AutoUpdateChange struct {
	ID   string `json:"id"`
	From string `json:"from"`
	To   string `json:"to"`
}

// AutoUpdateFailure records a single failed upgrade attempt.
type AutoUpdateFailure struct {
	ID    string `json:"id"`
	To    string `json:"to"`
	Error string `json:"error"`
}

// RunAutoUpdatePass performs one auto-update sweep: it upgrades every currently
// active, opted-in plugin for which the marketplace catalog reports a newer
// version. Disabled/errored plugins are intentionally skipped — a plugin only
// auto-updates while it is running (see the plugin auto-updater spec), so a
// disabled plugin stays on its installed version until re-enabled. The catalog
// is fetched only when at least one active plugin is opted in, so a deployment
// with auto-update entirely off never reaches out to any marketplace source.
//
// It is safe to call directly (tests drive it without the poller) and returns
// the outcome for logging; a nil marketplace (discovery unavailable) is a
// no-op, not an error.
func (s *Service) RunAutoUpdatePass(ctx context.Context) (AutoUpdateOutcome, error) {
	m := s.Marketplace()
	if m == nil {
		return AutoUpdateOutcome{}, nil
	}
	def, err := s.AutoUpdateDefault()
	if err != nil {
		return AutoUpdateOutcome{}, fmt.Errorf("plugins: auto-update read default: %w", err)
	}

	optedIn := s.autoUpdateCandidates(def)
	if len(optedIn) == 0 {
		return AutoUpdateOutcome{}, nil
	}

	catalog, err := m.Catalog(ctx, s.InstalledForMarketplace())
	if err != nil {
		return AutoUpdateOutcome{}, fmt.Errorf("plugins: auto-update catalog fetch: %w", err)
	}
	return s.applyAutoUpdates(ctx, catalog.Plugins), nil
}

// autoUpdateCandidates returns the set of active, opted-in plugin ids eligible
// for auto-update this pass, given the resolved instance-wide default.
func (s *Service) autoUpdateCandidates(def bool) map[string]bool {
	out := map[string]bool{}
	for _, rec := range s.List() {
		if rec.Status != StatusActive {
			continue
		}
		if effectiveAutoUpdate(rec.AutoUpdate, def) {
			out[rec.ID] = true
		}
	}
	return out
}

// applyAutoUpdates reinstalls every catalog entry that reports an available
// update and is still eligible at install time, stopping early if ctx is
// cancelled (e.g. backend shutdown mid-pass).
func (s *Service) applyAutoUpdates(ctx context.Context, entries []marketplace.CatalogEntry) AutoUpdateOutcome {
	var outcome AutoUpdateOutcome
	for _, entry := range entries {
		if ctx.Err() != nil {
			return outcome
		}
		if entry.InstallState != marketplace.StateUpdateAvailable || entry.PackageURL == "" {
			continue
		}
		// Re-check eligibility immediately before installing. The candidate
		// snapshot and the catalog fetch above can lag seconds behind operator
		// actions (disable, uninstall, or flipping the toggle off), so a stale
		// entry must not reach InstallFromURL — Install unconditionally
		// re-activates, and reactivating a plugin the operator just disabled
		// would violate the active-and-opted-in-only contract.
		if !s.eligibleForAutoUpdate(entry.ID) {
			continue
		}
		from := entry.InstalledVersion
		if _, err := s.InstallFromURL(ctx, entry.PackageURL); err != nil {
			outcome.Failed = append(outcome.Failed, AutoUpdateFailure{ID: entry.ID, To: entry.Version, Error: err.Error()})
			s.log.Warn("plugins: auto-update failed",
				zap.String("plugin_id", entry.ID), zap.String("from", from),
				zap.String("to", entry.Version), zap.Error(err))
			continue
		}
		outcome.Updated = append(outcome.Updated, AutoUpdateChange{ID: entry.ID, From: from, To: entry.Version})
		s.log.Info("plugins: auto-updated plugin",
			zap.String("plugin_id", entry.ID), zap.String("from", from), zap.String("to", entry.Version))
	}
	return outcome
}

// eligibleForAutoUpdate reports whether id is, right now, an installed active
// plugin that is effectively opted in to auto-update. It is the authoritative
// gate applied immediately before an auto-update install (see applyAutoUpdates),
// closing the window between the candidate snapshot / catalog fetch and the
// install during which an operator may have disabled, uninstalled, or opted the
// plugin out. Read without holding the lifecycle lock deliberately: the only
// remaining race is the sub-millisecond gap before InstallFromURL's own
// download begins, and holding the lock across a multi-second package download
// would needlessly block concurrent operator actions.
func (s *Service) eligibleForAutoUpdate(id string) bool {
	def, err := s.AutoUpdateDefault()
	if err != nil {
		return false
	}
	rec, ok := s.registry.Get(id)
	if !ok {
		return false
	}
	return rec.Status == StatusActive && effectiveAutoUpdate(rec.AutoUpdate, def)
}
