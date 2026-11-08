package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/kandev/kandev/internal/task/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// initialRecoveryPrompt is the current workflow composition for one explicit
// fresh-start recovery. Receipt content stays raw; only the dispatch prompt,
// trusted reference context, effective plan mode, and workflow-entry binding
// come from this value.
type initialRecoveryPrompt struct {
	content          string
	referenceContext string
	planMode         bool
	binding          *models.CeilingWorkflowEntryBinding
}

// initialRecoveryPromptPlan separates the two fresh-start input sources. A
// pending receipt keeps its raw content for transcript matching; a legacy
// description-only recovery transports the prepared prompt to the executor.
type initialRecoveryPromptPlan struct {
	receipt        *initialRecoveryPrompt
	legacyOverride *string
	binding        *models.CeilingWorkflowEntryBinding
}

// prepareInitialRecoveryPrompt resolves current workflow policy before fresh
// recovery clears provider identity. A nil plan means the current path has no
// recoverable initial prompt.
func (s *Service) prepareInitialRecoveryPrompt(
	ctx context.Context,
	taskID string,
	session *models.TaskSession,
	submission *models.InitialPromptSubmission,
) (*initialRecoveryPromptPlan, error) {
	if session == nil || session.TaskID != taskID {
		return nil, nil
	}
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load task for fresh recovery prompt: %w", err)
	}
	if task == nil || task.IsEphemeral {
		return nil, nil
	}
	if submission != nil {
		prepared, composeErr := s.composeFreshStartWorkflowPrompt(ctx, task, session, submission.Content)
		if composeErr != nil {
			return nil, composeErr
		}
		prepared.planMode = submission.PlanMode || prepared.planMode
		return &initialRecoveryPromptPlan{receipt: prepared, binding: prepared.binding}, nil
	}
	eligible, err := s.legacyFreshStartPromptEligible(ctx, task, session)
	if err != nil || !eligible {
		return nil, err
	}
	prepared, err := s.composeFreshStartWorkflowPrompt(ctx, task, session, task.Description)
	if err != nil {
		return nil, err
	}
	if prepared == nil {
		return nil, nil
	}
	prepared.content = s.effectivePromptForSessionWithConfigMode(
		session.ID, prepared.content, prepared.planMode, session, nil,
	)
	return &initialRecoveryPromptPlan{legacyOverride: &prepared.content, binding: prepared.binding}, nil
}

// legacyFreshStartPromptEligible reports whether the existing description
// fallback may receive the new workflow composition. Stored transcript rows and
// prompt history prove storage, not provider acceptance. An existing receipt or
// provider conversation identity excludes this path.
func (s *Service) legacyFreshStartPromptEligible(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
) (bool, error) {
	if task == nil || session == nil || !eligibleInitialSubmissionRecovery(session) {
		return false, nil
	}
	if sessionHasProviderSessionToken(session) {
		return false, nil
	}
	if strings.TrimSpace(task.Description) == "" {
		return false, nil
	}
	stored, exists, err := models.LoadInitialPromptSubmission(session.Metadata)
	if err != nil {
		return false, fmt.Errorf("load initial submission before fresh recovery prompt: %w", err)
	}
	if exists || stored != nil {
		return false, nil
	}
	running, err := s.repo.GetExecutorRunningBySessionID(ctx, session.ID)
	if err != nil && !errors.Is(err, models.ErrExecutorRunningNotFound) {
		return false, fmt.Errorf("load provider identity before fresh recovery prompt: %w", err)
	}
	if running != nil && strings.TrimSpace(running.ResumeToken) != "" {
		return false, nil
	}
	preserved, err := s.hasPreservedWorkflowAutoStartPrompt(ctx, session.ID)
	if err != nil {
		return false, err
	}
	if preserved {
		return false, nil
	}
	return true, nil
}

// hasPreservedWorkflowAutoStartPrompt keeps an automatic workflow queue entry
// authoritative for its own composed input. Queue storage or admission errors
// are reported so recovery cannot treat an unreadable queue as empty.
func (s *Service) hasPreservedWorkflowAutoStartPrompt(ctx context.Context, sessionID string) (bool, error) {
	if s.messageQueue == nil {
		return false, nil
	}
	entries, err := s.messageQueue.ListEntries(ctx, sessionID)
	if err != nil {
		return false, fmt.Errorf("read workflow queue ownership before fresh recovery prompt: %w", err)
	}
	for _, entry := range entries {
		if preserved, _ := entry.Metadata[metaKeyWorkflowAutoStart].(bool); preserved {
			return true, nil
		}
	}
	return false, nil
}

