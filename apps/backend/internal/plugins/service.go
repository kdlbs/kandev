package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/pkgtar"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

// maxDownloadSize caps the response body InstallFromURL will read, per the
// task's build instructions (100MB cap).
const maxDownloadSize = 100 << 20

// downloadTimeout bounds how long InstallFromURL waits for the whole
// download.
const downloadTimeout = 60 * time.Second

// Service is the core plugin service: install/uninstall, the in-memory
// Registry, the lifecycle state machine, and the runtime.Manager wiring
// that spawns/supervises each plugin's subprocess.
//
// # Extension points
//
// Event delivery (internal/plugins/delivery) is wired in by backendapp
// after Provide, following the same post-construction "SetX" pattern
// internal/jira/service.go uses for SetTaskDeleter / SetRepositoryLookup
// (avoids an import cycle between this package and its siblings):
//
//   - SetDeliverer(d Deliverer) attaches the event-delivery subsystem.
//     Install, Uninstall, Enable, Disable, and any successful SetStatus
//     call notify the attached Deliverer via Refresh() so it can
//     re-subscribe to the event bus based on current registry state.
//   - StateStore() exposes the already-constructed *state.Store so the
//     HTTP layer doesn't need a second NewStore(pool) call.
//   - Registry() and EventBus() are exposed for any other read-only wiring
//     (e.g. proxies checking a plugin's manifest/capabilities without
//     going through Service's error-wrapping Get).
type Service struct {
	mu sync.Mutex

	// syncMu serializes Sync/bootScan calls (service_sync.go) so concurrent
	// operator clicks — or a boot scan racing an operator-triggered sync —
	// cannot double-install the same dropped tarball or dir sideload.
	syncMu sync.Mutex

	pluginsDir string
	store      store.Store
	registry   *Registry
	state      *state.Store
	eventBus   bus.EventBus
	log        *logger.Logger

	deliverer Deliverer
	runtime   PluginRuntime
	secrets   SecretRevealer

	httpClient *http.Client
}

// NewService wires a Service from its already-constructed dependencies.
// Provide is the usual entry point in production; NewService is exposed
// directly for tests that want a fake store.Store/PluginRuntime.
func NewService(pluginStore store.Store, registry *Registry, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		store:      pluginStore,
		registry:   registry,
		eventBus:   eventBus,
		log:        log,
		httpClient: &http.Client{},
	}
}

// SetDeliverer attaches the event-delivery subsystem. See the "Extension
// points" doc comment on Service. Safe to call at most once during startup
// wiring; not safe to call concurrently with Install/SetStatus/Uninstall.
func (s *Service) SetDeliverer(d Deliverer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deliverer = d
}

// Deliverer returns the currently attached event-delivery subsystem, or nil
// if none has been attached yet.
func (s *Service) Deliverer() Deliverer {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.deliverer
}

// SetState wires the already-constructed plugin_state store. Provide calls
// this; also exposed for tests (in this package and others, e.g.
// internal/backendapp) that build a Service without going through Provide.
func (s *Service) SetState(st *state.Store) {
	s.state = st
}

// StateStore returns the plugin_state store Provide constructed, for the
// Host RPC implementation (host.go) and any HTTP wiring that needs it
// without re-initializing the schema.
func (s *Service) StateStore() *state.Store {
	return s.state
}

// SetSecrets wires the secret revealer Provide was constructed with.
func (s *Service) SetSecrets(v SecretRevealer) {
	s.secrets = v
}

// SetRuntime wires the runtime.Manager Provide constructed.
func (s *Service) SetRuntime(rt PluginRuntime) {
	s.runtime = rt
}

// Runtime returns the runtime manager Service spawns/supervises plugin
// processes through, for boot-time wiring (spawning every active plugin)
// and the HTTP layer (webhook/tool invocation).
func (s *Service) Runtime() PluginRuntime {
	return s.runtime
}

// Shutdown stops every currently-running plugin process. Callers (e.g.
// backendapp's startPluginsSubsystems) register this with addCleanup for
// graceful backend shutdown.
func (s *Service) Shutdown() {
	if s.runtime != nil {
		s.runtime.StopAll()
	}
}

// SetPluginsDir wires the root directory pkgtar.Install/pkgtar.Remove
// operate under (the same directory store.FSStore persists records in).
func (s *Service) SetPluginsDir(dir string) {
	s.pluginsDir = dir
}

