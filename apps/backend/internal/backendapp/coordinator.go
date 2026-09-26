package backendapp

import (
	"context"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	settingsstore "github.com/kandev/kandev/internal/agent/settings/store"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/coordinator"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// initCoordinatorWiring builds the coordinator store unconditionally (it is a
// requiredstores catalog entry) and, only when features.coordinator is
// enabled, the service that sits on top of it
// (coordinators.md#flag-and-wiring, Build decision 15).
func initCoordinatorWiring(
	ctx context.Context,
	dbPool *db.Pool,
	storeTracker *requiredstores.Tracker,
	taskSvc *taskservice.Service,
	agentProfiles settingsstore.Repository,
	enabled bool,
	log *logger.Logger,
) (*coordinator.Service, error) {
	store, storeErr := coordinator.NewStore(dbPool.Writer(), dbPool.Reader())
	if recordErr := recordRequiredStore(ctx, storeTracker, "coordinator", storeErr); recordErr != nil {
		return nil, fmt.Errorf("initialize coordinator: %w", recordErr)
	}
	if !enabled {
		return nil, nil
	}

	validator := coordinator.NewValidator(agentProfiles, taskSvc)
	return coordinator.NewService(store, validator, taskSvc, log), nil
}

// registerCoordinatorHTTPRoutes is a test seam over coordinator.RegisterRoutes:
// production always calls the real function; tests may override it to
// observe its call time relative to when T0 was captured.
var registerCoordinatorHTTPRoutes = coordinator.RegisterRoutes

// registerCoordinatorRoutes registers the coordinator CRUD, proposals-read and
// stalls-read HTTP routes, the coordinator.updated WS forwarder, and starts
// the background startup pass. Callers must only invoke it when
// features.coordinator is enabled.
//
// T0 is recorded before the routes register, so that any task a request
// creates after this point has created_at >= T0
// (coordinators.md#flag-and-wiring): a later work package's conversation
// cleanup pass treats only tasks with created_at < T0 as pre-restart
// leftovers.
func registerCoordinatorRoutes(p routeParams) {
	if p.router == nil || p.services == nil || p.services.Coordinator == nil {
		return
	}
	t0 := time.Now().UTC()
	svc := p.services.Coordinator
	registerCoordinatorHTTPRoutes(p.router, svc, p.log)
	if p.gateway != nil {
		gateways.RegisterCoordinatorNotifications(p.ctx, p.eventBus, p.gateway.Hub, p.log)
	}
	hooks := []func(context.Context, time.Time){
		registerCoordinatorConversation(p.router, p.eventBus, svc, p.log),
		registerCoordinatorSubscribers(p.router, p.eventBus, svc, p.log),
		registerCoordinatorDecisions(p.router, p.eventBus, svc, p.log),
	}
	runCoordinatorBackgroundPass(p.ctx, t0, hooks)
}

// runCoordinatorBackgroundPass is a test seam over startCoordinatorBackgroundPass:
// production always calls it directly; tests may override it to observe the
// T0 value it is given relative to when routes registered.
var runCoordinatorBackgroundPass = startCoordinatorBackgroundPass

// startCoordinatorBackgroundPass runs each later work package's named
// registration hook with the given T0, in the fixed order the spec
// describes (Build decision 14): conversation (task 03), subscribers (task
// 04), decisions (task 07). All three are no-ops in WP-1; later work orders
// fill in their bodies without changing this call site or ordering.
func startCoordinatorBackgroundPass(ctx context.Context, t0 time.Time, hooks []func(context.Context, time.Time)) {
	go func() {
		for _, hook := range hooks {
			hook(ctx, t0)
		}
	}()
}

// registerCoordinatorConversation is task 03's named registration function
// (Build decision 14). No-op until that work package lands.
func registerCoordinatorConversation(_ *gin.Engine, _ bus.EventBus, _ *coordinator.Service, _ *logger.Logger) func(context.Context, time.Time) {
	return func(context.Context, time.Time) {}
}

// registerCoordinatorSubscribers is task 04's named registration function
// (Build decision 14). It subscribes the coordinator package to task.stalled
// and workspace.deleted as soon as it is called (before the background pass
// runs), and returns a hook that prunes stall records once the startup pass
// reaches it and releases both subscriptions when the app context ends
// (docs/specs/coordinator/system-design/needs-you.md#stall-records,
// coordinators.md#workspace-deletion).
func registerCoordinatorSubscribers(_ *gin.Engine, eventBus bus.EventBus, svc *coordinator.Service, log *logger.Logger) func(context.Context, time.Time) {
	if eventBus == nil || svc == nil {
		return func(context.Context, time.Time) {}
	}

	var subs []bus.Subscription
	if sub, err := coordinator.SubscribeTaskStalled(eventBus, svc, log); err != nil {
		log.Error("failed to subscribe coordinator to task.stalled", zap.Error(err))
	} else {
		subs = append(subs, sub)
	}
	if sub, err := coordinator.SubscribeWorkspaceDeleted(eventBus, svc, log); err != nil {
		log.Error("failed to subscribe coordinator to workspace.deleted", zap.Error(err))
	} else {
		subs = append(subs, sub)
	}

	return func(ctx context.Context, _ time.Time) {
		go func() {
			<-ctx.Done()
			for _, sub := range subs {
				if sub.IsValid() {
					_ = sub.Unsubscribe()
				}
			}
		}()

		pruned, err := svc.PruneStalls(ctx, time.Now().UTC())
		if err != nil {
			log.Warn("coordinator stall pruning failed", zap.Error(err))
			return
		}
		if pruned > 0 {
			log.Info("coordinator stall pruning complete", zap.Int64("pruned", pruned))
		}
	}
}

// registerCoordinatorDecisions is task 07's named registration function
// (Build decision 14). No-op until that work package lands.
func registerCoordinatorDecisions(_ *gin.Engine, _ bus.EventBus, _ *coordinator.Service, _ *logger.Logger) func(context.Context, time.Time) {
	return func(context.Context, time.Time) {}
}
