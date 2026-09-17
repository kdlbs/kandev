package backendapp

import (
	"context"
	"fmt"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/common/config"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/common/ports"
	"github.com/kandev/kandev/internal/orchestration/personas"
	orchestrationruntime "github.com/kandev/kandev/internal/orchestration/runtime"
	"github.com/kandev/kandev/internal/orchestrator"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	taskservice "github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"strings"
)

func newOrchestrationRuntime(cfg *config.Config, repos *Repositories, services *Services, orch *orchestrator.Service, cli string, log *logger.Logger) *orchestrationruntime.Service {
	apiPort := cfg.Server.Port
	if apiPort == 0 {
		apiPort = ports.Backend
	}
	personasSvc := &personas.Service{Profiles: repos.AgentSettings, Repo: repos.Orchestration, Terminate: func(ctx context.Context, id string) error {
		return orch.PersonaSessionTerminator().TerminateAllForAgent(ctx, id, "orchestrator_deleted")
	}}
	return &orchestrationruntime.Service{
		AssistantEnabled: cfg.Features.Orchestration && cfg.Features.PersonalAssistant,
		Repo:             repos.Orchestration, Personas: personasSvc, Runs: repos.Runs,
		Auth: runtimeauth.NewAgentAuth(""), Tasks: services.Task,
		Credentials: assistantCredentialReader{store: repos.Secrets},
		Manager:     &taskCreatorAdapter{taskSvc: services.Task, profiles: repos.AgentSettings, orch: orch, taskRepo: repos.Task, workflow: repos.Workflow},
		APIURL:      fmt.Sprintf("http://localhost:%d", apiPort), CLI: cli,
		Start: func(ctx context.Context, launch orchestrationruntime.Launch) error {
			return orch.StartTaskWithRoute(ctx, launch.TaskID, launch.PersonaID, orchexecutor.LaunchContext{ExecutorProfileID: launch.ExecutorID, Prompt: launch.Prompt, Env: launch.Env, OnSessionPrepared: launch.OnSessionPrepared}, orchexecutor.RouteOverride{ExecutionProfileID: launch.ProfileID})
		},
		UpdateStatus: func(ctx context.Context, ws, id, status string) error {
			return updateOrchestratedStatus(ctx, services.Task, repos, ws, id, status)
		},
	}
}
func updateOrchestratedStatus(ctx context.Context, tasks *taskservice.Service, repos *Repositories, ws, id, status string) error {
	task, err := tasks.GetTask(ctx, id)
	if err != nil || task.WorkspaceID != ws {
		return fmt.Errorf("task must belong to this workspace")
	}
	states := map[string]v1.TaskState{"done": v1.TaskStateCompleted, "COMPLETED": v1.TaskStateCompleted, "todo": v1.TaskStateTODO, "in_progress": v1.TaskStateInProgress, "in_review": v1.TaskStateReview, "review": v1.TaskStateReview}
	state, ok := states[status]
	if !ok {
		return fmt.Errorf("use done, todo, in_progress or in_review")
	}
	if state == v1.TaskStateCompleted {
		if err := validateOrchestratedCompletion(ctx, repos, task.WorkflowStepID, id); err != nil {
			return err
		}
	}
	_, err = tasks.UpdateTask(ctx, id, &taskservice.UpdateTaskRequest{State: &state})
	if err != nil {
		return err
	}
	if (state == v1.TaskStateTODO || state == v1.TaskStateInProgress) && (task.State == v1.TaskStateCompleted || task.State == v1.TaskStateReview) {
		return repos.Workflow.SupersedeTaskDecisions(ctx, id)
	}
	return nil
}

func validateOrchestratedCompletion(ctx context.Context, repos *Repositories, stepID, id string) error {
	participants, err := repos.Workflow.ListStepParticipantsForTask(ctx, stepID, id)
	if err != nil {
		return err
	}
	decisions, err := repos.Workflow.ListActiveTaskDecisions(ctx, id)
	if err != nil {
		return err
	}
	latest := map[string]string{}
	for _, decision := range decisions {
		latest[decision.ParticipantID] = strings.ToLower(decision.Decision)
	}
	for _, participant := range participants {
		if participant.DecisionRequired && latest[participant.ID] != "approved" {
			return fmt.Errorf("required review or approval is pending")
		}
	}
	return nil
}
