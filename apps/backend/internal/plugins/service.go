package plugins

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/manifest"
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

	// lifecycleLocks serializes Enable/Disable/Install/InstallFromURL/
	// Uninstall/UpdateConfig per plugin id, so two near-simultaneous
	// lifecycle requests for the same id (e.g. two Enable clicks) cannot
	// both pass an idempotency check built on a stale read and race each
	// other's status-machine transition. Different ids stay fully
	// concurrent. Never taken by handleStatusChange (the runtime.Manager
	// supervision-goroutine callback) — that path only touches s.mu — so
	// holding a lifecycleLocks entry while calling into PluginRuntime
	// cannot deadlock against it.
	lifecycleLocks *keyedMutex

	pluginsDir string
	store      store.Store
	registry   *Registry
	state      *state.Store
	eventBus   bus.EventBus
	log        *logger.Logger

	deliverer Deliverer
	runtime   PluginRuntime
	secrets   SecretVault

	// Host data API (ADR 0043) service-layer dependencies, wired via
	// SetDataSources and handed to every pluginHost hostForPlugin builds.
	// nil until backendapp calls SetDataSources (see its doc comment); a
	// pluginHost built before that falls back to Unimplemented for these
	// accessors regardless of declared capabilities (see host_data.go's
	// accessor nil-checks).
	taskData         taskDataSource
	workflows        workflowLister
	workflowSteps    workflowStepLister
	agentProfiles    agentProfileDataSource
	sessionCodeStats sessionCodeStatsSource

	// kandevVersion is the currently running kandev build version, used to
	// enforce a package's manifest.min_kandev_version at Install (see
	// SetKandevVersion / checkMinKandevVersion). Empty (the default) means
	// no enforcement — no caller currently wires this in production; see
	// SetKandevVersion's doc comment.
	kandevVersion string

	httpClient *http.Client
}

// NewService wires a Service from its already-constructed dependencies.
// Provide is the usual entry point in production; NewService is exposed
// directly for tests that want a fake store.Store/PluginRuntime.
func NewService(pluginStore store.Store, registry *Registry, eventBus bus.EventBus, log *logger.Logger) *Service {
	return &Service{
		store:          pluginStore,
		registry:       registry,
		eventBus:       eventBus,
		log:            log,
		httpClient:     &http.Client{},
		lifecycleLocks: newKeyedMutex(),
	}
}

// keyedMutex hands out a *sync.Mutex per key, creating it on first use and
// keeping it around for the process lifetime (the plugin id keyspace is
// small and long-lived, so there is nothing to garbage-collect). Mirrors the
// parentMutex pattern in internal/task/service/handoff_service.go.
type keyedMutex struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newKeyedMutex() *keyedMutex {
	return &keyedMutex{locks: make(map[string]*sync.Mutex)}
}

