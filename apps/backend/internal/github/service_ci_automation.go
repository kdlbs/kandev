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
		return nil, fmt.Errorf("github store not configured")
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
		return nil, fmt.Errorf("github store not configured")
	}
	opts, err := s.store.UpdateTaskCIOptions(ctx, taskID, patch)
	if err != nil {
		return nil, err
	}
	return s.buildTaskCIOptionsResponse(ctx, opts)
}

// GetTaskCIPRState returns per-PR CI automation state, or nil.
func (s *Service) GetTaskCIPRState(ctx context.Context, taskID, repositoryID string, prNumber int) (*TaskCIPRAutomationState, error) {
	if s.store == nil {
		return nil, fmt.Errorf("github store not configured")
	}
	return s.store.GetTaskCIPRState(ctx, taskID, repositoryID, prNumber)
}

// RecordTaskCIFixAttempt records an auto-fix attempt.
func (s *Service) RecordTaskCIFixAttempt(ctx context.Context, attempt TaskCIFixAttempt) error {
	if s.store == nil {
		return fmt.Errorf("github store not configured")
	}
	return s.store.RecordTaskCIFixAttempt(ctx, attempt)
}

// RecordTaskCIMergeAttempt records an auto-merge attempt.
func (s *Service) RecordTaskCIMergeAttempt(ctx context.Context, attempt TaskCIMergeAttempt) error {
	if s.store == nil {
		return fmt.Errorf("github store not configured")
	}
	return s.store.RecordTaskCIMergeAttempt(ctx, attempt)
}

// RecordTaskCIError records a CI automation error.
func (s *Service) RecordTaskCIError(ctx context.Context, taskID, repositoryID string, prNumber int, message string) error {
	if s.store == nil {
		return fmt.Errorf("github store not configured")
	}
	return s.store.RecordTaskCIError(ctx, taskID, repositoryID, prNumber, message)
}

// ClearTaskCIError clears a CI automation error.
func (s *Service) ClearTaskCIError(ctx context.Context, taskID, repositoryID string, prNumber int) error {
	if s.store == nil {
		return fmt.Errorf("github store not configured")
	}
	return s.store.ClearTaskCIError(ctx, taskID, repositoryID, prNumber)
}

func (s *Service) buildTaskCIOptionsResponse(ctx context.Context, opts *TaskCIOptions) (*TaskCIOptionsResponse, error) {
	prStates, err := s.taskCIPRStates(ctx, opts.TaskID)
	if err != nil {
		return nil, err
	}
	effectivePrompt, usingDefault := s.effectiveCIAutoFixPrompt(ctx, opts)
	return &TaskCIOptionsResponse{
		TaskID:                 opts.TaskID,
		AutoFixEnabled:         opts.AutoFixEnabled,
		AutoMergeEnabled:       opts.AutoMergeEnabled,
		AutoFixPromptOverride:  opts.AutoFixPromptOverride,
		EffectiveAutoFixPrompt: effectivePrompt,
		UsingDefaultPrompt:     usingDefault,
		UpdatedAt:              opts.UpdatedAt,
		PRStates:               prStates,
	}, nil
}

func (s *Service) effectiveCIAutoFixPrompt(ctx context.Context, opts *TaskCIOptions) (string, bool) {
	if opts.AutoFixPromptOverride != nil {
		if override := strings.TrimSpace(*opts.AutoFixPromptOverride); override != "" {
			return override, false
		}
	}
	fallback := promptcfg.Get(defaultCIAutoFixPromptName)
	if s.promptResolver == nil {
		return fallback, true
	}
	return s.promptResolver.ResolvePromptContent(ctx, defaultCIAutoFixPromptName, fallback), true
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
