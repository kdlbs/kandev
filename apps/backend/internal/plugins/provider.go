package plugins

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ports"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
	"github.com/kandev/kandev/internal/plugins/webapp"
)

// marketplaceURLEnv overrides the built-in official marketplace source URL at
// boot (used by e2e to point the catalog at a local fixture server).
const marketplaceURLEnv = "KANDEV_PLUGIN_MARKETPLACE_URL"

// pluginsSubdir is the directory name under the Kandev home dir plugins
// live under: records ("<id>.yml"/"<id>.config.yml"), extracted packages
// ("<id>/<version>/..."), and per-plugin writable data
// ("<id>/data", KANDEV_PLUGIN_DATA_DIR) all share this one root, per
// docs/plans/plugins/GRPC-CONTRACT.md §6.
const pluginsSubdir = "plugins"

// Provide builds the plugin Service, following the repo's provider pattern
// (see apps/backend/AGENTS.md "Provider Pattern" and internal/jira/provider.go):
//
//   - An FS-backed installation store rooted at <cfg.ResolvedHomeDir()>/plugins.
//   - The SQLite-backed plugin_state store (internal/plugins/state), built
//     from dbPool and wired onto the Service (see StateStore()).
//   - An in-memory Registry, loaded from the FS store so existing
//     installations survive a backend restart.
//   - A runtime.Manager rooted at the same plugins directory, wired with
//     svc.handleStatusChange as its OnStatusChange callback so the
//     supervision loop's health transitions drive Service's state machine.
//
// secrets is passed straight through to Service.RevealSecret — callers pass
// secretadapter.New(secretsStore) in production (see internal/backendapp
// initPluginsService for the equivalent pattern); tests can pass a fake.
//
// eventBus may be nil (tests, or during early boot before the bus is ready).
//
// Event delivery (internal/plugins/delivery) and spawning already-active
// plugins (Service.StartActivePlugins) are NOT started by Provide — see the
// "Extension points" doc comment on Service for how backendapp attaches
// them after calling Provide. cleanup stops the runtime manager (kills any
// spawned processes); callers should register it with addCleanup.
// StoreInitErrors contains independent initialization results for the six
// plugin-owned required SQL stores. Filesystem, web-artifact, and runtime
// failures are intentionally not included: those capabilities can degrade
// while the rest of the backend remains available.
type StoreInitErrors map[string]error

