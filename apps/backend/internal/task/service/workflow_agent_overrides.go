package service

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

func normalizeWorkflowAgentOverrides(
	ctx context.Context,
	workspaceID, workflowID string,
	requested map[string]string,
	steps []*wfmodels.WorkflowStep,
	profiles AgentProfileReader,
) (*models.WorkflowAgentOverrides, error) {
	if len(requested) == 0 {
		return nil, nil
	}
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" {
		return nil, fmt.Errorf("%w: workflow_id is required", models.ErrInvalidWorkflowAgentOverrides)
	}

	fixedStepsBySource := fixedWorkflowAgentOverrideSteps(workflowID, steps)

	sources := make([]string, 0, len(requested))
	for sourceProfileID := range requested {
		sources = append(sources, sourceProfileID)
	}
	sort.Strings(sources)
	bindings := make([]models.WorkflowAgentOverrideBinding, 0, len(requested))
	for _, rawSourceProfileID := range sources {
		nextBindings, err := normalizeWorkflowAgentOverrideSource(
			ctx,
			workspaceID,
			workflowID,
			rawSourceProfileID,
			requested[rawSourceProfileID],
			fixedStepsBySource,
			profiles,
		)
		if err != nil {
			return nil, err
		}
		bindings = append(bindings, nextBindings...)
	}
	return models.NewWorkflowAgentOverrides(workflowID, bindings)
}

func fixedWorkflowAgentOverrideSteps(
	workflowID string,
	steps []*wfmodels.WorkflowStep,
) map[string][]*wfmodels.WorkflowStep {
	fixedStepsBySource := make(map[string][]*wfmodels.WorkflowStep)
	for _, step := range steps {
		if step == nil || step.WorkflowID != workflowID || step.SessionTarget != nil {
			continue
		}
		sourceProfileID := strings.TrimSpace(step.AgentProfileID)
		if sourceProfileID == "" {
			continue
		}
		fixedStepsBySource[sourceProfileID] = append(fixedStepsBySource[sourceProfileID], step)
	}
	for sourceProfileID := range fixedStepsBySource {
		sort.SliceStable(fixedStepsBySource[sourceProfileID], func(i, j int) bool {
			return fixedStepsBySource[sourceProfileID][i].ID < fixedStepsBySource[sourceProfileID][j].ID
		})
	}
	return fixedStepsBySource
}

func normalizeWorkflowAgentOverrideSource(
	ctx context.Context,
	workspaceID, workflowID, rawSourceProfileID, rawReplacementProfileID string,
	fixedStepsBySource map[string][]*wfmodels.WorkflowStep,
	profiles AgentProfileReader,
) ([]models.WorkflowAgentOverrideBinding, error) {
	sourceProfileID := strings.TrimSpace(rawSourceProfileID)
	replacementProfileID := strings.TrimSpace(rawReplacementProfileID)
	if sourceProfileID == "" || replacementProfileID == "" {
		return nil, fmt.Errorf("%w: source and replacement profile IDs are required", models.ErrInvalidWorkflowAgentOverrides)
	}
	fixedSteps := fixedStepsBySource[sourceProfileID]
	if len(fixedSteps) == 0 {
		return nil, fmt.Errorf("%w: source profile %q is not a fixed profile in workflow %q", models.ErrInvalidWorkflowAgentOverrides, sourceProfileID, workflowID)
	}
	if sourceProfileID == replacementProfileID {
		return nil, nil
	}
	if profiles == nil {
		return nil, fmt.Errorf("%w: profile validation unavailable", models.ErrInvalidWorkflowAgentOverrides)
	}
	profile, err := profiles.GetAgentProfile(ctx, replacementProfileID)
	if err != nil {
		return nil, fmt.Errorf("%w: replacement profile %q: %v", models.ErrInvalidWorkflowAgentOverrides, replacementProfileID, err)
	}
	if profile == nil || profile.DeletedAt != nil || !profile.Enabled {
		return nil, fmt.Errorf("%w: replacement profile %q is unavailable", models.ErrInvalidWorkflowAgentOverrides, replacementProfileID)
	}
	if profile.WorkspaceID != "" && profile.WorkspaceID != workspaceID {
		return nil, fmt.Errorf("%w: replacement profile %q is outside the task workspace", models.ErrInvalidWorkflowAgentOverrides, replacementProfileID)
	}
	bindings := make([]models.WorkflowAgentOverrideBinding, 0, len(fixedSteps))
	for _, step := range fixedSteps {
		bindings = append(bindings, models.WorkflowAgentOverrideBinding{
			StepID:               step.ID,
			SourceProfileID:      sourceProfileID,
			ReplacementProfileID: replacementProfileID,
		})
	}
	return bindings, nil
}

