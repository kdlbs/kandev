package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	settingsmodels "github.com/kandev/kandev/internal/agent/settings/models"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	WorkflowChangeErrorInvalid       = "invalid_workflow_change"
	WorkflowChangeErrorAgentInvalid  = "workflow_agent_unavailable"
	WorkflowChangeErrorAgentIncompat = "workflow_agent_incompatible"
)

var ErrWorkflowChangeConflict = repoerrors.ErrWorkflowChangeConflict

type WorkflowChangeValidationError struct {
	Code            string
	SourceProfileID string
}

func (e *WorkflowChangeValidationError) Error() string {
	return "invalid workflow change"
}

func (e *WorkflowChangeValidationError) Unwrap() error {
	return ErrInvalidWorkflowChange
}

var ErrInvalidWorkflowChange = errors.New("invalid workflow change")

func invalidWorkflowChange(code, sourceProfileID string) error {
	return &WorkflowChangeValidationError{Code: code, SourceProfileID: sourceProfileID}
}

// ValidateWorkflowChange checks a read-only change form submission and returns
// its normalized destination override record. MoveTaskWithOptions repeats this
// validation and adds the transactional source guard before committing.
func (s *Service) ValidateWorkflowChange(
	ctx context.Context,
	taskID, workflowID, workflowStepID string,
	request *models.WorkflowChangeRequest,
) (*models.WorkflowAgentOverrides, error) {
	task, err := s.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.prepareWorkflowChange(ctx, task, workflowID, workflowStepID, request)
}

