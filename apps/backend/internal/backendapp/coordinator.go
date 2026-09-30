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
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/persistence/requiredstores"
	taskservice "github.com/kandev/kandev/internal/task/service"
	taskusage "github.com/kandev/kandev/internal/task/usage"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
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
	workflowSvc *workflowservice.Service,
	agentProfiles settingsstore.Repository,
	enabled bool,
	phase2 bool,
	phase3 bool,
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
	svc := coordinator.NewService(store, validator, taskSvc, log, coordinator.WithPhase2(phase2),
		coordinator.WithPhase3(phase3Effective(enabled, phase2, phase3)))
	svc.SetProposalDeps(taskSvc, taskSvc, taskSvc, workflowSvc)
	svc.SetUndoDeps(&coordinatorUndoSeam{tasks: taskSvc, steps: workflowSvc})
	return svc, nil
}

// phase3Effective is the single source for the phase 3 gate: the autonomy
// surface exists only when coordinator, coordinator phase 2 and coordinator
// phase 3 are all on.
func phase3Effective(coordinatorOn, phase2, phase3 bool) bool {
	return coordinatorOn && phase2 && phase3
}

// coordinatorStandingInstructionsReader closes over svc to build the
// Standing Instructions system-prompt content
// (docs/specs/coordinator/system-design/copilot.md#standing-instructions)
// for orchestrator.Service.SetCoordinatorStandingInstructionsReader.
// internal/orchestrator cannot import internal/coordinator directly (the
// dependency runs the other way), so this closure is the seam: it reads the
// coordinator's name/context through svc and renders them with
// coordinator.StandingInstructions.
func coordinatorStandingInstructionsReader(
	svc *coordinator.Service,
	log *logger.Logger,
) func(ctx context.Context, coordinatorID, workspaceName, workspaceID string) (string, error) {
	return func(ctx context.Context, coordinatorID, workspaceName, workspaceID string) (string, error) {
		name, coordinatorContext, err := svc.CoordinatorStandingInstructionsData(ctx, coordinatorID)
		if err != nil {
			return "", err
		}
		var sections []string
		orders, orderErr := svc.StandingOrdersInstructionSection(ctx, coordinatorID)
		if orderErr != nil {
			log.Warn("standing orders unreadable; instructions built without them",
				zap.String("coordinator_id", coordinatorID), zap.Error(orderErr))
		} else {
			sections = append(sections, orders)
		}
		goal, goalErr := svc.GoalInstructionSection(ctx, coordinatorID)
		if goalErr != nil {
			log.Warn("goal unreadable; instructions built without it",
				zap.String("coordinator_id", coordinatorID), zap.Error(goalErr))
		} else {
			sections = append(sections, goal)
		}
		sections = append(sections, svc.ImprovementInstructionSection())
		return coordinator.StandingInstructions(workspaceName, workspaceID, name, coordinatorContext, sections...), nil
	}
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
		registerCoordinatorDecisions(p.router, p.eventBus, svc, p.taskSvc, p.services.Workflow, p.log),
	}
	if svc.Phase3Enabled() {
		if p.taskRepo != nil && p.taskSvc != nil {
			svc.SetRelayDeps(p.taskRepo, p.taskSvc)
		}
		if p.taskSvc != nil {
			coordinatorWakeSources = &coordinatorWakeReader{tasks: p.taskSvc}
		}
		wireCoordinatorDelivery(svc, p.taskSvc, p.orchestratorSvc)
		if p.addCleanup != nil {
			p.addCleanup(func() error {
				svc.StopWakeRecorder()
				svc.StopWakeBackstop()
				svc.StopDelivery()
				return nil
			})
		}
		wireCoordinatorSpend(p.services.UsageWriter, svc, p.taskSvc, p.orchestratorSvc)
		wireCoordinatorContainment(svc, containmentDepsFrom(p), p.orchestratorSvc, p.log)
		if p.taskSvc != nil && p.orchestratorSvc != nil {
			svc.SetReplyDeps(p.taskSvc, p.orchestratorSvc)
		}
		for _, register := range phase3Registrations() {
			hooks = append(hooks, register(p.router, p.eventBus, svc, p.log))
		}
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
// 04), decisions (task 07). Conversation and subscribers remain no-ops until
// their work packages land; decisions runs StartupRecoveryPass.
func startCoordinatorBackgroundPass(ctx context.Context, t0 time.Time, hooks []func(context.Context, time.Time)) {
	go func() {
		for _, hook := range hooks {
			hook(ctx, t0)
		}
	}()
}

// registerCoordinatorConversation is task 03's named registration function
// (Build decision 14): it registers the conversation route and returns the
// startup cleanup pass as the background hook. svc's conversation
// dependencies (task and session management) must already be set —
// wireCoordinatorConversation (coordinator_conversation.go) calls
// svc.SetConversationDeps and svc.SetConversationHooks from
// registerSecondaryRoutes, before this function runs. When the dependencies
// were not set, it logs an error and registers nothing, so the route is 404
// rather than half-built.
func registerCoordinatorConversation(router *gin.Engine, _ bus.EventBus, svc *coordinator.Service, log *logger.Logger) func(context.Context, time.Time) {
	if !svc.ConversationDepsReady() {
		log.Error("coordinator conversation dependencies not set; conversation route not registered")
		return func(context.Context, time.Time) {}
	}
	coordinator.RegisterConversationRoute(router, svc, log)
	return svc.CleanupConversationTasks
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

	if svc.Phase2Enabled() {
		if sub, err := coordinator.SubscribeWorkflowDeleted(eventBus, svc, log); err != nil {
			log.Error("failed to subscribe coordinator to workflow.deleted", zap.Error(err))
		} else {
			subs = append(subs, sub)
		}
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
// (Build decision 14). It wires the approve/reject dependencies (the task
// service, the workflow step reader used for step-eligibility checks, and
// the event bus coordinator.updated publishes on) via SetDecisionDeps before
// the HTTP routes registered by registerCoordinatorHTTPRoutes above can serve
// a request, then returns a background-pass hook that runs StartupRecoveryPass
// and, once it returns, starts the once-a-minute approval sweep
// (docs/specs/coordinator/system-design/proposal-recovery.md#recovery).
// *taskservice.Service satisfies coordinator.DecisionTaskService and
// *workflowservice.Service satisfies coordinator.WorkflowStepReader.
func registerCoordinatorDecisions(
	_ *gin.Engine,
	eventBus bus.EventBus,
	svc *coordinator.Service,
	taskSvc *taskservice.Service,
	steps coordinator.WorkflowStepReader,
	_ *logger.Logger,
) func(context.Context, time.Time) {
	svc.SetDecisionDeps(taskSvc, steps, eventBus)
	return func(ctx context.Context, t0 time.Time) {
		svc.StartupRecoveryPass(ctx, t0)
		svc.StartApprovalSweep(ctx)
		svc.StartActivityRetention(ctx)
	}
}

// coordinatorRegistration is the shape shared by the phase 3 registration
// functions: each registers its routes and subscriptions and returns the hook
// that runs in the startup background pass.
type coordinatorRegistration = func(*gin.Engine, bus.EventBus, *coordinator.Service, *logger.Logger) func(context.Context, time.Time)

func noopCoordinatorHook(*gin.Engine, bus.EventBus, *coordinator.Service, *logger.Logger) func(context.Context, time.Time) {
	return func(context.Context, time.Time) {}
}

// Phase 3 registration functions, one per work package. They are package
// variables so tests can swap them; each later work package replaces its
// no-op default.
var (
	registerCoordinatorContainment  coordinatorRegistration = noopCoordinatorHook
	registerCoordinatorSpend        coordinatorRegistration = noopCoordinatorHook
	registerCoordinatorWake         coordinatorRegistration = registerCoordinatorWakeState
	registerCoordinatorDelivery     coordinatorRegistration = registerCoordinatorDeliveryWorker
	registerCoordinatorRelay        coordinatorRegistration = registerCoordinatorRelayRoutes
	registerCoordinatorReply        coordinatorRegistration = noopCoordinatorHook
	registerCoordinatorAutomatic    coordinatorRegistration = noopCoordinatorHook
	registerCoordinatorImprovements coordinatorRegistration = noopCoordinatorHook
)

func phase3Registrations() []coordinatorRegistration {
	return []coordinatorRegistration{
		registerCoordinatorContainment,
		registerCoordinatorSpend,
		registerCoordinatorWake,
		registerCoordinatorDelivery,
		registerCoordinatorRelay,
		registerCoordinatorReply,
		registerCoordinatorAutomatic,
		registerCoordinatorImprovements,
	}
}

// registerCoordinatorRelayRoutes mounts the answer-in-place read route. The
// readers it serves from are set on svc by registerCoordinatorRoutes.
func registerCoordinatorRelayRoutes(router *gin.Engine, _ bus.EventBus, svc *coordinator.Service, log *logger.Logger) func(context.Context, time.Time) {
	coordinator.RegisterRelayRoutes(router, svc, log)
	return func(context.Context, time.Time) {}
}

// registerCoordinatorWakeState returns the startup hook that prunes wake
// state. A prune failure is logged and never blocks startup.
func registerCoordinatorWakeState(_ *gin.Engine, eventBus bus.EventBus, svc *coordinator.Service, log *logger.Logger) func(context.Context, time.Time) {
	if src := coordinatorWakeSources; src != nil && eventBus != nil {
		svc.StartWakeRecorder(eventBus, src)
	} else {
		log.Warn("coordinator wake sources unavailable; event-driven wake recording is off")
	}
	return func(ctx context.Context, _ time.Time) {
		turns, wakes, err := svc.PruneWakeState(ctx, time.Now().UTC())
		if err != nil {
			log.Warn("coordinator wake state pruning failed", zap.Error(err))
		} else if turns > 0 || wakes > 0 {
			log.Info("coordinator wake state pruning complete",
				zap.Int64("turns", turns), zap.Int64("wakes", wakes))
		}
		svc.StartWakeBackstop(ctx)
	}
}

// coordinatorWakeSources is the task-side read surface of the wake recorder
// and backstop, set by registerCoordinatorRoutes before the phase 3
// registrations run.
var coordinatorWakeSources coordinator.WakeSources

// wireCoordinatorSpend gives the coordinator its usage ledger reader, active
// turn reader and turn canceller, and registers the ceiling check as the usage
// writer's post-commit observer. A missing dependency leaves the operation
// that needs it failing closed, so a nil argument is tolerated.
func wireCoordinatorSpend(writer *taskusage.Writer, svc *coordinator.Service, taskSvc *taskservice.Service, orch *orchestrator.Service) {
	var ledger coordinator.SpendLedger
	var turns coordinator.ActiveTurnReader
	if taskSvc != nil {
		ledger, turns = taskSvc, taskSvc
	}
	var canceller coordinator.TurnCanceller
	if orch != nil {
		canceller = orch
	}
	svc.SetSpendDeps(ledger, turns, canceller)
	if writer != nil {
		writer.SetRecordedObserver(svc.ObserveUsage)
	}
}
