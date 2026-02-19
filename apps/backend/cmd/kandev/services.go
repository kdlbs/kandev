package main

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/discovery"
	"github.com/kandev/kandev/internal/agent/registry"
	agentsettingscontroller "github.com/kandev/kandev/internal/agent/settings/controller"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	editorservice "github.com/kandev/kandev/internal/editors/service"
	"github.com/kandev/kandev/internal/events/bus"
	promptservice "github.com/kandev/kandev/internal/prompts/service"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
)

func provideServices(cfg *config.Config, log *logger.Logger, repos *Repositories, eventBus bus.EventBus, agentRegistry *registry.Registry) (*Services, *agentsettingscontroller.Controller, error) {
	// Load custom TUI agents from DB into registry before discovery
	loadCustomTUIAgents(context.Background(), repos, agentRegistry, log)

	discoveryRegistry, err := discovery.LoadRegistry(context.Background(), agentRegistry, log)
	if err != nil {
		return nil, nil, err
	}
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
	taskSvc.SetWorkflowStepGetter(&workflowStepGetterAdapter{svc: workflowSvc})

	// Wire start step resolver to task service for CreateTask
	taskSvc.SetStartStepResolver(&startStepResolverAdapter{svc: workflowSvc})

	// Wire workflow provider to workflow service for export/import
	workflowSvc.SetWorkflowProvider(&workflowProviderAdapter{svc: taskSvc})

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

// loadCustomTUIAgents loads user-defined TUI agents from the database into the registry.
// Non-fatal: logs warnings but continues if any individual agent fails.
func loadCustomTUIAgents(ctx context.Context, repos *Repositories, agentRegistry *registry.Registry, log *logger.Logger) {
	tuiAgents, err := repos.AgentSettings.ListTUIAgents(ctx)
	if err != nil {
		log.Warn("failed to load custom TUI agents from database", zap.Error(err))
		return
	}
	for _, agent := range tuiAgents {
		if agent.TUIConfig == nil {
			continue
		}
		cfg := agent.TUIConfig
		if regErr := agentRegistry.RegisterCustomTUIAgent(
			agent.Name, cfg.DisplayName, cfg.Command, cfg.Description, cfg.Model, cfg.CommandArgs,
		); regErr != nil {
			log.Warn("failed to register custom TUI agent",
				zap.String("name", agent.Name), zap.Error(regErr))
		}
	}
}

// workflowStepGetterAdapter adapts workflow service to task service's WorkflowStepGetter interface.
// Since task service now uses wfmodels.WorkflowStep directly, the adapter simply delegates to the service.
type workflowStepGetterAdapter struct {
	svc *workflowservice.Service
}

// GetStep implements taskservice.WorkflowStepGetter.
func (a *workflowStepGetterAdapter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetStep(ctx, stepID)
}

// GetNextStepByPosition implements taskservice.WorkflowStepGetter.
func (a *workflowStepGetterAdapter) GetNextStepByPosition(ctx context.Context, boardID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetNextStepByPosition(ctx, boardID, currentPosition)
}

// startStepResolverAdapter adapts workflow service to task service's StartStepResolver interface.
type startStepResolverAdapter struct {
	svc *workflowservice.Service
}

// ResolveStartStep implements taskservice.StartStepResolver.
func (a *startStepResolverAdapter) ResolveStartStep(ctx context.Context, workflowID string) (string, error) {
	step, err := a.svc.ResolveStartStep(ctx, workflowID)
	if err != nil {
		return "", err
	}
	return step.ID, nil
}

// workflowProviderAdapter adapts task service to workflow service's WorkflowProvider interface.
type workflowProviderAdapter struct {
	svc *taskservice.Service
}

// ListWorkflows implements workflowservice.WorkflowProvider.
func (a *workflowProviderAdapter) ListWorkflows(ctx context.Context, workspaceID string) ([]*taskmodels.Workflow, error) {
	return a.svc.ListWorkflows(ctx, workspaceID)
}

// GetWorkflow implements workflowservice.WorkflowProvider.
func (a *workflowProviderAdapter) GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error) {
	return a.svc.GetWorkflow(ctx, id)
}

// CreateWorkflow implements workflowservice.WorkflowProvider.
func (a *workflowProviderAdapter) CreateWorkflow(ctx context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error) {
	return a.svc.CreateWorkflow(ctx, &taskservice.CreateWorkflowRequest{
		WorkspaceID: workspaceID,
		Name:        name,
		Description: description,
	})
}