func (s *Service) validateWorkflowAgentOverrides(ctx context.Context, req *CreateTaskRequest) error {
	req.normalizedWorkflowAgentOverrides = nil
	if len(req.WorkflowAgentOverrides) == 0 {
		return nil
	}
	if req.WorkflowID == "" {
		return fmt.Errorf("%w: workflow_id is required", models.ErrInvalidWorkflowAgentOverrides)
	}
	lister, ok := s.workflowStepGetter.(workflowStepLister)
	if !ok || lister == nil {
		return fmt.Errorf("%w: workflow step listing unavailable", models.ErrInvalidWorkflowAgentOverrides)
	}
	steps, err := lister.ListStepsByWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return fmt.Errorf("%w: list workflow steps: %v", models.ErrInvalidWorkflowAgentOverrides, err)
	}
	normalized, err := normalizeWorkflowAgentOverrides(ctx, req.WorkspaceID, req.WorkflowID, req.WorkflowAgentOverrides, steps, s.agentProfiles)
	if err != nil {
		return err
	}
	if normalized == nil || len(normalized.Steps) == 0 {
		req.normalizedWorkflowAgentOverrides = normalized
		return nil
	}
	executor, executorProfile, err := s.resolveWorkflowAgentOverrideExecutor(ctx, req)
	if err != nil {
		return fmt.Errorf("%w: %v", models.ErrInvalidWorkflowAgentOverrides, err)
	}
	if s.agentProfileExecutorValidator == nil {
		return fmt.Errorf("%w: executor compatibility validation unavailable", models.ErrInvalidWorkflowAgentOverrides)
	}
	if err := s.validateWorkflowAgentOverrideProfiles(ctx, normalized, executor, executorProfile); err != nil {
		return err
	}
	req.normalizedWorkflowAgentOverrides = normalized
	return nil
}

func (s *Service) validateWorkflowAgentOverrideProfiles(
	ctx context.Context,
	overrides *models.WorkflowAgentOverrides,
	executor *models.Executor,
	executorProfile *models.ExecutorProfile,
) error {
	validatedProfiles := make(map[string]struct{})
	for _, binding := range overrides.Steps {
		if _, ok := validatedProfiles[binding.ReplacementProfileID]; ok {
			continue
		}
		profile, err := s.agentProfiles.GetAgentProfile(ctx, binding.ReplacementProfileID)
		if err != nil {
			return fmt.Errorf("%w: load replacement profile %q for executor validation: %v", models.ErrInvalidWorkflowAgentOverrides, binding.ReplacementProfileID, err)
		}
		if profile == nil {
			return fmt.Errorf("%w: replacement profile %q is unavailable", models.ErrInvalidWorkflowAgentOverrides, binding.ReplacementProfileID)
		}
		if err := s.agentProfileExecutorValidator.ValidateAgentProfileForExecutor(ctx, profile, executor, executorProfile); err != nil {
			return fmt.Errorf("%w: replacement profile %q is incompatible with the selected executor: %w", models.ErrInvalidWorkflowAgentOverrides, binding.ReplacementProfileID, err)
		}
		validatedProfiles[binding.ReplacementProfileID] = struct{}{}
	}
	return nil
}

func workflowAgentOverrideStringValue(values map[string]interface{}, key string) string {
	value, ok := values[key]
	if !ok {
		return ""
	}
	stringValue, _ := value.(string)
	return strings.TrimSpace(stringValue)
}

func firstNonEmptyWorkflowAgentOverrideValue(current, candidate string) string {
	if current != "" {
		return current
	}
	return candidate
}

func mergeWorkflowAgentOverrideExecutorIDs(values map[string]interface{}, executorID, executorProfileID *string) {
	*executorID = firstNonEmptyWorkflowAgentOverrideValue(
		*executorID,
		workflowAgentOverrideStringValue(values, models.MetaKeyExecutorID),
	)
	*executorProfileID = firstNonEmptyWorkflowAgentOverrideValue(
		*executorProfileID,
		workflowAgentOverrideStringValue(values, models.MetaKeyExecutorProfileID),
	)
}

func workflowAgentOverrideExecutorIDs(req *CreateTaskRequest) (string, string) {
	executorID := strings.TrimSpace(req.ExecutorID)
	executorProfileID := strings.TrimSpace(req.ExecutorProfileID)
	metadata := req.Metadata
	deferred, _ := metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	for _, values := range []map[string]interface{}{req.DeferredLaunch, metadata, deferred} {
		mergeWorkflowAgentOverrideExecutorIDs(values, &executorID, &executorProfileID)
	}
	return executorID, executorProfileID
}

