package plugins

import (
	"fmt"
	"os"
	"path/filepath"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins/marketplace"
	"github.com/kandev/kandev/internal/plugins/runtime"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/store"
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
func Provide(cfg *config.Config, dbPool *db.Pool, secrets SecretVault, eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, error) {
	dir := filepath.Join(cfg.ResolvedHomeDir(), pluginsSubdir)
	pluginStore := store.NewFSStore(dir)
	pluginStore.SetLogger(log)

	stateStore, err := state.NewStore(dbPool)
	if err != nil {
		return nil, nil, fmt.Errorf("plugins: init state store: %w", err)
	}

	registry := NewRegistry()
	if err := registry.Load(pluginStore); err != nil {
		return nil, nil, fmt.Errorf("plugins: load registry: %w", err)
	}

	svc := NewService(pluginStore, registry, eventBus, log)
	svc.SetState(stateStore)
	svc.SetSecrets(secrets)
	svc.SetPluginsDir(dir)

	if err := attachMarketplace(svc, dbPool, log); err != nil {
		// Non-fatal: the rest of the plugin system still works without the
		// discovery catalog (install-by-URL/upload is unaffected).
		log.Warn("plugins: marketplace init failed (non-fatal)", zap.Error(err))
	}

	rt := runtime.NewManager(dir, svc.handleStatusChange, log)
	svc.SetRuntime(rt)

	cleanup := func() error {
		rt.StopAll()
		return nil
	}
	return svc, cleanup, nil
}

// attachMarketplace builds the marketplace source store, seeds the built-in
// official source (URL overridable via KANDEV_PLUGIN_MARKETPLACE_URL), and
// attaches the catalog service to svc.
func attachMarketplace(svc *Service, dbPool *db.Pool, log *logger.Logger) error {
	sourceStore, err := marketplace.NewSourceStore(dbPool)
	if err != nil {
		return fmt.Errorf("marketplace source store: %w", err)
	}
	officialURL := marketplace.OfficialSourceURL
	if override := os.Getenv(marketplaceURLEnv); override != "" {
		officialURL = override
	}
	if err := sourceStore.EnsureBuiltin(marketplace.OfficialSourceName, officialURL); err != nil {
		return fmt.Errorf("marketplace seed builtin: %w", err)
	}
	svc.SetMarketplace(marketplace.NewService(sourceStore, log))
	return nil
}
