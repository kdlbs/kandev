package github

import (
	"context"
	"fmt"
	"strings"

	promptcfg "github.com/kandev/kandev/config/prompts"
)

// GetTaskCIOptionsResponse returns task CI automation options plus effective prompt text.
func (s *Service) GetTaskCIOptionsResponse(ctx context.Context, taskID string) (*TaskCIOptionsResponse, error) {
	if s.store == nil {
		return nil, errStoreUnavailable
	}
	opts, err := s.store.GetTaskCIOptions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.buildTaskCIOptionsResponse(ctx, opts)
}

// UpdateTaskCIOptions updates task CI automation options and returns the response shape.
func (s *Service) UpdateTaskCIOptions(ctx context.Context, taskID string, patch TaskCIOptionsPatch) (*TaskCIOptionsResponse, error) {
	if s.store == nil {
		return nil, errStoreUnavailable
	}
	if err := s.populateReviewReviewer(ctx, taskID, &patch); err != nil {
		return nil, err
	}
	opts, err := s.store.UpdateTaskCIOptions(ctx, taskID, patch)
	if err != nil {
		return nil, err
	}
	return s.buildTaskCIOptionsResponse(ctx, opts)
}

func (s *Service) populateReviewReviewer(
	ctx context.Context,
	taskID string,
	patch *TaskCIOptionsPatch,
) error {
	if patch.PromptOnReviewRequested == nil {
		return nil
	}
	if !*patch.PromptOnReviewRequested {
		empty := ""
		patch.ReviewReviewerLogin = &empty
		return nil
	}
	resolved, err := s.resolveTaskPRAutomationClient(ctx, taskID)
	if err != nil {
		return err
	}
	login, err := resolved.Client.GetAuthenticatedUser(ctx)
	if err != nil {
		return fmt.Errorf("resolve authenticated reviewer: %w", err)
	}
	patch.ReviewReviewerLogin = &login
	return nil
}

// GetTaskCIPRState returns per-PR CI automation state, or nil.
func (s *Service) GetTaskCIPRState(ctx context.Context, taskID, repositoryID string, prNumber int) (*TaskCIPRAutomationState, error) {
	if s.store == nil {
		return nil, errStoreUnavailable
	}
	return s.store.GetTaskCIPRState(ctx, taskID, repositoryID, prNumber)
}

// RecordTaskCIFixAttempt records an auto-fix attempt.
func (s *Service) RecordTaskCIFixAttempt(ctx context.Context, attempt TaskCIFixAttempt) error {
	if s.store == nil {
		return errStoreUnavailable
	}
	return s.store.RecordTaskCIFixAttempt(ctx, attempt)
}

// RefreshTaskCIFixCheckpoint records the current CI checkpoint without recording a prompt dispatch.
func (s *Service) RefreshTaskCIFixCheckpoint(ctx context.Context, taskID, repositoryID string, prNumber int, signature, checkpointJSON string) error {
	if s.store == nil {
		return errStoreUnavailable
	}
	return s.store.RefreshTaskCIFixCheckpoint(ctx, taskID, repositoryID, prNumber, signature, checkpointJSON)
}

// RecordTaskCIMergeAttempt records an auto-merge attempt.
func (s *Service) RecordTaskCIMergeAttempt(ctx context.Context, attempt TaskCIMergeAttempt) error {
	if s.store == nil {
		return errStoreUnavailable
	}
	return s.store.RecordTaskCIMergeAttempt(ctx, attempt)
}

// RecordTaskCIError records a CI automation error.
func (s *Service) RecordTaskCIError(ctx context.Context, taskID, repositoryID string, prNumber int, message string) error {
	if s.store == nil {
		return errStoreUnavailable
	}
	return s.store.RecordTaskCIError(ctx, taskID, repositoryID, prNumber, message)
}

// MarkTaskCIAutoFixExhausted records that auto-fix reached its per-PR round cap.
func (s *Service) MarkTaskCIAutoFixExhausted(ctx context.Context, taskID, repositoryID string, prNumber int, message string) error {
	if s.store == nil {
		return errStoreUnavailable
	}
	return s.store.MarkTaskCIAutoFixExhausted(ctx, taskID, repositoryID, prNumber, message)
}

// ClearTaskCIError clears a CI automation error.
func (s *Service) ClearTaskCIError(ctx context.Context, taskID, repositoryID string, prNumber int) error {
	if s.store == nil {
		return errStoreUnavailable
	}
	return s.store.ClearTaskCIError(ctx, taskID, repositoryID, prNumber)
}

