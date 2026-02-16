package main

import (
	"context"
	"errors"

	"github.com/kandev/kandev/internal/agent/lifecycle"
	"github.com/kandev/kandev/internal/agent/registry"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/repository"
	taskservice "github.com/kandev/kandev/internal/task/service"
	userservice "github.com/kandev/kandev/internal/user/service"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	workflowservice "github.com/kandev/kandev/internal/workflow/service"
	"github.com/kandev/kandev/internal/worktree"
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
	worktreeRecreator *worktree.Recreator,
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
	if workflowSvc != nil {
		orchestratorSvc.SetWorkflowStepGetter(&orchestratorWorkflowStepGetterAdapter{svc: workflowSvc})
	}

	// Wire worktree recreator for handling missing worktrees during session resume
	if worktreeRecreator != nil {
		orchestratorSvc.SetWorktreeRecreator(newWorktreeRecreatorAdapter(worktreeRecreator))
	}

	return orchestratorSvc, msgCreator, nil
}

// orchestratorWorkflowStepGetterAdapter adapts workflow service to orchestrator's WorkflowStepGetter interface.
// Since orchestrator now uses wfmodels.WorkflowStep directly, the adapter simply delegates to the service.
type orchestratorWorkflowStepGetterAdapter struct {
	svc *workflowservice.Service
}

// GetStep implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetStep(ctx context.Context, stepID string) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetStep(ctx, stepID)
}

// GetNextStepByPosition implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetNextStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetNextStepByPosition(ctx, workflowID, currentPosition)
}

// GetPreviousStepByPosition implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetPreviousStepByPosition(ctx context.Context, workflowID string, currentPosition int) (*wfmodels.WorkflowStep, error) {
	return a.svc.GetPreviousStepByPosition(ctx, workflowID, currentPosition)
}