// RevealSecret resolves the cleartext value of the secret reference ref via
// the shared secret vault. Returns an error if no vault was wired (e.g. a
// test Service constructed via NewService directly) or if ref does not
// resolve.
func (s *Service) RevealSecret(ctx context.Context, ref string) (string, error) {
	if s.secrets == nil {
		return "", errors.New("plugins: secret vault not configured")
	}
	return s.secrets.Reveal(ctx, ref)
}

// ActiveUIPlugins returns every StatusActive plugin record that declares a
// native UI bundle (ui.bundle), used to populate the boot payload's Plugins
// list.
func (s *Service) ActiveUIPlugins() []store.Record {
	var out []store.Record
	for _, rec := range s.List() {
		if rec.Status == StatusActive && rec.UI.Bundle != "" {
			out = append(out, *rec)
		}
	}
	return out
}

// Registry returns the underlying in-memory Registry.
func (s *Service) Registry() *Registry {
	return s.registry
}

// EventBus returns the event bus Service was constructed with (may be nil
// in tests).
func (s *Service) EventBus() bus.EventBus {
	return s.eventBus
}

// hostForPlugin builds the Host implementation bound to pluginID, gated by
// that plugin's currently-registered capabilities. Passed to
// PluginRuntime.Start as the hostFactory; the runtime manager calls it
// again on every restart, so a config/capability change takes effect on
// the plugin's next spawn.
func (s *Service) hostForPlugin(pluginID string) pluginsdk.Host {
	rec, err := s.Get(pluginID)
	if err != nil {
		rec = &store.Record{} // every capability check below denies; should not happen in practice
	}
	return &pluginHost{
		pluginID:     pluginID,
		capabilities: rec.Capabilities,
		state:        s.state,
		secrets:      s.secrets,
		bus:          s.eventBus,
	}
}

// notifyDeliverer calls Refresh on the attached Deliverer, if any. Must be
// called without s.mu held (Deliverer implementations may call back into
// Service).
func (s *Service) notifyDeliverer() {
	s.mu.Lock()
	d := s.deliverer
	s.mu.Unlock()
	if d != nil {
		d.Refresh()
	}
}

// List returns every installed plugin, sorted by id.
func (s *Service) List() []*store.Record {
	return s.registry.List()
}

// Get returns the record for id, or store.ErrNotFound.
func (s *Service) Get(id string) (*store.Record, error) {
	rec, ok := s.registry.Get(id)
	if !ok {
		return nil, store.ErrNotFound
	}
	return rec, nil
}

// UpdateConfig replaces the operator-editable config for id.
func (s *Service) UpdateConfig(id string, config map[string]any) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	return s.store.SetConfig(id, config)
}

// Install verifies and extracts r (a tar.gz plugin package) via pkgtar into
// the plugins directory, persists a fresh store.Record (status
// "registered"), adds it to the in-memory registry, and attempts to spawn
// and activate it. A pkgtar error (e.g. pkgtar.ErrVersionExists) is
// returned unchanged so callers can map it to the right HTTP status. If the
// package is valid but the initial spawn fails, the record is still
// persisted (status "error") and returned alongside the spawn error, so an
// operator can fix the issue and retry via Enable.
func (s *Service) Install(ctx context.Context, r io.Reader) (*store.Record, error) {
	result, err := pkgtar.Install(r, s.pluginsDir)
	if err != nil {
		return nil, err
	}

	rec := &store.Record{
		Manifest:    *result.Manifest,
		Status:      StatusRegistered,
		InstallPath: result.InstallPath,
		Signed:      result.Signed,
		InstalledAt: time.Now().UTC(),
	}
	if err := s.store.Save(rec); err != nil {
		_ = pkgtar.Remove(s.pluginsDir, rec.ID)
		return nil, fmt.Errorf("plugins: persist installed record: %w", err)
	}
	s.registry.Add(rec)

	activateErr := s.activate(rec)
	s.notifyDeliverer()

	installed, getErr := s.Get(rec.ID)
	if getErr != nil {
		return rec, activateErr
	}
	return installed, activateErr
}

// InstallFromURL downloads url (capped at maxDownloadSize, bounded by
// downloadTimeout) and installs it via Install. url is operator-provided
// (an admin installing a plugin from a URL), so this does not attempt full
// SSRF elimination, but validateInstallURL rejects non-http(s) schemes and
// URLs with no host before any request is built.
func (s *Service) InstallFromURL(ctx context.Context, url string) (*store.Record, error) {
	if err := validateInstallURL(url); err != nil {
		return nil, fmt.Errorf("plugins: %w", err)
	}

	reqCtx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("plugins: build download request: %w", err)
	}
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("plugins: download package: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("plugins: download package: server responded %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, maxDownloadSize+1))
	if err != nil {
		return nil, fmt.Errorf("plugins: read package: %w", err)
	}
	if int64(len(data)) > maxDownloadSize {
		return nil, fmt.Errorf("plugins: package exceeds max download size of %d bytes", maxDownloadSize)
	}

	return s.Install(ctx, bytes.NewReader(data))
}

