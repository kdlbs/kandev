package main

import (
	"github.com/kandev/kandev/internal/agent/discovery"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	"github.com/kandev/kandev/internal/events/bus"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
)

func provideServices(cfg *config.Config, log *logger.Logger, repos *Repositories, eventBus bus.EventBus) (*Services, *agentsettingscontroller.Controller, error) {
	discoveryRegistry, err := discovery.LoadRegistry()
	if err != nil {
		return nil, nil, err
	}
	agentSettingsController := agentsettingscontroller.NewController(repos.AgentSettings, discoveryRegistry, repos.Task, log)

	userSvc := userservice.NewService(repos.User, eventBus, log)
	editorSvc := editorservice.NewService(repos.Editor, repos.Task, userSvc)
	promptSvc := promptservice.NewService(repos.Prompts)
	taskSvc := taskservice.NewService(
		repos.Task,
		eventBus,
		log,
		taskservice.RepositoryDiscoveryConfig{
			Roots:    cfg.RepositoryDiscovery.Roots,
			MaxDepth: cfg.RepositoryDiscovery.MaxDepth,
		},
	)

	return &Services{
		Task:    taskSvc,
		User:    userSvc,
		Editor:  editorSvc,
		Prompts: promptSvc,
		// Notification service is initialized after gateway is available.
		Notification: nil,
	}, agentSettingsController, nil
}
