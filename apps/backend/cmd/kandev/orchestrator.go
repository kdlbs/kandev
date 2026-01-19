package main

import (
	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
)

func provideOrchestrator(
	log *logger.Logger,
	eventBus bus.EventBus,
	taskRepo repository.Repository,
	taskSvc *taskservice.Service,
	userSvc *userservice.Service,
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
) (*orchestrator.Service, error) {
	taskRepoAdapter := &taskRepositoryAdapter{repo: taskRepo, svc: taskSvc}

	var agentManagerClient executor.AgentManagerClient
	if lifecycleMgr != nil {
		agentManagerClient = newLifecycleAdapter(lifecycleMgr, agentRegistry, log)
	} else {
		agentManagerClient = executor.NewMockAgentManagerClient(log)
	}

	serviceCfg := orchestrator.DefaultServiceConfig()
	orchestratorSvc := orchestrator.NewService(serviceCfg, eventBus, agentManagerClient, taskRepoAdapter, taskRepo, userSvc, log)
	taskSvc.SetExecutionStopper(orchestratorSvc)

	msgCreator := &messageCreatorAdapter{svc: taskSvc}
	orchestratorSvc.SetMessageCreator(msgCreator)

	return orchestratorSvc, nil
}