// validateInstallURL is the sink-level guard InstallFromURL applies before
// building any outbound request: raw must parse as a URL with an http or
// https scheme and a non-empty host. It rejects file://, gopher://, and
// other schemes that would let an operator-supplied string reach something
// other than a plain HTTP(S) fetch. This narrows, but does not eliminate,
// the residual SSRF surface inherent to letting an operator point the
// installer at an arbitrary http(s) URL (including internal hosts).
func validateInstallURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid install URL: %w", err)
	}
	switch parsed.Scheme {
	case "http", "https":
	default:
		return fmt.Errorf("invalid install URL: unsupported scheme %q (must be http or https)", parsed.Scheme)
	}
	if parsed.Hostname() == "" {
		return errors.New("invalid install URL: missing host")
	}
	return nil
}

// Uninstall stops id's process (if running), removes its extracted package
// tree from disk, and deletes its record from both the store and the
// in-memory registry, then notifies the attached Deliverer.
func (s *Service) Uninstall(id string) error {
	if _, err := s.Get(id); err != nil {
		return err
	}
	if s.runtime != nil {
		s.runtime.Stop(id)
	}
	if err := pkgtar.Remove(s.pluginsDir, id); err != nil {
		return fmt.Errorf("plugins: remove installed package: %w", err)
	}
	if err := s.store.Delete(id); err != nil {
		return err
	}
	s.registry.Remove(id)
	s.notifyDeliverer()
	return nil
}

// Enable transitions id to StatusActive, spawning its process first if it
// is not already running. Idempotent: a no-op (nil error) if id is already
// active.
func (s *Service) Enable(id string) error {
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
	return nil
}

// Disable stops id's process (if running) and transitions it to
// StatusDisabled. Idempotent: a no-op (nil error) if id is already
// disabled.
func (s *Service) Disable(id string) error {
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
	return nil
}

// activate spawns rec's process (if not already running) and transitions it
// to StatusActive. If the spawn fails, it best-effort transitions the
// record to StatusError (ignoring an invalid-transition failure, e.g. from
// "disabled") and returns the spawn error.
func (s *Service) activate(rec *store.Record) error {
	if s.runtime != nil && !s.runtime.Running(rec.ID) {
		if err := s.runtime.Start(context.Background(), rec, s.hostForPlugin); err != nil {
			_ = s.SetStatus(rec.ID, StatusError)
			return fmt.Errorf("plugins: start %q: %w", rec.ID, err)
		}
	}
	return s.SetStatus(rec.ID, StatusActive)
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
	s.mu.Lock()

	rec, ok := s.registry.Get(id)
	if !ok {
		s.mu.Unlock()
		return store.ErrNotFound
	}
	if !canTransition(rec.Status, status) {
		s.mu.Unlock()
		return &ErrInvalidTransition{ID: id, From: rec.Status, To: status}
	}

	updated, ok := s.registry.SetStatus(id, status)
	if !ok {
		s.mu.Unlock()
		return store.ErrNotFound
	}
	if err := s.store.Save(updated); err != nil {
		// Roll back the in-memory change so registry and disk stay in sync.
		s.registry.SetStatus(id, rec.Status)
		s.mu.Unlock()
		return err
	}
	s.mu.Unlock()
	return nil
}

// handleStatusChange is the runtime.Manager OnStatusChange callback (see
// Provide, where it is bound as a Manager constructor argument): invoked
// from the supervision loop's own goroutine whenever a running plugin's
// health transitions. healthy=false drives active -> error; healthy=true
// drives error -> active plus a Deliverer.Flush (the buffered-event
// recovery replay). Restart count is persisted best-effort afterward.
func (s *Service) handleStatusChange(id string, healthy bool) {
	newStatus := StatusError
	if healthy {
		newStatus = StatusActive
	}
	if err := s.SetStatus(id, newStatus); err != nil {
		s.log.Warn("plugins: health transition failed",
			zap.String("plugin_id", id), zap.Bool("healthy", healthy), zap.Error(err))
	} else {
		s.notifyDeliverer()
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
		if err := s.runtime.Start(ctx, rec, s.hostForPlugin); err != nil {
			s.log.Warn("plugins: failed to spawn active plugin at boot",
				zap.String("plugin_id", rec.ID), zap.Error(err))
			_ = s.SetStatus(rec.ID, StatusError)
		}
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
