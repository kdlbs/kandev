package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/plugins"
	"github.com/kandev/kandev/internal/plugins/delivery"
	pluginstore "github.com/kandev/kandev/internal/plugins/store"
)

// pluginActivePluginsAdapter adapts *plugins.Service to delivery.PluginLister
// so the Deliverer can track which plugins should receive live deliveries
// (StatusActive) or buffer them (StatusError) without internal/plugins/delivery
// importing internal/plugins — see that package's doc comment ("backendapp
// adapts *plugins.Service to satisfy them").
type pluginActivePluginsAdapter struct {
	svc *plugins.Service
}

// ActivePlugins implements delivery.PluginLister.
func (a pluginActivePluginsAdapter) ActivePlugins() []delivery.PluginRecord {
	records := a.svc.Registry().List()
	out := make([]delivery.PluginRecord, 0, len(records))
	for _, rec := range records {
		if rec.Status != pluginstore.StatusActive && rec.Status != pluginstore.StatusError {
			continue
		}
		out = append(out, delivery.PluginRecord{
			ID:            rec.ID,
			EventSubjects: rec.Capabilities.Events,
			Status:        rec.Status,
		})
	}
	return out
}

// startPluginsSubsystems attaches event delivery to svc and spawns every
// already-active, runtime-managed plugin (resuming plugins that were active
// before a backend restart), registering shutdown via addCleanup. Mirrors
// how the Jira/Linear/Sentry pollers are started in
// startAgentInfrastructure: construction happens in initPluginsService
// (services.go), lifecycle start happens here once ctx/addCleanup exist.
func startPluginsSubsystems(ctx context.Context, svc *plugins.Service, eventBus bus.EventBus, log *logger.Logger, addCleanup func(func() error)) {
	transport := plugins.NewRuntimeTransport(svc.Runtime())
	deliverer := delivery.New(eventBus, transport, pluginActivePluginsAdapter{svc: svc}, log)
	svc.SetDeliverer(deliverer)
	deliverer.Refresh()
	addCleanup(func() error { deliverer.Stop(); return nil })
	addCleanup(func() error { svc.Shutdown(); return nil })

	svc.StartActivePlugins(ctx)

	log.Info("Plugins subsystems started (event delivery + active plugin spawn)")
}
