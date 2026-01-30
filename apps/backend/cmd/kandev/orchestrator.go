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

	msgCreator := &messageCreatorAdapter{svc: taskSvc}
	orchestratorSvc.SetMessageCreator(msgCreator)

	orchestratorSvc.SetTurnService(newTurnServiceAdapter(taskSvc))

	// Wire workflow step getter for prompt building
	if workflowSvc != nil {
		orchestratorSvc.SetWorkflowStepGetter(&orchestratorWorkflowStepGetterAdapter{svc: workflowSvc})
	}

	return orchestratorSvc, msgCreator, nil
}

// orchestratorWorkflowStepGetterAdapter adapts workflow service to orchestrator's WorkflowStepGetter interface.
type orchestratorWorkflowStepGetterAdapter struct {
	svc *workflowservice.Service
}

// GetStep implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetStep(ctx context.Context, stepID string) (*orchestrator.WorkflowStep, error) {
	step, err := a.svc.GetStep(ctx, stepID)
	if err != nil {
		return nil, err
	}
	onCompleteStepID := ""
	if step.OnCompleteStepID != nil {
		onCompleteStepID = *step.OnCompleteStepID
	}
	onApprovalStepID := ""
	if step.OnApprovalStepID != nil {
		onApprovalStepID = *step.OnApprovalStepID
	}
	return &orchestrator.WorkflowStep{
		ID:               step.ID,
		Name:             step.Name,
		StepType:         string(step.StepType),
		AutoStartAgent:   step.AutoStartAgent,
		PlanMode:         step.PlanMode,
		RequireApproval:  step.RequireApproval,
		PromptPrefix:     step.PromptPrefix,
		PromptSuffix:     step.PromptSuffix,
		OnCompleteStepID: onCompleteStepID,
		OnApprovalStepID: onApprovalStepID,
	}, nil
}

// GetSourceStep implements orchestrator.WorkflowStepGetter.
func (a *orchestratorWorkflowStepGetterAdapter) GetSourceStep(ctx context.Context, boardID, targetStepID string) (*orchestrator.WorkflowStep, error) {
	step, err := a.svc.GetSourceStep(ctx, boardID, targetStepID)
	if err != nil {
		return nil, err
	}
	if step == nil {
		return nil, nil
	}
	onCompleteStepID := ""
	if step.OnCompleteStepID != nil {
		onCompleteStepID = *step.OnCompleteStepID
	}
	onApprovalStepID := ""
	if step.OnApprovalStepID != nil {
		onApprovalStepID = *step.OnApprovalStepID
	}
	return &orchestrator.WorkflowStep{
		ID:               step.ID,
		Name:             step.Name,
		StepType:         string(step.StepType),
		AutoStartAgent:   step.AutoStartAgent,
		PlanMode:         step.PlanMode,
		RequireApproval:  step.RequireApproval,
		PromptPrefix:     step.PromptPrefix,
		PromptSuffix:     step.PromptSuffix,
		OnCompleteStepID: onCompleteStepID,
		OnApprovalStepID: onApprovalStepID,
	}, nil
}
