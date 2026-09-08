package plugins

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/plugins/manifest"
	"github.com/kandev/kandev/internal/plugins/store"
)

// Enable transitions id to StatusActive, spawning its process first if it
// is not already running. Idempotent: a no-op (nil error) if id is already
// active.
func (s *Service) Enable(id string) error {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	dispatchLock := s.dispatchLocks.lockFor(id)
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	if rec.Status == StatusActive {
		return nil
	}
	if err := s.activate(rec); err != nil {
		return err
	}
	s.notifyDeliverer()
	s.notifyAgentToolCatalogChanged()
	return nil
}

// Disable stops id's process (if running) and transitions it to
// StatusDisabled. Idempotent: a no-op (nil error) if id is already
// disabled.
func (s *Service) Disable(id string) error {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()
	dispatchLock := s.dispatchLocks.lockFor(id)
	dispatchLock.Lock()
	defer dispatchLock.Unlock()

	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	if rec.Status == StatusDisabled {
		return nil
	}
	if s.runtime != nil {
		s.runtime.Stop(id)
	}
	if err := s.SetStatus(id, StatusDisabled); err != nil {
		return err
	}
	s.notifyDeliverer()
	s.notifyAgentToolCatalogChanged()
	return nil
}

// activateStartTimeout bounds the context activate hands to runtime.Start,
// so a hung plugin binary cannot block Enable/Install indefinitely. The
// runtime.Manager itself also enforces a startTimeout on the underlying
// go-plugin handshake (the actual blocking call is not context-aware); this
// context bound is defense-in-depth and gives Start a chance to short-circuit
// on ctx.Err() before ever spawning.
const activateStartTimeout = 30 * time.Second

// activate spawns rec's process (if not already running) and transitions it
// to StatusActive. If the spawn fails, it records the failure and transitions
// the record to StatusError before returning the spawn error.
func (s *Service) activate(rec *store.Record) error {
	s.ownershipMu.Lock()
	defer s.ownershipMu.Unlock()
	if err := s.ensureOwnershipAvailable(&rec.Manifest); err != nil {
		return err
	}
	if s.runtime != nil && !s.runtime.Running(rec.ID) {
		ctx, cancel := context.WithTimeout(context.Background(), activateStartTimeout)
		defer cancel()
		if err := s.runtime.Start(ctx, rec, s.hostForPlugin); err != nil {
			if setErr := s.setStatusAndDiagnostic(rec.ID, StatusError, err, true); setErr != nil {
				s.log.Warn("plugins: could not persist activation failure",
					zap.String("plugin_id", rec.ID), zap.Error(setErr))
			}
			return fmt.Errorf("plugins: start %q: %w", rec.ID, err)
		}
	}
	return s.setStatus(rec.ID, StatusActive)
}

// ensureOwnershipAvailable rejects duplicate ownership among active plugins.
// Disabled plugins release ownership immediately; an in-place upgrade excludes
// its previous record by plugin ID.
func (s *Service) ensureOwnershipAvailable(candidate *manifest.Manifest) error {
	for _, provider := range candidate.RepositoryProviders {
		if owner, found := s.registry.activeRepositoryProviderOwner(provider, candidate.ID); found {
			return fmt.Errorf("plugins: repository provider %q is already owned by active plugin %q", provider, owner)
		}
	}
	for _, source := range candidate.ReferenceSources {
		s.mu.Lock()
		_, reservedSource := s.reservedReferenceSources[source.Source]
		_, reservedProviderKind := s.reservedReferenceProviderKinds[source.Provider+"\x00"+source.Kind]
		s.mu.Unlock()
		if reservedSource {
			return fmt.Errorf("plugins: reference source %q is owned by the host", source.Source)
		}
		if reservedProviderKind {
			return fmt.Errorf("plugins: reference provider and kind %q/%q is owned by the host", source.Provider, source.Kind)
		}
		if owner, found := s.registry.activeReferenceSourceOwner(source.Source, candidate.ID); found {
			return fmt.Errorf("plugins: reference source %q is already owned by active plugin %q", source.Source, owner)
		}
		if owner, found := s.registry.activeReferenceProviderKindOwner(source.Provider, source.Kind, candidate.ID); found {
			return fmt.Errorf("plugins: reference provider and kind %q/%q is already owned by active plugin %q", source.Provider, source.Kind, owner)
		}
	}
	return nil
}