// CombinedError returns a stable aggregate for callers that still use the
// all-or-nothing provider API.
func (e StoreInitErrors) CombinedError() error {
	if len(e) == 0 {
		return nil
	}
	ids := make([]string, 0, len(e))
	for id := range e {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	joined := make([]error, 0, len(ids))
	for _, id := range ids {
		joined = append(joined, fmt.Errorf("%s: %w", id, e[id]))
	}
	return errors.Join(joined...)
}

// Provide builds the plugin service and preserves the historical provider
// contract by returning the aggregate of required store errors.
func Provide(cfg *config.Config, dbPool *db.Pool, secrets SecretVault, eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, error) {
	svc, cleanup, storeErrors := ProvideWithStoreErrors(cfg, dbPool, secrets, eventBus, log)
	return svc, cleanup, storeErrors.CombinedError()
}

// ProvideWithStoreErrors builds as much of the plugin service as possible and
// reports each required SQL store independently. The backend records these
// results against the required-store tracker, while filesystem and runtime
// capabilities remain independently degradable.
func ProvideWithStoreErrors(cfg *config.Config, dbPool *db.Pool, secrets SecretVault, eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, StoreInitErrors) {
	dir := filepath.Join(cfg.ResolvedHomeDir(), pluginsSubdir)
	pluginStore := store.NewFSStore(dir)
	pluginStore.SetLogger(log)

	registry := NewRegistry()
	if err := registry.Load(pluginStore); err != nil {
		warnProvider(log, "Plugins registry load failed; starting with an empty registry", err)
	}

	svc := NewService(pluginStore, registry, eventBus, log)
	if cfg.Features.Canvases {
		svc.subscribeWebAppEvents()
	}
	svc.warnLoadedWebhookAccessIssues()

	stateStore, userStateStore, instanceStore, instanceState, sourceStore, settingsStore, storeErrors := initializeRequiredStores(dbPool)
	if stateStore != nil {
		svc.SetState(stateStore)
	}
	if userStateStore != nil {
		svc.SetUserState(userStateStore)
	}

	configureWebAppStorage(cfg, svc, instanceStore, instanceState, log)
	svc.SetSecrets(secrets)
	svc.SetPluginsDir(dir)

	seedMarketplace(svc, sourceStore, storeErrors, log)
	if settingsStore != nil {
		svc.SetSettings(settingsStore)
	}

	rt := runtime.NewManager(dir, svc.handleStatusChange, log)
	svc.SetRuntime(rt)
	stopArtifactCleanup := func() {}
	if cfg.Features.Canvases {
		stopArtifactCleanup = svc.StartWebAppArtifactCleanupWorker(context.Background())
	}

	cleanup := func() error {
		stopArtifactCleanup()
		svc.closeWebAppEvents()
		rt.StopAll()
		return nil
	}
	return svc, cleanup, storeErrors
}

func recordStoreError(storeErrors StoreInitErrors, id string, err error) {
	if err != nil {
		storeErrors[id] = err
	}
}

func initializeRequiredStores(dbPool *db.Pool) (*state.Store, *state.UserStore, *instances.Store, *state.InstanceStore, *marketplace.SourceStore, *settingsStore, StoreInitErrors) {
	storeErrors := make(StoreInitErrors)
	if dbPool == nil || dbPool.Writer() == nil || dbPool.Reader() == nil {
		err := errors.New("database pool is unavailable")
		for _, id := range []string{
			"plugin-instances", "plugin-marketplace", "plugin-settings",
			"plugin-state", "plugin-instance-state", "plugin-user-state",
		} {
			storeErrors[id] = err
		}
		return nil, nil, nil, nil, nil, nil, storeErrors
	}
	stateStore, err := state.NewStore(dbPool)
	recordStoreError(storeErrors, "plugin-state", err)
	userStateStore, err := state.NewUserStore(dbPool)
	recordStoreError(storeErrors, "plugin-user-state", err)
	instanceStore, err := instances.NewStore(dbPool)
	recordStoreError(storeErrors, "plugin-instances", err)
	instanceState, err := state.NewInstanceStore(dbPool)
	recordStoreError(storeErrors, "plugin-instance-state", err)
	sourceStore, err := marketplace.NewSourceStore(dbPool)
	recordStoreError(storeErrors, "plugin-marketplace", err)
	settingsStore, err := newSettingsStore(dbPool)
	recordStoreError(storeErrors, "plugin-settings", err)
	return stateStore, userStateStore, instanceStore, instanceState, sourceStore, settingsStore, storeErrors
}

func configureWebAppStorage(cfg *config.Config, svc *Service, instanceStore *instances.Store, instanceState *state.InstanceStore, log *logger.Logger) {
	if instanceState != nil {
		svc.SetInstanceState(instanceState)
	}
	artifactStore, err := webapp.NewArtifactStore(filepath.Join(cfg.ResolvedHomeDir(), pluginsSubdir, "webapps"))
	if err != nil {
		warnProvider(log, "Plugin web-artifact storage is unavailable", err)
		return
	}
	if !cfg.Features.Canvases || instanceStore == nil {
		return
	}
	if _, err := instanceStore.ReconcileArtifacts(context.Background(), func(path, digest string, bytes int64) (instances.ArtifactCheck, error) {
		artifact, err := artifactStore.Reconcile(webapp.Artifact{Digest: digest, RelativePath: path, Bytes: bytes})
		if err != nil {
			return instances.ArtifactCheck{}, err
		}
		return instances.ArtifactCheck{Available: artifact.Available, Reason: artifact.Reason}, nil
	}); err != nil {
		warnProvider(log, "Plugin web-artifact reconciliation failed", err)
	}
	svc.SetWebAppStorage(instanceStore, artifactStore)
	backendPort := cfg.Server.Port
	if backendPort == 0 {
		backendPort = ports.Backend
	}
	webPort := cfg.Launcher.WebPort
	if webPort == 0 {
		webPort = backendPort
	}
	frameAncestors, err := webapp.FrameAncestorsForConfig(backendPort, webPort, fmt.Sprintf("http://localhost:%d", webPort))
	if err != nil {
		warnProvider(log, "Plugin web-app runtime is unavailable", err)
		return
	}
	svc.SetWebRuntime(webapp.NewRuntime(webapp.NewTokenManager(nil), artifactStore, svc.validateWebAppBinding, frameAncestors))
}

func seedMarketplace(svc *Service, sourceStore *marketplace.SourceStore, storeErrors StoreInitErrors, log *logger.Logger) {
	if sourceStore == nil {
		return
	}
	officialURL := marketplace.OfficialSourceURL
	if override := os.Getenv(marketplaceURLEnv); override != "" {
		officialURL = override
	}
	if err := sourceStore.EnsureBuiltin(marketplace.OfficialSourceName, officialURL); err != nil {
		recordStoreError(storeErrors, "plugin-marketplace", fmt.Errorf("marketplace seed builtin: %w", err))
	}
	svc.SetMarketplace(marketplace.NewService(sourceStore, log))
}

func warnProvider(log *logger.Logger, message string, err error) {
	if log != nil {
		log.Warn(message, zap.Error(err))
	}
}