func (s *Service) IsReviewRequestedForLogin(
	ctx context.Context, workspaceID, owner, repo string, prNumber int, login string,
) (bool, error) {
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, owner, repo)
	if err != nil {
		return false, err
	}
	pr, err := resolved.Client.GetPR(ctx, owner, repo, prNumber)
	if err != nil {
		return false, err
	}
	for _, reviewer := range pr.RequestedReviewers {
		if reviewer.Type == reviewerTypeUser && strings.EqualFold(reviewer.Login, login) {
			return true, nil
		}
	}
	return false, nil
}

func (s *Service) HasEnabledTaskPRAgentPrompts(ctx context.Context, taskID string) (bool, error) {
	opts, err := s.store.GetTaskCIOptions(ctx, taskID)
	if err != nil {
		return false, err
	}
	return opts.PromptOnReviewRequested || opts.PromptOnMerged || opts.PromptOnClosed, nil
}

func (s *Service) SetTaskPRReviewRequestState(
	ctx context.Context, taskID, repositoryID string, prNumber int, requested bool,
) error {
	return s.store.SetTaskPRReviewRequestState(ctx, taskID, repositoryID, prNumber, requested)
}

// RebindTaskPRReviewer resolves the current GitHub login and atomically resets
// the task's review-request baselines if the connected account changed.
func (s *Service) RebindTaskPRReviewer(ctx context.Context, taskID string) (string, bool, error) {
	resolved, err := s.resolveTaskPRAutomationClient(ctx, taskID)
	if err != nil {
		return "", false, err
	}
	login, err := resolved.Client.GetAuthenticatedUser(ctx)
	if err != nil {
		return "", false, fmt.Errorf("resolve authenticated reviewer: %w", err)
	}
	changed, err := s.store.RebindTaskPRReviewer(ctx, taskID, login)
	if err != nil {
		return "", false, err
	}
	return login, changed, nil
}

func (s *Service) resolveTaskPRAutomationClient(
	ctx context.Context,
	taskID string,
) (*resolvedServiceClient, error) {
	prs, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("load linked PR for authenticated reviewer: %w", err)
	}
	if len(prs) == 0 {
		return nil, fmt.Errorf("resolve authenticated reviewer: task has no linked pull request")
	}
	pr := prs[0]
	for _, candidate := range prs {
		if strings.TrimSpace(candidate.WorkspaceID) != "" {
			pr = candidate
			break
		}
	}
	workspaceID := strings.TrimSpace(pr.WorkspaceID)
	if workspaceID == "" {
		workspaceID, err = s.resolveTaskWatchWorkspace(ctx, taskID)
		if err != nil {
			return nil, err
		}
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, pr.Owner, pr.Repo)
	if err != nil {
		return nil, fmt.Errorf("resolve authenticated reviewer: %w", err)
	}
	return resolved, nil
}

// resolveTaskWatchWorkspace recovers the workspace for linked PR rows written by
// the workspace-less association path. The watch belongs to the same task, so
// reviewer identity still resolves inside that task's own workspace and never
// falls back to an ambient client.
func (s *Service) resolveTaskWatchWorkspace(ctx context.Context, taskID string) (string, error) {
	watch, err := s.store.GetPRWatchByTask(ctx, taskID)
	if err != nil {
		return "", fmt.Errorf("resolve authenticated reviewer workspace: %w", err)
	}
	if watch == nil || strings.TrimSpace(watch.WorkspaceID) == "" {
		return "", fmt.Errorf("resolve authenticated reviewer: %w", ErrGitHubWorkspaceRequired)
	}
	return strings.TrimSpace(watch.WorkspaceID), nil
}

func (s *Service) SetTaskPRObservedState(
	ctx context.Context, taskID, repositoryID string, prNumber int, state string,
) error {
	return s.store.SetTaskPRObservedState(ctx, taskID, repositoryID, prNumber, state)
}

func (s *Service) RecordTaskPRLifecyclePrompt(ctx context.Context, prompt TaskPRLifecyclePrompt) error {
	return s.store.RecordTaskPRLifecyclePrompt(ctx, prompt)
}