func (s *Service) prepareWorkflowChange(
	ctx context.Context,
	task *models.Task,
	workflowID, workflowStepID string,
	request *models.WorkflowChangeRequest,
) (*models.WorkflowAgentOverrides, error) {
	if err := validateWorkflowChangeSource(task, workflowID, workflowStepID, request); err != nil {
		return nil, err
	}
	// Project-backed tasks remain Office-owned if the IsFromOffice projection is missing.
	if task.ArchivedAt != nil || task.IsEphemeral || task.IsFromOffice || task.ProjectID != "" {
		return nil, invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	workflow, err := s.workflows.GetWorkflow(ctx, workflowID)
	if err != nil {
		return nil, invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	if workflow == nil || workflow.Hidden || workflow.WorkspaceID != task.WorkspaceID {
		return nil, invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	lister, ok := s.workflowStepGetter.(workflowStepLister)
	if !ok || lister == nil {
		return nil, invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	steps, err := lister.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	if !containsWorkflowChangeStep(steps, workflowStepID) {
		return nil, invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	return s.normalizeWorkflowChangeAgentOverrides(ctx, task, workflowID, steps, request.AgentOverrides)
}

func validateWorkflowChangeSource(
	task *models.Task,
	workflowID, workflowStepID string,
	request *models.WorkflowChangeRequest,
) error {
	if request == nil || task == nil || workflowID == "" || workflowStepID == "" ||
		strings.TrimSpace(request.ExpectedWorkflowID) == "" ||
		strings.TrimSpace(request.ExpectedStepID) == "" || request.ExpectedUpdatedAt.IsZero() || request.AgentOverrides == nil {
		return invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	if task.WorkflowID != request.ExpectedWorkflowID || task.WorkflowStepID != request.ExpectedStepID ||
		!task.UpdatedAt.Equal(request.ExpectedUpdatedAt) {
		return ErrWorkflowChangeConflict
	}
	if workflowID == task.WorkflowID {
		return invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	return nil
}

func containsWorkflowChangeStep(steps []*wfmodels.WorkflowStep, stepID string) bool {
	for _, step := range steps {
		if step != nil && step.ID == stepID {
			return true
		}
	}
	return false
}

func (s *Service) normalizeWorkflowChangeAgentOverrides(
	ctx context.Context,
	task *models.Task,
	workflowID string,
	steps []*wfmodels.WorkflowStep,
	requested map[string]string,
) (*models.WorkflowAgentOverrides, error) {
	fixedSteps := fixedWorkflowAgentOverrideSteps(workflowID, steps)
	requestedBySource, err := validateWorkflowChangeSources(requested, fixedSteps)
	if err != nil {
		return nil, err
	}
	if len(fixedSteps) == 0 {
		return nil, nil
	}
	executor, executorProfile, err := s.workflowChangeExecutor(ctx, task)
	if err != nil {
		return nil, invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	bindings := make([]models.WorkflowAgentOverrideBinding, 0, len(requested))
	validatedProfiles := make(map[string]struct{})
	sources := sortedWorkflowChangeSources(fixedSteps)
	for _, sourceID := range sources {
		replacementID := sourceID
		if requestedID, ok := requestedBySource[sourceID]; ok {
			replacementID = requestedID
		}
		profile, err := s.loadWorkflowChangeProfile(ctx, task.WorkspaceID, replacementID, sourceID)
		if err != nil {
			return nil, err
		}
		if _, ok := validatedProfiles[replacementID]; !ok {
			if s.agentProfileExecutorValidator == nil || s.agentProfileExecutorValidator.ValidateAgentProfileForExecutor(ctx, profile, executor, executorProfile) != nil {
				return nil, invalidWorkflowChange(WorkflowChangeErrorAgentIncompat, sourceID)
			}
			validatedProfiles[replacementID] = struct{}{}
		}
		if replacementID == sourceID {
			continue
		}
		for _, step := range fixedSteps[sourceID] {
			bindings = append(bindings, models.WorkflowAgentOverrideBinding{
				StepID: step.ID, SourceProfileID: sourceID, ReplacementProfileID: replacementID,
			})
		}
	}
	overrides, err := models.NewWorkflowAgentOverrides(workflowID, bindings)
	if err != nil {
		return nil, invalidWorkflowChange(WorkflowChangeErrorInvalid, "")
	}
	return overrides, nil
}

func validateWorkflowChangeSources(
	requested map[string]string,
	fixedSteps map[string][]*wfmodels.WorkflowStep,
) (map[string]string, error) {
	requestedBySource := make(map[string]string, len(requested))
	for rawSource, rawReplacement := range requested {
		source := strings.TrimSpace(rawSource)
		replacement := strings.TrimSpace(rawReplacement)
		if source == "" || replacement == "" || len(fixedSteps[source]) == 0 {
			return nil, invalidWorkflowChange(WorkflowChangeErrorAgentInvalid, source)
		}
		if _, exists := requestedBySource[source]; exists {
			return nil, invalidWorkflowChange(WorkflowChangeErrorAgentInvalid, source)
		}
		requestedBySource[source] = replacement
	}
	return requestedBySource, nil
}

func sortedWorkflowChangeSources(fixedSteps map[string][]*wfmodels.WorkflowStep) []string {
	sources := make([]string, 0, len(fixedSteps))
	for source := range fixedSteps {
		sources = append(sources, source)
	}
	sort.Strings(sources)
	return sources
}

func (s *Service) loadWorkflowChangeProfile(
	ctx context.Context,
	workspaceID, profileID, sourceID string,
) (*settingsmodels.AgentProfile, error) {
	if s.agentProfiles == nil {
		return nil, invalidWorkflowChange(WorkflowChangeErrorAgentInvalid, sourceID)
	}
	profile, err := s.agentProfiles.GetAgentProfile(ctx, profileID)
	if err != nil || profile == nil || profile.DeletedAt != nil || !profile.Enabled ||
		(profile.WorkspaceID != "" && profile.WorkspaceID != workspaceID) {
		return nil, invalidWorkflowChange(WorkflowChangeErrorAgentInvalid, sourceID)
	}
	return profile, nil
}

func (s *Service) workflowChangeExecutor(
	ctx context.Context,
	task *models.Task,
) (*models.Executor, *models.ExecutorProfile, error) {
	executorID := ""
	executorProfileID := ""
	if s.sessions != nil {
		if session, _ := s.sessions.GetPrimarySessionByTaskID(ctx, task.ID); session != nil {
			executorID = session.ExecutorID
			executorProfileID = session.ExecutorProfileID
		} else if session, _ := s.sessions.GetActiveTaskSessionByTaskID(ctx, task.ID); session != nil {
			executorID = session.ExecutorID
			executorProfileID = session.ExecutorProfileID
		}
	}
	if task.Metadata != nil {
		executorID = firstNonEmptyWorkflowAgentOverrideValue(executorID, models.StringFromAny(task.Metadata[models.MetaKeyExecutorID]))
		executorProfileID = firstNonEmptyWorkflowAgentOverrideValue(executorProfileID, models.StringFromAny(task.Metadata[models.MetaKeyExecutorProfileID]))
	}
	if s.executors == nil {
		return nil, nil, fmt.Errorf("executor repository unavailable")
	}
	executorProfile, executorID, err := s.resolveWorkflowAgentOverrideExecutorProfile(ctx, executorID, executorProfileID)
	if err != nil {
		return nil, nil, err
	}
	if executorID == "" {
		executorID, err = s.resolveWorkflowAgentOverrideWorkspaceExecutor(ctx, task.WorkspaceID)
		if err != nil {
			return nil, nil, err
		}
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	if err != nil || executor == nil || executor.DeletedAt != nil || executor.Status == models.ExecutorStatusDisabled {
		return nil, nil, fmt.Errorf("task executor is unavailable")
	}
	return executor, executorProfile, nil
}

func workflowChangeGuard(request *models.WorkflowChangeRequest) *models.WorkflowChangeSource {
	if request == nil {
		return nil
	}
	return &models.WorkflowChangeSource{
		WorkflowID: request.ExpectedWorkflowID,
		StepID:     request.ExpectedStepID,
		UpdatedAt:  request.ExpectedUpdatedAt.UTC(),
	}
}