// SetStatus applies a single-hop status transition for id, enforcing the
// state machine (allowedTransitions in types.go). On success the change is
// persisted to the store and applied to the in-memory registry. Returns
// *ErrInvalidTransition without mutating anything if the transition is not
// legal, and store.ErrNotFound if id is not installed. Callers that need
// the attached Deliverer notified (most of them) call notifyDeliverer
// separately — SetStatus itself does not, since activate/Disable call it
// both for the runtime spawn/stop and the status transition, and only want
// a single Refresh for the whole operation.
func (s *Service) SetStatus(id string, status Status) error {
	if status == StatusActive {
		s.ownershipMu.Lock()
		defer s.ownershipMu.Unlock()
		rec, err := s.Get(id)
		if err != nil {
			return err
		}
		if err := s.ensureOwnershipAvailable(&rec.Manifest); err != nil {
			return err
		}
	}
	return s.setStatusAndDiagnostic(id, status, nil, false)
}

func (s *Service) setStatus(id string, status Status) error {
	return s.setStatusAndDiagnostic(id, status, nil, false)
}

// setStatusAndDiagnostic applies a lifecycle transition together with its
// persisted runtime diagnostic. allowSame is used only for repeated failure
// reports (for example, a failed retry while the record is already in error);
// public SetStatus keeps same-state transitions invalid.
func (s *Service) setStatusAndDiagnostic(id string, status Status, failure error, allowSame bool) error {
	s.mu.Lock()

	rec, ok := s.registry.Get(id)
	if !ok {
		s.mu.Unlock()
		return store.ErrNotFound
	}
	if rec.Status == status {
		if !allowSame {
			s.mu.Unlock()
			return &ErrInvalidTransition{ID: id, From: rec.Status, To: status}
		}
	} else if !canTransition(rec.Status, status) {
		s.mu.Unlock()
		return &ErrInvalidTransition{ID: id, From: rec.Status, To: status}
	}

	lastError := rec.LastError
	lastErrorAt := rec.LastErrorAt
	if status == StatusActive {
		lastError = ""
		lastErrorAt = nil
	}
	if failure != nil {
		lastError = normalizePluginError(failure)
		now := time.Now().UTC()
		lastErrorAt = &now
	}

	updated, ok := s.registry.SetRuntimeState(id, status, lastError, lastErrorAt)
	if !ok {
		s.mu.Unlock()
		return store.ErrNotFound
	}
	if err := s.store.Save(updated); err != nil {
		// Roll back the in-memory change so registry and disk stay in sync.
		s.registry.Add(rec)
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	if status != StatusActive {
		s.revokeGitCredentialProviderLeases(updated.RepositoryProviders)
	}
	return nil
}

// handleStatusChange is the runtime.Manager OnStatusChange callback (see
// Provide, where it is bound as a Manager constructor argument): invoked
// from the supervision loop's own goroutine whenever a running plugin's
// health transitions. healthy=false drives active -> error; healthy=true
// drives error -> active plus a Deliverer.Flush (the buffered-event
// recovery replay). Restart count is persisted best-effort afterward.
func (s *Service) handleStatusChange(id string, healthy bool, reason error) {
	newStatus := StatusError
	if healthy {
		newStatus = StatusActive
	}
	var err error
	if healthy {
		s.ownershipMu.Lock()
		rec, getErr := s.Get(id)
		err = getErr
		if err == nil {
			err = s.ensureOwnershipAvailable(&rec.Manifest)
		}
		if err == nil {
			err = s.setStatusAndDiagnostic(id, newStatus, reason, false)
		}
		s.ownershipMu.Unlock()
	} else {
		err = s.setStatusAndDiagnostic(id, newStatus, reason, true)
	}
	if err != nil {
		s.log.Warn("plugins: health transition failed",
			zap.String("plugin_id", id), zap.Bool("healthy", healthy), zap.Error(err))
	} else {
		s.notifyDeliverer()
		s.notifyAgentToolCatalogChanged()
		if healthy {
			if d := s.Deliverer(); d != nil {
				d.Flush(id)
			}
		}
	}
	s.recordRestartCount(id)
}

// recordRestartCount best-effort persists the runtime manager's current
// restart count for id onto its store.Record.
func (s *Service) recordRestartCount(id string) {
	if s.runtime == nil {
		return
	}
	// Serialize this metadata write with lifecycle/diagnostic persistence so a
	// restart callback cannot save a stale record over a concurrent Enable or
	// recovery that just cleared LastError.
	s.mu.Lock()
	defer s.mu.Unlock()
	updated, ok := s.registry.SetRestartCount(id, s.runtime.RestartCount(id))
	if !ok {
		return
	}
	if err := s.store.Save(updated); err != nil {
		s.log.Warn("plugins: persist restart count failed", zap.String("plugin_id", id), zap.Error(err))
	}
}

// StartActivePlugins runs the conservative boot filesystem scan (dir
// sideloads registered disabled, missing-install detection — see
// service_sync.go's bootScan) and then spawns every currently-StatusActive,
// runtime-managed plugin's process. Called once at boot (backendapp's
// startPluginsSubsystems) so plugins that were active before a restart
// resume running. A spawn failure is logged and the plugin transitions to
// StatusError rather than aborting the rest of the boot sequence.
func (s *Service) StartActivePlugins(ctx context.Context) {
	s.logBootScanResult(s.bootScan(ctx))

	if s.runtime == nil {
		return
	}
	for _, rec := range s.List() {
		if rec.Status != StatusActive || !rec.IsManaged() || s.runtime.Running(rec.ID) {
			continue
		}
		if err := s.ensureOwnershipAvailable(&rec.Manifest); err != nil {
			s.log.Warn("plugins: active ownership collision", zap.String("plugin_id", rec.ID), zap.Error(err))
			if statusErr := s.SetStatus(rec.ID, StatusError); statusErr == nil {
				s.notifyAgentToolCatalogChanged()
			}
			continue
		}
		if err := s.runtime.Start(ctx, rec, s.hostForPlugin); err != nil {
			s.log.Warn("plugins: failed to spawn active plugin at boot",
				zap.String("plugin_id", rec.ID), zap.Error(err))
			if setErr := s.setStatusAndDiagnostic(rec.ID, StatusError, err, true); setErr != nil {
				s.log.Warn("plugins: could not persist boot activation failure",
					zap.String("plugin_id", rec.ID), zap.Error(setErr))
			} else {
				// The deliverer was refreshed before boot activation began, so
				// reconcile it after an active plugin fails to spawn. Otherwise
				// its worker would continue treating the plugin as active.
				s.notifyDeliverer()
				s.notifyAgentToolCatalogChanged()
			}
			continue
		}
		// Boot is the only trigger that reaches an instance which already
		// accumulated superseded versions but installs nothing new (a plugin
		// already at its latest version, or one whose auto-update is off), so
		// it prunes too — under the same confirmed-start precondition as
		// Install, which is what keeps disabled plugins, sideloads registered
		// disabled, and plugins that fail to spawn untouched. Unlike Install,
		// this loop holds no lifecycle lock, so the prune takes one itself.
		s.pruneUnderLifecycleLock(rec.ID, rec.Version, "")
	}
}

// logBootScanResult logs what the boot filesystem scan found, if anything —
// a silent no-op scan (the common case) logs nothing.
func (s *Service) logBootScanResult(result *SyncResult) {
	if result == nil || (len(result.Added) == 0 && len(result.Missing) == 0 && len(result.Errors) == 0) {
		return
	}
	s.log.Info("plugins: boot filesystem scan found changes",
		zap.Strings("sideloaded", result.Added),
		zap.Strings("missing", result.Missing),
		zap.Int("errors", len(result.Errors)))
	for _, e := range result.Errors {
		s.log.Warn("plugins: boot scan error", zap.String("path", e.Path), zap.String("reason", e.Reason))
	}
}