// lockFor returns the mutex for key, creating it if this is the first call
// for that key. Callers must Lock/Unlock the returned mutex themselves.
func (k *keyedMutex) lockFor(key string) *sync.Mutex {
	k.mu.Lock()
	defer k.mu.Unlock()
	m, ok := k.locks[key]
	if !ok {
		m = &sync.Mutex{}
		k.locks[key] = m
	}
	return m
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

// SetSecrets wires the secret vault Provide was constructed with.
func (s *Service) SetSecrets(v SecretVault) {
	s.secrets = v
}

// SetDataSources wires the Host data API's (ADR 0043) service-layer
// dependencies, following the same post-construction "SetX" pattern as
// SetDeliverer/SetSecrets (see the "Extension points" doc comment on
// Service). backendapp calls this once, passing its already-constructed
// task, workflow, agent-settings, and analytics services directly — each
// argument's interface is a narrow slice of one of those services
// (host_data.go's taskDataSource/workflowLister/workflowStepLister/
// agentProfileDataSource/sessionCodeStatsSource), satisfied structurally, so
// no adapter type is needed. Not called by Provide itself: the plugins
// package cannot import internal/task/service, internal/workflow/service,
// etc. without an import cycle, mirroring why event delivery is wired the
// same way. Every pluginHost hostForPlugin builds after this call gets
// these dependencies; one built before (e.g. very early boot) falls back to
// Unimplemented for the Host data API regardless of declared capabilities.
func (s *Service) SetDataSources(
	tasks taskDataSource,
	workflows workflowLister,
	workflowSteps workflowStepLister,
	agentProfiles agentProfileDataSource,
	sessionCodeStats sessionCodeStatsSource,
) {
	s.taskData = tasks
	s.workflows = workflows
	s.workflowSteps = workflowSteps
	s.agentProfiles = agentProfiles
	s.sessionCodeStats = sessionCodeStats
}

// SetKandevVersion wires the currently running kandev build version,
// enabling Install to enforce a package's manifest.min_kandev_version
// (checkMinKandevVersion): a package requiring a newer kandev is rejected
// rather than installed and left to fail confusingly at spawn time. Not
// currently called by Provide — the running build version needs to be
// threaded down from internal/backendapp's ldflags-injected Version, which
// is outside this package; until a caller wires it, min_kandev_version
// remains parsed and stored but unenforced (the pre-existing behavior).
func (s *Service) SetKandevVersion(v string) {
	s.kandevVersion = v
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
		pluginID:         pluginID,
		capabilities:     rec.Capabilities,
		configSchema:     rec.ConfigSchema,
		state:            s.state,
		secrets:          s.secrets,
		bus:              s.eventBus,
		configs:          s.store,
		taskData:         s.taskData,
		workflows:        s.workflows,
		workflowSteps:    s.workflowSteps,
		agentProfiles:    s.agentProfiles,
		sessionCodeStats: s.sessionCodeStats,
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

// UpdateConfig replaces the operator-editable config for id. Incoming
// secret fields carrying the mask placeholder keep their stored value
// (mergeMaskedSecrets), the result is validated against the manifest's
// config_schema (ErrConfigInvalid on mismatch, mapped to 400 by the HTTP
// layer), secret fields are moved into the encrypted vault
// (storeConfigSecrets — the config file persists only a vault reference),
// and a currently-running plugin is restarted so the new config takes
// effect — hostForPlugin rebuilds the Host per spawn, and plugins read
// config at startup via the Host GetConfig RPC.
func (s *Service) UpdateConfig(ctx context.Context, id string, config map[string]any) error {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

	rec, err := s.Get(id)
	if err != nil {
		return err
	}
	existing, err := s.store.GetConfig(id)
	if err != nil {
		return err
	}
	merged := mergeMaskedSecrets(config, existing, rec.ConfigSchema)
	if err := validateConfigSchema(merged, rec.ConfigSchema); err != nil {
		return err
	}
	stored, err := s.storeConfigSecrets(ctx, rec, merged, existing)
	if err != nil {
		return err
	}
	if err := s.store.SetConfig(id, stored); err != nil {
		return err
	}
	return s.restartForConfigChange(rec)
}

// storeConfigSecrets moves each secret config field's cleartext value into
// the encrypted vault (id pluginConfigSecretID) and replaces it with the
// configVaultRef marker, so <id>.config.yml never persists a cleartext
// secret. A field already carrying its ref (the mask-merge round trip) is
// left alone; a secret field removed from the config deletes its vault
// entry best-effort. With no vault wired (NewService without SetSecrets —
// tests, embedded uses) values fall back to the config file itself, with a
// warning: functional, but without at-rest encryption.
func (s *Service) storeConfigSecrets(
	ctx context.Context, rec *store.Record, merged, existing map[string]any,
) (map[string]any, error) {
	secretFields := secretPropertyKeys(rec.ConfigSchema)
	if len(secretFields) == 0 {
		return merged, nil
	}
	if s.secrets == nil {
		s.log.Warn("plugins: no secret vault wired; storing secret config fields in the config file",
			zap.String("plugin_id", rec.ID))
		return merged, nil
	}

	out := make(map[string]any, len(merged))
	for k, v := range merged {
		out[k] = v
	}
	for field := range secretFields {
		value, present := out[field]
		if !present {
			s.deleteConfigSecret(ctx, rec.ID, field, existing)
			continue
		}
		cleartext, ok := value.(string)
		if !ok || cleartext == "" || isConfigVaultRef(rec.ID, field, value) {
			continue
		}
		vaultID := pluginConfigSecretID(rec.ID, field)
		if err := s.secrets.Set(ctx, vaultID, vaultID, cleartext); err != nil {
			return nil, fmt.Errorf("plugins: store secret config field %q: %w", field, err)
		}
		out[field] = configVaultRef(rec.ID, field)
	}
	return out, nil
}

// deleteConfigSecret best-effort removes the vault entry backing a secret
// config field that was removed from the config, if the stored config
// actually pointed at it.
func (s *Service) deleteConfigSecret(ctx context.Context, pluginID, field string, existing map[string]any) {
	if !isConfigVaultRef(pluginID, field, existing[field]) {
		return
	}
	if err := s.secrets.Delete(ctx, pluginConfigSecretID(pluginID, field)); err != nil {
		s.log.Warn("plugins: failed to delete removed secret config field from vault",
			zap.String("plugin_id", pluginID), zap.String("field", field), zap.Error(err))
	}
}

// GetMaskedConfig returns id's stored config with secret values (per the
// manifest's config_schema) replaced by the mask placeholder — the shape
// the operator settings UI is allowed to see.
func (s *Service) GetMaskedConfig(id string) (map[string]any, error) {
	rec, err := s.Get(id)
	if err != nil {
		return nil, err
	}
	config, err := s.store.GetConfig(id)
	if err != nil {
		return nil, err
	}
	return maskSecrets(config, rec.ConfigSchema), nil
}

// restartForConfigChange bounces id's process after a config write so the
// plugin re-reads its config on the fresh spawn. A plugin that is not
// running (disabled, errored, or no runtime wired) is left alone — it will
// pick the config up on its next spawn anyway. The config is already
// persisted by the time this runs; a restart failure transitions the plugin
// to StatusError and is returned so the operator sees that the save
// succeeded but the plugin did not come back up.
func (s *Service) restartForConfigChange(rec *store.Record) error {
	if s.runtime == nil || !s.runtime.Running(rec.ID) {
		return nil
	}
	s.runtime.Stop(rec.ID)
	ctx, cancel := context.WithTimeout(context.Background(), activateStartTimeout)
	defer cancel()
	if err := s.runtime.Start(ctx, rec, s.hostForPlugin); err != nil {
		_ = s.SetStatus(rec.ID, StatusError)
		s.notifyDeliverer()
		return fmt.Errorf("plugins: config saved but restart of %q failed: %w", rec.ID, err)
	}
	return nil
}

// Install verifies and extracts r (a tar.gz plugin package) via pkgtar into
// the plugins directory, persists a fresh store.Record (status
// "registered"), adds it to the in-memory registry, and attempts to spawn
// and activate it. A pkgtar error (e.g. pkgtar.ErrVersionExists) is
// returned unchanged so callers can map it to the right HTTP status. If the
// package is valid but the initial spawn fails, the record is still
// persisted (status "error") and returned alongside the spawn error, so an
// operator can fix the issue and retry via Enable.
//
// Installing a new version of a plugin id that is currently active/running
// stops the old process first (activate's own "already running" idempotency
// check would otherwise skip spawning entirely, leaving the live subprocess
// running the OLD version's binary even though the record/install_path now
// point at the new one). If persisting the fresh record then fails,
// rollbackFailedInstall removes only the just-extracted version directory
// (every other installed version, and the plugin's writable data directory,
// survive) and restarts the previous version's process, so a failed upgrade
// attempt never destroys a previously working install.
func (s *Service) Install(ctx context.Context, r io.Reader) (*store.Record, error) {
	result, err := pkgtar.Install(r, s.pluginsDir)
	if err != nil {
		return nil, err
	}
	if err := s.checkMinKandevVersion(result.Manifest.MinKandevVersion); err != nil {
		_ = os.RemoveAll(result.InstallPath)
		return nil, err
	}

	// The plugin id is only known once pkgtar.Install has parsed the
	// package's manifest, so the per-plugin lock is acquired here rather
	// than at the very top of the function — this still covers
	// InstallFromURL, which calls through to Install. It serializes the
	// rest of this method (the record/registry/activate mutation) against
	// any other Enable/Disable/Install/Uninstall/UpdateConfig call for the
	// same id.
	lock := s.lifecycleLocks.lockFor(result.Manifest.ID)
	lock.Lock()
	defer lock.Unlock()

	oldRec, hadOldRec := s.registry.Get(result.Manifest.ID)
	wasRunning := s.runtime != nil && s.runtime.Running(result.Manifest.ID)
	if wasRunning {
		s.runtime.Stop(result.Manifest.ID)
	}

	rec := &store.Record{
		Manifest:    *result.Manifest,
		Status:      StatusRegistered,
		InstallPath: result.InstallPath,
		Signed:      result.Signed,
		InstalledAt: time.Now().UTC(),
	}
	if err := s.store.Save(rec); err != nil {
		s.rollbackFailedInstall(result.InstallPath, oldRec, hadOldRec && wasRunning)
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

// checkMinKandevVersion rejects a package whose manifest declares a
// min_kandev_version newer than the currently running kandev build
// (manifest.CompareVersions). A no-op (nil error) when either side is
// unset: minVersion == "" (the manifest doesn't declare one, the common
// case today) or s.kandevVersion == "" (no running version wired via
// SetKandevVersion).
func (s *Service) checkMinKandevVersion(minVersion string) error {
	if minVersion == "" || s.kandevVersion == "" {
		return nil
	}
	if manifest.CompareVersions(s.kandevVersion, minVersion) < 0 {
		return fmt.Errorf("plugins: requires kandev >= %s, running %s", minVersion, s.kandevVersion)
	}
	return nil
}

// rollbackFailedInstall cleans up after a store.Save failure partway
// through Install: it removes only freshInstallPath (the version directory
// pkgtar.Install just extracted), never the whole destRoot/<id>/ tree —
// other installed versions and the plugin's writable data directory
// (destRoot/<id>/data) must survive. If restartOld is true (an existing
// record was running and got stopped to make way for this install),
// oldRec's process is best-effort restarted so the failed upgrade attempt
// doesn't also take down the previously working version; a restart failure
// is logged, not returned, since Install is already returning the original
// Save error.
func (s *Service) rollbackFailedInstall(freshInstallPath string, oldRec *store.Record, restartOld bool) {
	if err := os.RemoveAll(freshInstallPath); err != nil {
		s.log.Warn("plugins: failed to remove extracted package after a persist failure",
			zap.String("install_path", freshInstallPath), zap.Error(err))
	}
	if !restartOld || s.runtime == nil || oldRec == nil {
		return
	}
	startCtx, cancel := context.WithTimeout(context.Background(), activateStartTimeout)
	defer cancel()
	if err := s.runtime.Start(startCtx, oldRec, s.hostForPlugin); err != nil {
		s.log.Warn("plugins: failed to restart previous version after a failed upgrade",
			zap.String("plugin_id", oldRec.ID), zap.Error(err))
	}
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
// tree from disk, deletes its record from both the store and the in-memory
// registry, and deletes every plugin_state row scoped to id (best-effort —
// a failure here is logged but does not fail the overall Uninstall, since
// the package/record are already gone by that point), then notifies the
// attached Deliverer. Clearing plugin_state matters so a plugin reinstalled
// under the same id (or an id later reused by a different plugin) never
// silently inherits stale state from this install.
func (s *Service) Uninstall(id string) error {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

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
	s.deletePluginState(id)
	s.deletePluginSecrets(id)
	s.notifyDeliverer()
	return nil
}

// deletePluginSecrets best-effort removes every vault entry in id's
// namespace ("plugin:<id>:..." — both SetSecret-owned and config-backed
// entries), so a reinstall under the same id never inherits stale secrets.
// Mirrors deletePluginState: a nil vault or a listing failure is logged,
// not fatal — the package/record are already gone by this point.
func (s *Service) deletePluginSecrets(id string) {
	if s.secrets == nil {
		return
	}
	ctx := context.Background()
	ids, err := s.secrets.ListIDs(ctx)
	if err != nil {
		s.log.Warn("plugins: failed to list vault ids on uninstall", zap.String("plugin_id", id), zap.Error(err))
		return
	}
	for _, vaultID := range ids {
		if !hasPluginVaultPrefix(vaultID, id) {
			continue
		}
		if err := s.secrets.Delete(ctx, vaultID); err != nil {
			s.log.Warn("plugins: failed to delete plugin secret on uninstall",
				zap.String("plugin_id", id), zap.String("vault_id", vaultID), zap.Error(err))
		}
	}
}

// deletePluginState best-effort removes every plugin_state row for id. A
// nil state store (e.g. a Service constructed without SetState in tests, or
// before backendapp finishes wiring) is a silent no-op.
func (s *Service) deletePluginState(id string) {
	if s.state == nil {
		return
	}
	if err := s.state.DeleteAll(context.Background(), id); err != nil {
		s.log.Warn("plugins: failed to delete plugin_state on uninstall", zap.String("plugin_id", id), zap.Error(err))
	}
}

// Enable transitions id to StatusActive, spawning its process first if it
// is not already running. Idempotent: a no-op (nil error) if id is already
// active.
func (s *Service) Enable(id string) error {
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

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
	lock := s.lifecycleLocks.lockFor(id)
	lock.Lock()
	defer lock.Unlock()

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

// activateStartTimeout bounds the context activate hands to runtime.Start,
// so a hung plugin binary cannot block Enable/Install indefinitely. The
// runtime.Manager itself also enforces a startTimeout on the underlying
// go-plugin handshake (the actual blocking call is not context-aware); this
// context bound is defense-in-depth and gives Start a chance to short-circuit
// on ctx.Err() before ever spawning.
const activateStartTimeout = 30 * time.Second

// activate spawns rec's process (if not already running) and transitions it
// to StatusActive. If the spawn fails, it best-effort transitions the
// record to StatusError (ignoring an invalid-transition failure, e.g. from
// "disabled") and returns the spawn error.
func (s *Service) activate(rec *store.Record) error {
	if s.runtime != nil && !s.runtime.Running(rec.ID) {
		ctx, cancel := context.WithTimeout(context.Background(), activateStartTimeout)
		defer cancel()
		if err := s.runtime.Start(ctx, rec, s.hostForPlugin); err != nil {
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