func (s *Service) resolveWorkflowAgentOverrideExecutor(
	ctx context.Context,
	req *CreateTaskRequest,
) (*models.Executor, *models.ExecutorProfile, error) {
	executorID, executorProfileID := workflowAgentOverrideExecutorIDs(req)
	if s.executors == nil {
		return nil, nil, fmt.Errorf("executor repository unavailable")
	}
	executorProfile, executorID, err := s.resolveWorkflowAgentOverrideExecutorProfile(
		ctx,
		executorID,
		executorProfileID,
	)
	if err != nil {
		return nil, nil, err
	}
	if executorID == "" {
		executorID, err = s.resolveWorkflowAgentOverrideWorkspaceExecutor(ctx, req.WorkspaceID)
		if err != nil {
			return nil, nil, err
		}
	}
	executor, err := s.executors.GetExecutor(ctx, executorID)
	if err != nil {
		return nil, nil, fmt.Errorf("load executor %q: %w", executorID, err)
	}
	if executor == nil || executor.DeletedAt != nil || executor.Status == models.ExecutorStatusDisabled {
		return nil, nil, fmt.Errorf("executor %q is unavailable", executorID)
	}
	return executor, executorProfile, nil
}

func (s *Service) resolveWorkflowAgentOverrideExecutorProfile(
	ctx context.Context,
	executorID, executorProfileID string,
) (*models.ExecutorProfile, string, error) {
	if executorProfileID == "" {
		return nil, executorID, nil
	}
	executorProfile, err := s.executors.GetExecutorProfile(ctx, executorProfileID)
	if err != nil {
		return nil, "", fmt.Errorf("load executor profile %q: %w", executorProfileID, err)
	}
	if executorProfile == nil || strings.TrimSpace(executorProfile.ExecutorID) == "" {
		return nil, "", fmt.Errorf("executor profile %q is unavailable", executorProfileID)
	}
	if executorID != "" && executorID != executorProfile.ExecutorID {
		return nil, "", fmt.Errorf("executor profile %q does not belong to executor %q", executorProfileID, executorID)
	}
	return executorProfile, executorProfile.ExecutorID, nil
}

// ErrInvalidAssigneeAgentProfile identifies a create-time
// assignee_agent_profile_id that does not name an enabled Office agent
// instance scoped to the task's own workspace.
var ErrInvalidAssigneeAgentProfile = errors.New("invalid assignee_agent_profile_id")

// ValidateAssigneeAgentProfile checks a caller-supplied create-time assignee
// before any task row is written. prepareTaskForCreation calls this only when
// req.RequireAssigneeAgentProfileValidation is set — the untrusted HTTP
// create-task handler is the only caller that sets it, because
// CreateTaskRequest.AssigneeAgentProfileID is also used by trusted internal
// callers (agent-created subtasks, onboarding, routines) that already know
// the profile is valid and must not be re-gated by a workspace-scoped Office
// eligibility rule that doesn't apply to them. Gating inside
// prepareTaskForCreation (rather than validating in the handler before
// CreateTask runs) also means a duplicate external_id request short-circuits
// to the existing task before this validation ever runs against it.
//
// Office eligibility mirrors ListAgentInstances' own filter
// (agentInstanceFilter, workspace_id != ” AND deleted_at IS NULL, plus a
// workspace_id match): a global/kanban-legacy profile (WorkspaceID == "") is
// deliberately rejected here even though normalizeWorkflowAgentOverrideSource
// above treats it as universally allowed — the New Task dialog never offers a
// global profile as an assignee, so this path holds to the stricter rule.
func (s *Service) ValidateAssigneeAgentProfile(ctx context.Context, workspaceID, assigneeAgentProfileID string) error {
	assigneeID := strings.TrimSpace(assigneeAgentProfileID)
	if assigneeID == "" {
		return nil
	}
	if s.agentProfiles == nil {
		return fmt.Errorf("%w: profile validation unavailable", ErrInvalidAssigneeAgentProfile)
	}
	profile, err := s.agentProfiles.GetAgentProfile(ctx, assigneeID)
	if err != nil {
		return fmt.Errorf("%w: %q: %v", ErrInvalidAssigneeAgentProfile, assigneeID, err)
	}
	if profile == nil || profile.DeletedAt != nil || !profile.Enabled {
		return fmt.Errorf("%w: %q is unavailable", ErrInvalidAssigneeAgentProfile, assigneeID)
	}
	if profile.WorkspaceID == "" || profile.WorkspaceID != workspaceID {
		return fmt.Errorf("%w: %q is not an office agent in this workspace", ErrInvalidAssigneeAgentProfile, assigneeID)
	}
	return nil
}

func (s *Service) resolveWorkflowAgentOverrideWorkspaceExecutor(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	if s.workspaces == nil {
		return "", fmt.Errorf("workspace repository unavailable")
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID)
	if err != nil {
		return "", fmt.Errorf("load workspace default executor: %w", err)
	}
	if workspace == nil || workspace.DefaultExecutorID == nil || strings.TrimSpace(*workspace.DefaultExecutorID) == "" {
		return "", fmt.Errorf("effective executor is not configured")
	}
	return strings.TrimSpace(*workspace.DefaultExecutorID), nil
}
