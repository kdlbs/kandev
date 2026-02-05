package main

import (
	"context"

	"go.uber.org/zap"

	agentcontroller "github.com/kandev/kandev/internal/agent/controller"
	agenthandlers "github.com/kandev/kandev/internal/agent/handlers"
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/scripts"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	gateways "github.com/kandev/kandev/internal/gateway/websocket"
	notificationcontroller "github.com/kandev/kandev/internal/notifications/controller"
	notificationservice "github.com/kandev/kandev/internal/notifications/service"
	notificationstore "github.com/kandev/kandev/internal/notifications/store"
	"github.com/kandev/kandev/internal/orchestrator"
	orchestratorcontroller "github.com/kandev/kandev/internal/orchestrator/controller"
	orchestratorhandlers "github.com/kandev/kandev/internal/orchestrator/handlers"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
)

// scriptServiceAdapter adapts the task service to scripts.ScriptService.
type scriptServiceAdapter struct {
	taskSvc *taskservice.Service
}

func (a *scriptServiceAdapter) GetRepositoryScript(ctx context.Context, id string) (*scripts.RepositoryScript, error) {
	script, err := a.taskSvc.GetRepositoryScript(ctx, id)
	if err != nil {
		return nil, err
	}
	return &scripts.RepositoryScript{
		ID:      script.ID,
		Name:    script.Name,
		Command: script.Command,
	}, nil
}

func provideGateway(
	ctx context.Context,
	log *logger.Logger,
	eventBus bus.EventBus,
	taskSvc *taskservice.Service,
	userSvc *userservice.Service,
	orchestratorSvc *orchestrator.Service,
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
	notificationRepo notificationstore.Repository,
	taskRepo repository.Repository,
) (*gateways.Gateway, *notificationservice.Service, *notificationcontroller.Controller, error) {
	gateway, cleanup, err := gateways.Provide(log)
	if err != nil {
		return nil, nil, nil, err
	}
	_ = cleanup

	// Enable dedicated terminal WebSocket for passthrough mode
	scriptSvc := &scriptServiceAdapter{taskSvc: taskSvc}
	if lifecycleMgr != nil {
		gateway.SetLifecycleManager(lifecycleMgr, userSvc, scriptSvc)
	}

	orchestratorCtrl := orchestratorcontroller.NewController(orchestratorSvc)
	orchestratorHandlers := orchestratorhandlers.NewHandlers(orchestratorCtrl, log)
	orchestratorHandlers.RegisterHandlers(gateway.Dispatcher)

	if lifecycleMgr != nil && agentRegistry != nil {
		agentCtrl := agentcontroller.NewController(lifecycleMgr, agentRegistry)
		agentHandlers := agenthandlers.NewHandlers(agentCtrl, log)
		agentHandlers.RegisterHandlers(gateway.Dispatcher)

		workspaceFileHandlers := agenthandlers.NewWorkspaceFileHandlers(lifecycleMgr, log)
		workspaceFileHandlers.RegisterHandlers(gateway.Dispatcher)

		shellHandlers := agenthandlers.NewShellHandlers(lifecycleMgr, scriptSvc, log)
		shellHandlers.RegisterHandlers(gateway.Dispatcher)

		gitHandlers := agenthandlers.NewGitHandlers(lifecycleMgr, log)
		gitHandlers.RegisterHandlers(gateway.Dispatcher)

		passthroughHandlers := agenthandlers.NewPassthroughHandlers(lifecycleMgr, log)
		passthroughHandlers.RegisterHandlers(gateway.Dispatcher)
	}

	go gateway.Hub.Run(ctx)
	gateways.RegisterTaskNotifications(ctx, eventBus, gateway.Hub, log)
	gateways.RegisterUserNotifications(ctx, eventBus, gateway.Hub, log)

	notificationSvc := notificationservice.NewService(notificationRepo, taskRepo, gateway.Hub, log)
	notificationCtrl := notificationcontroller.NewController(notificationSvc)
	if eventBus != nil {
		_, err = eventBus.Subscribe(events.TaskSessionStateChanged, func(ctx context.Context, event *bus.Event) error {
			data, ok := event.Data.(map[string]interface{})
			if !ok {
				return nil
			}
			taskID, _ := data["task_id"].(string)
			sessionID, _ := data["session_id"].(string)
			newState, _ := data["new_state"].(string)
			notificationSvc.HandleTaskSessionStateChanged(ctx, taskID, sessionID, newState)
			return nil
		})
		if err != nil {
			log.Error("Failed to subscribe to task session notifications", zap.Error(err))
		}
	}

	return gateway, notificationSvc, notificationCtrl, nil
}
