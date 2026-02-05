package main

import (
	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/registry"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	"github.com/kandev/kandev/internal/events/bus"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

func provideServices(cfg *config.Config, log *logger.Logger, repos *Repositories, eventBus bus.EventBus) (*Services, *agentsettingscontroller.Controller, error) {
	discoveryRegistry, err := discovery.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	agentRegistry := registry.NewRegistry(log)
	agentRegistry.LoadDefaults()
	agentSettingsController := agentsettingscontroller.NewController(repos.AgentSettings, discoveryRegistry, agentRegistry, repos.Task, log)

	userSvc := userservice.NewService(repos.User, eventBus, log)
	editorSvc := editorservice.NewService(repos.Editor, repos.Task, userSvc)
	promptSvc := promptservice.NewService(repos.Prompts)
	workflowSvc := workflowservice.NewService(repos.Workflow, log)
	taskSvc := taskservice.NewService(
		repos.Task,
		eventBus,
		log,
		taskservice.RepositoryDiscoveryConfig{
			Roots:    cfg.RepositoryDiscovery.Roots,
			MaxDepth: cfg.RepositoryDiscovery.MaxDepth,
		},
	)

	// Wire workflow step creator to task service for board creation
	taskSvc.SetWorkflowStepCreator(workflowSvc)

	// Wire workflow step getter to task service for MoveTask
	// Since both workflow/models.WorkflowStep and task/service.WorkflowStep
	// are now aliases to v1.WorkflowStep, workflowSvc directly implements
	// the taskservice.WorkflowStepGetter interface - no adapter needed.
	taskSvc.SetWorkflowStepGetter(workflowSvc)

	return &Services{
		Task:     taskSvc,
		User:     userSvc,
		Editor:   editorSvc,
		Prompts:  promptSvc,
		Workflow: workflowSvc,
		// Notification service is initialized after gateway is available.
		Notification: nil,
	}, agentSettingsController, nil
}