// ShouldHoldTerminalPRWatch keeps a terminal PR attached until its subscribed
// lifecycle prompt has been accepted or durably queued.
func (s *Service) ShouldHoldTerminalPRWatch(
	ctx context.Context, taskID, repositoryID string, prNumber int, state string,
) (bool, error) {
	opts, err := s.store.GetTaskCIOptions(ctx, taskID)
	if err != nil {
		return false, err
	}
	subscribed := (state == prStateMerged && opts.PromptOnMerged) ||
		(state == prStateClosed && opts.PromptOnClosed)
	if !subscribed {
		return false, nil
	}
	checkpoint, err := s.store.GetTaskCIPRState(ctx, taskID, repositoryID, prNumber)
	if err != nil {
		return false, err
	}
	return checkpoint == nil || checkpoint.LastObservedPRState != state, nil
}

func (s *Service) buildTaskCIOptionsResponse(ctx context.Context, opts *TaskCIOptions) (*TaskCIOptionsResponse, error) {
	prStates, err := s.taskCIPRStates(ctx, opts.TaskID)
	if err != nil {
		return nil, err
	}
	effectivePrompt, usingDefault := s.effectiveCIAutoFixPrompt(ctx, opts)
	reviewPrompt := effectiveTaskPRPrompt("pr-review-requested")
	mergedPrompt := effectiveTaskPRPrompt("pr-merged-final")
	closedPrompt := effectiveTaskPRPrompt("pr-closed-final")
	return &TaskCIOptionsResponse{
		TaskID:                  opts.TaskID,
		AutoFixEnabled:          opts.AutoFixEnabled,
		AutoMergeEnabled:        opts.AutoMergeEnabled,
		AutoFixPromptOverride:   opts.AutoFixPromptOverride,
		AutoFixMaxRounds:        TaskCIAutoFixMaxRounds,
		EffectiveAutoFixPrompt:  effectivePrompt,
		UsingDefaultPrompt:      usingDefault,
		PromptOnReviewRequested: opts.PromptOnReviewRequested,
		PromptOnMerged:          opts.PromptOnMerged,
		PromptOnClosed:          opts.PromptOnClosed,
		ReviewReviewerLogin:     opts.ReviewReviewerLogin,
		EffectiveReviewPrompt:   reviewPrompt,
		EffectiveMergedPrompt:   mergedPrompt,
		EffectiveClosedPrompt:   closedPrompt,
		UpdatedAt:               opts.UpdatedAt,
		PRStates:                prStates,
	}, nil
}

func effectiveTaskPRPrompt(name string) string {
	return promptcfg.Get(name)
}

func (s *Service) effectiveCIAutoFixPrompt(ctx context.Context, opts *TaskCIOptions) (string, bool) {
	if opts.AutoFixPromptOverride != nil {
		if override := strings.TrimSpace(*opts.AutoFixPromptOverride); override != "" {
			return override, false
		}
	}
	fallback := promptcfg.Get(defaultCIAutoFixPromptName)
	resolver := s.getPromptResolver()
	if resolver == nil {
		return fallback, true
	}
	return resolver.ResolvePromptContent(ctx, defaultCIAutoFixPromptName, fallback), true
}

func (s *Service) taskCIPRStates(ctx context.Context, taskID string) ([]*TaskCIPRAutomationState, error) {
	stored, err := s.store.ListTaskCIPRStates(ctx, taskID)
	if err != nil {
		return nil, err
	}
	byKey := make(map[string]*TaskCIPRAutomationState, len(stored))
	for _, state := range stored {
		byKey[taskCIPRStateKey(state.RepositoryID, state.PRNumber)] = state
	}
	prs, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	out := make([]*TaskCIPRAutomationState, 0, max(len(prs), len(stored)))
	seen := make(map[string]struct{}, len(prs))
	for _, pr := range prs {
		key := taskCIPRStateKey(pr.RepositoryID, pr.PRNumber)
		if state, ok := byKey[key]; ok {
			out = append(out, state)
		} else {
			out = append(out, &TaskCIPRAutomationState{
				TaskID:       taskID,
				RepositoryID: pr.RepositoryID,
				PRNumber:     pr.PRNumber,
			})
		}
		seen[key] = struct{}{}
	}
	for _, state := range stored {
		key := taskCIPRStateKey(state.RepositoryID, state.PRNumber)
		if _, ok := seen[key]; !ok {
			out = append(out, state)
		}
	}
	return out, nil
}

func taskCIPRStateKey(repositoryID string, prNumber int) string {
	return fmt.Sprintf("%s#%d", repositoryID, prNumber)
}
