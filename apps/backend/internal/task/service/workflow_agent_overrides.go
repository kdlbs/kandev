package service

import (
	"context"
	"database/sql"
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

// ErrExplicitAgentProfileRequired rejects a task creation whose resolved
// workflow opts into require_explicit_agent_profile when the caller supplied
// neither an available explicit agent_profile_id nor valid
// workflow_agent_overrides selection. It is a validation failure, not a server fault.
var ErrExplicitAgentProfileRequired = errors.New("explicit agent profile is required by this workflow")

// enforceExplicitAgentProfilePolicy rejects a create before any task,
// repository, or session row is written when the resolved workflow requires an
// usable explicit agent profile selection. Only a
// caller-provided profile (ExplicitAgentProfileID) or a validated
// workflow_agent_overrides binding for the task's resolved destination step
// counts: creator/parent/workflow/workspace fallbacks and stored task
// metadata deliberately do not, and neither does a binding for a different
// step, so a workflow that requires an explicit choice cannot be satisfied by
// an inherited default or by a selection that never applies to the step the
// task actually lands on.
//
// workflow is the value validateTaskWorkflow already loaded for this create,
// so the policy does not repeat that read and cannot add a new failure mode to
// the policy-off path. destinationStepID and resolveErr are the step
// prepareTaskForCreation already resolved for this create and its resolver
// outcome: passing them in (instead of re-resolving here) keeps the policy
// check and the task row on the same step. A resolver failure is propagated
// only when the destination is actually needed (opt-in workflow with validated
// overrides); every other create keeps the historical tolerant behavior, so a
// transient lookup failure outside this policy never fails the create. It is
// never reported as a missing explicit profile.
//
// It runs after validateWorkflowAgentOverrides so normalized overrides are
// available, and after CreateTask's external_id short-circuit, so a duplicate
// retry returns the existing task without re-evaluating this policy.
func (s *Service) enforceExplicitAgentProfilePolicy(ctx context.Context, req *CreateTaskRequest, workflow *models.Workflow, destinationStepID string, resolveErr error) error {
	if req.WorkflowID == "" || workflow == nil || !workflow.RequireExplicitAgentProfile {
		return nil
	}
	if strings.TrimSpace(req.ExplicitAgentProfileID) != "" {
		return s.validateExplicitProfileSelection(ctx, req)
	}
	if req.normalizedWorkflowAgentOverrides != nil && len(req.normalizedWorkflowAgentOverrides.Steps) > 0 {
		if resolveErr != nil {
			return resolveErr
		}
		if destinationStepID != "" {
			if _, ok := req.normalizedWorkflowAgentOverrides.ReplacementFor(req.WorkflowID, destinationStepID); ok {
				return nil
			}
		}
		return ErrExplicitAgentProfileRequired
	}
	return ErrExplicitAgentProfileRequired
}

func (s *Service) validateExplicitProfileSelection(ctx context.Context, req *CreateTaskRequest) error {
	if s.agentProfiles == nil {
		return fmt.Errorf("%w: profile validation is unavailable", ErrExplicitAgentProfileRequired)
	}
	profile, err := s.agentProfiles.GetAgentProfile(ctx, req.ExplicitAgentProfileID)
	if err != nil {
		return fmt.Errorf("%w: could not validate selected profile: %v", ErrExplicitAgentProfileRequired, err)
	}
	if profile == nil || !profile.Enabled || profile.DeletedAt != nil ||
		(profile.WorkspaceID != "" && profile.WorkspaceID != req.WorkspaceID) {
		return fmt.Errorf("%w: selected profile is unavailable in this workspace", ErrExplicitAgentProfileRequired)
	}
	return nil
}

// ResolveWorkflowAgentOverrideReplacement resolves the replacement profile a
// requested workflow_agent_overrides map applies to stepID, reusing the same
// normalization and validation the create path uses (source-key trimming,
// SessionTarget exclusion, replacement eligibility). A pre-create adapter (the
// MCP create handler) uses it so the profile it reports and records matches the
// task-aware runtime resolver instead of re-deriving the map's rules.
func (s *Service) ResolveWorkflowAgentOverrideReplacement(
	ctx context.Context, workspaceID, workflowID, stepID string, requested map[string]string,
) (string, bool, error) {
	workflowID = strings.TrimSpace(workflowID)
	stepID = strings.TrimSpace(stepID)
	if len(requested) == 0 || workflowID == "" || stepID == "" {
		return "", false, nil
	}
	lister, ok := s.workflowStepGetter.(workflowStepLister)
	if !ok || lister == nil {
		return "", false, fmt.Errorf("%w: workflow step listing unavailable", models.ErrInvalidWorkflowAgentOverrides)
	}
	steps, err := lister.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return "", false, fmt.Errorf("%w: list workflow steps: %v", models.ErrInvalidWorkflowAgentOverrides, err)
	}
	normalized, err := normalizeWorkflowAgentOverrides(ctx, workspaceID, workflowID, requested, steps, s.agentProfiles)
	if err != nil {
		return "", false, err
	}
	if normalized == nil {
		return "", false, nil
	}
	replacement, ok := normalized.ReplacementFor(workflowID, stepID)
	return replacement, ok, nil
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
// assignee_agent_profile_id that does not name an Office agent instance
// scoped to the task's own workspace.
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
// Unlike that sibling check, this one does not gate on profile.Enabled:
// ListAgentInstances (the picker the dialog's assignee list is drawn from)
// never filters on it either, so a profile the picker offers must remain
// assignable here.
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
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("%w: %q is unavailable", ErrInvalidAssigneeAgentProfile, assigneeID)
		}
		return fmt.Errorf("look up assignee agent profile %q: %w", assigneeID, err)
	}
	if profile == nil || profile.DeletedAt != nil {
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
