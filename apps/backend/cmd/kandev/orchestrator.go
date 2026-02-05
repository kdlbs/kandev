package main

import (
	"errors"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

func provideOrchestrator(
	log *logger.Logger,
	eventBus bus.EventBus,
	taskRepo repository.Repository,
	taskSvc *taskservice.Service,
	userSvc *userservice.Service,
	lifecycleMgr *lifecycle.Manager,
	agentRegistry *registry.Registry,
	workflowSvc *workflowservice.Service,
) (*orchestrator.Service, *messageCreatorAdapter, error) {
	if lifecycleMgr == nil {
		return nil, nil, errors.New("lifecycle manager is required: configure agent runtime (docker or standalone)")
	}

	taskRepoAdapter := &taskRepositoryAdapter{repo: taskRepo, svc: taskSvc}
	agentManagerClient := newLifecycleAdapter(lifecycleMgr, agentRegistry, log)

	serviceCfg := orchestrator.DefaultServiceConfig()
	orchestratorSvc := orchestrator.NewService(serviceCfg, eventBus, agentManagerClient, taskRepoAdapter, taskRepo, userSvc, log)
	taskSvc.SetExecutionStopper(orchestratorSvc)

	msgCreator := &messageCreatorAdapter{svc: taskSvc, logger: log}
	orchestratorSvc.SetMessageCreator(msgCreator)

	orchestratorSvc.SetTurnService(newTurnServiceAdapter(taskSvc))

	// Wire workflow step getter for prompt building
	// Since both orchestrator.WorkflowStep and workflow/models.WorkflowStep
	// are now aliases to v1.WorkflowStep, workflowSvc directly implements
	// the orchestrator.WorkflowStepGetter interface - no adapter needed.
	if workflowSvc != nil {
		orchestratorSvc.SetWorkflowStepGetter(workflowSvc)
	}

	return orchestratorSvc, msgCreator, nil
}