// composeFreshStartWorkflowPrompt reuses the first-launch workflow composer
// with strict policy reads. A missing step or unreadable workflow fails before
// recovery clears provider identity or dispatches an incomplete prompt.
func (s *Service) composeFreshStartWorkflowPrompt(
	ctx context.Context,
	task *models.Task,
	session *models.TaskSession,
	content string,
) (*initialRecoveryPrompt, error) {
	ctx = withWorkflowMetaCache(ctx)
	if task.WorkflowStepID == "" {
		if session.IsPassthrough {
			return &initialRecoveryPrompt{content: content}, nil
		}
		expanded, referenceContext := s.expandPromptReferencesWithContext(ctx, content, false)
		return &initialRecoveryPrompt{content: expanded, referenceContext: referenceContext}, nil
	}
	if s.workflowStepGetter == nil {
		return nil, errors.New("workflow step instructions are unavailable for fresh recovery")
	}
	step, err := s.workflowStepGetter.GetStep(ctx, task.WorkflowStepID)
	if err != nil {
		return nil, fmt.Errorf("load workflow step instructions for fresh recovery: %w", err)
	}
	if step == nil {
		return nil, errors.New("workflow step instructions are unavailable for fresh recovery")
	}
	workflowPrompt := ""
	if step.WorkflowID != "" {
		meta, err := s.getWorkflowMeta(ctx, step.WorkflowID)
		if err != nil {
			return nil, fmt.Errorf("load workflow instructions for fresh recovery: %w", err)
		}
		workflowPrompt = meta.Prompt
	}
	if err := s.validateFreshStartPromptReferences(ctx, workflowPrompt, step, content, session.IsPassthrough); err != nil {
		return nil, err
	}
	binding, err := s.initialRecoveryWorkflowBinding(ctx, task, step, session.ID)
	if err != nil {
		return nil, err
	}
	composed, referenceContext := s.buildWorkflowPromptWithContextOptions(
		ctx, content, step, task.ID, session.ID, session.IsPassthrough,
		false, false, false,
	)
	return &initialRecoveryPrompt{
		content:          composed,
		referenceContext: referenceContext,
		planMode:         step.HasOnEnterAction(wfmodels.OnEnterEnablePlanMode),
		binding:          binding,
	}, nil
}

// validateFreshStartPromptReferences checks the saved-prompt references that
// the first-launch composer will expand. Recovery must stop before reset when a
// required saved prompt cannot be read.
func (s *Service) validateFreshStartPromptReferences(
	ctx context.Context,
	workflowPrompt string,
	step *wfmodels.WorkflowStep,
	content string,
	isPassthrough bool,
) error {
	if step == nil || isPassthrough {
		return nil
	}
	parts := make([]string, 0, 3)
	if strings.TrimSpace(workflowPrompt) != "" {
		parts = append(parts, workflowPrompt)
	}
	if step.Prompt != "" {
		parts = append(parts, step.Prompt)
	}
	if step.Prompt == "" || strings.Contains(step.Prompt, "{{task_prompt}}") {
		parts = append(parts, content)
	}
	candidate := strings.Join(parts, "\n\n")
	if !strings.Contains(candidate, "@") {
		return nil
	}
	if _, _, err := s.expandPromptReferencesWithContextStrict(ctx, candidate, false); err != nil {
		return fmt.Errorf("resolve saved prompt instructions for fresh recovery: %w", err)
	}
	return nil
}

// initialRecoveryWorkflowBinding captures the workflow entry that authorized
// the prepared instructions. The destination session is intentionally omitted:
// the failed session is being recovered, and its resume-attempt identity owns
// the destination. The route, step, and transition identity remain fenced.
func (s *Service) initialRecoveryWorkflowBinding(
	ctx context.Context,
	task *models.Task,
	step *wfmodels.WorkflowStep,
	sessionID string,
) (*models.CeilingWorkflowEntryBinding, error) {
	if task == nil || step == nil || step.ID == "" {
		return nil, nil
	}
	if route, ok := models.LoadWorkflowSessionRoute(task.Metadata); ok &&
		route.DestinationStepID != "" && route.DestinationStepID != step.ID {
		return nil, fmt.Errorf("%w: %s", ErrCeilingLaunchSuperseded, ceilingEntryDetailStepChanged)
	}
	binding, bound, err := s.deriveCeilingEntryBinding(
		ctx, task, map[string]interface{}{metaKeyWorkflowStepID: step.ID}, sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("load workflow entry identity for fresh recovery: %w", err)
	}
	if !bound || !binding.Valid() {
		return nil, fmt.Errorf("%w: workflow entry binding is unavailable", ErrCeilingLaunchSuperseded)
	}
	if binding.DestinationSessionID != "" && binding.DestinationSessionID != sessionID {
		return nil, fmt.Errorf("%w: workflow destination session changed", ErrCeilingLaunchSuperseded)
	}
	binding.DestinationSessionID = ""
	return &binding, nil
}
