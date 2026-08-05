package gitlab

import (
	"context"
	"errors"
	"strings"

	promptcfg "github.com/kandev/kandev/config/prompts"
)

// defaultMRAutoFixPromptName is the embedded default prompt template name
// (apps/backend/config/prompts/mr-auto-fix.md), mirroring GitHub's
// defaultCIAutoFixPromptName.
const defaultMRAutoFixPromptName = "mr-auto-fix"

// GetTaskMRAutomationResponse returns a task's MR automation options plus
// its per-MR lifecycle checkpoints (AC1).
func (s *Service) GetTaskMRAutomationResponse(ctx context.Context, taskID string) (*TaskMRAutomationResponse, error) {
	if err := s.authorizeTaskMRAccess(ctx, taskID); err != nil {
		return nil, err
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	// WorkspaceIDForTask also validates the task row exists — unlike
	// authorizeTaskMRAccess (a no-op for unscoped/auth-disabled callers),
	// this rejects an unknown task ID unconditionally instead of returning
	// an implicit all-false default for it.
	workspaceID, err := store.WorkspaceIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	opts, err := store.GetTaskMRAutomationOptions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	states, err := store.ListTaskMRLifecycleStates(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.taskMRAutomationResponseFromOptions(ctx, opts, states, workspaceID), nil
}

// GetTaskMRAutomationEvaluation returns the narrow snapshot needed for one
// lifecycle evaluation. It intentionally avoids the task-wide checkpoint scan
// used by the public response and takes reviewer identity from the persisted
// workspace configuration, whose save and health flows own identity refresh.
func (s *Service) GetTaskMRAutomationEvaluation(
	ctx context.Context, taskID, repositoryID, projectPath string, mrIID int,
) (*TaskMRAutomationEvaluation, error) {
	if err := s.authorizeTaskMRAccess(ctx, taskID); err != nil {
		return nil, err
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	workspaceID, err := store.WorkspaceIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	cfg, err := store.GetConfigForWorkspace(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	reviewerUsername := ""
	if cfg != nil {
		reviewerUsername = strings.TrimSpace(cfg.Username)
	}
	// Rebind before reading the target checkpoint. The store updates the
	// persisted reviewer and clears review-request baselines in one transaction,
	// so an account change cannot be evaluated against the previous account's
	// edge state.
	if _, err := s.rebindTaskMRReviewerFromConfig(ctx, taskID, reviewerUsername); err != nil {
		return nil, err
	}
	opts, err := store.GetTaskMRAutomationOptions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	checkpoint, err := store.GetTaskMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID)
	if err != nil {
		return nil, err
	}
	// Evaluation uses the workspace config as the authoritative current
	// identity; a missing config/username therefore fails closed without
	// probing GitLab for every MR.
	evaluationOpts := *opts
	evaluationOpts.ReviewReviewerUsername = reviewerUsername
	return &TaskMRAutomationEvaluation{
		Options:    s.taskMRAutomationResponseFromOptions(ctx, &evaluationOpts, nil, workspaceID),
		Checkpoint: checkpoint,
	}, nil
}

func (s *Service) rebindTaskMRReviewerFromConfig(ctx context.Context, taskID, username string) (bool, error) {
	store := s.requireStore()
	if store == nil {
		return false, errStoreUnavailable
	}
	return store.RebindTaskMRReviewer(ctx, taskID, username)
}

func (s *Service) taskMRAutomationResponseFromOptions(
	ctx context.Context, opts *TaskMRAutomationOptions, states []*TaskMRLifecycleState, workspaceID string,
) *TaskMRAutomationResponse {
	effectivePrompt, usingDefault := s.effectiveMRAutoFixPrompt(ctx, opts)
	return &TaskMRAutomationResponse{
		TaskID:                  opts.TaskID,
		AutoFixEnabled:          opts.AutoFixEnabled,
		AutoMergeEnabled:        opts.AutoMergeEnabled,
		AutoFixPromptOverride:   opts.AutoFixPromptOverride,
		AutoFixMaxRounds:        TaskMRAutoFixMaxRounds,
		EffectiveAutoFixPrompt:  effectivePrompt,
		UsingDefaultPrompt:      usingDefault,
		PromptOnReviewRequested: opts.PromptOnReviewRequested,
		PromptOnMerged:          opts.PromptOnMerged,
		PromptOnClosed:          opts.PromptOnClosed,
		ReviewReviewerUsername:  opts.ReviewReviewerUsername,
		UpdatedAt:               opts.UpdatedAt,
		MRStates:                states,
		WorkspaceID:             workspaceID,
	}
}

// effectiveMRAutoFixPrompt resolves the prompt text that will actually be
// sent on the next auto-fix dispatch: a non-empty per-task override wins;
// otherwise the default template, itself resolved through the editable
// prompt service when wired (so a workspace-level edit to the "mr-auto-fix"
// prompt applies) or the embedded fallback when not. Mirrors GitHub's
// effectiveCIAutoFixPrompt.
func (s *Service) effectiveMRAutoFixPrompt(ctx context.Context, opts *TaskMRAutomationOptions) (string, bool) {
	if opts.AutoFixPromptOverride != nil {
		if override := strings.TrimSpace(*opts.AutoFixPromptOverride); override != "" {
			return override, false
		}
	}
	fallback := promptcfg.Get(defaultMRAutoFixPromptName)
	resolver := s.getPromptResolver()
	if resolver == nil {
		return fallback, true
	}
	return resolver.ResolvePromptContent(ctx, defaultMRAutoFixPromptName, fallback), true
}

// UpdateTaskMRAutomationOptions applies a partial update. When the patch
// turns prompt_on_review_requested on, the workspace's authenticated GitLab
// username is resolved and persisted (AC5); turning it off clears the
// stored username. Resolution always goes through the strict, non-ambient
// workspace client (AC32).
func (s *Service) UpdateTaskMRAutomationOptions(ctx context.Context, taskID string, patch TaskMRAutomationPatch) (*TaskMRAutomationResponse, error) {
	if err := s.authorizeTaskMRAccess(ctx, taskID); err != nil {
		return nil, err
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	// See the identical check in GetTaskMRAutomationResponse: rejects an
	// unknown task ID before it can create an orphan options row.
	workspaceID, err := store.WorkspaceIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	reviewerUsername, err := s.resolveReviewerUsernameForPatch(ctx, taskID, patch)
	if err != nil {
		return nil, err
	}
	opts, err := store.UpdateTaskMRAutomationOptions(ctx, taskID, patch, reviewerUsername)
	if err != nil {
		return nil, err
	}
	states, err := store.ListTaskMRLifecycleStates(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.taskMRAutomationResponseFromOptions(ctx, opts, states, workspaceID), nil
}

func (s *Service) resolveReviewerUsernameForPatch(ctx context.Context, taskID string, patch TaskMRAutomationPatch) (*string, error) {
	if patch.PromptOnReviewRequested == nil {
		return nil, nil
	}
	if !*patch.PromptOnReviewRequested {
		empty := ""
		return &empty, nil
	}
	username, err := s.resolveAuthenticatedUsernameStrict(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &username, nil
}

func (s *Service) resolveAuthenticatedUsernameStrict(ctx context.Context, taskID string) (string, error) {
	client, err := s.clientForTaskStrict(ctx, taskID)
	if err != nil {
		return "", err
	}
	username, err := client.GetAuthenticatedUser(ctx)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(username) == "" {
		return "", errors.New("gitlab: cannot resolve authenticated user")
	}
	return username, nil
}

// HasEnabledTaskMRAgentPrompts reports whether any lifecycle switch is on,
// used by cleanup retention (AC24) and by the poll/evaluation gate (AC16).
func (s *Service) HasEnabledTaskMRAgentPrompts(ctx context.Context, taskID string) (bool, error) {
	store := s.requireStore()
	if store == nil {
		return false, errStoreUnavailable
	}
	opts, err := store.GetTaskMRAutomationOptions(ctx, taskID)
	if err != nil {
		return false, err
	}
	return opts.PromptOnReviewRequested || opts.PromptOnMerged || opts.PromptOnClosed, nil
}

// RebindTaskMRReviewer re-resolves the workspace's authenticated GitLab
// username against the strict client and, when it changed, atomically
// clears every review-request baseline for the task (risk 3 in the plan).
// Returns the current username.
func (s *Service) RebindTaskMRReviewer(ctx context.Context, taskID string) (string, bool, error) {
	store := s.requireStore()
	if store == nil {
		return "", false, errStoreUnavailable
	}
	username, err := s.resolveAuthenticatedUsernameStrict(ctx, taskID)
	if err != nil {
		return "", false, err
	}
	changed, err := store.RebindTaskMRReviewer(ctx, taskID, username)
	if err != nil {
		return "", false, err
	}
	return username, changed, nil
}

// GetTaskMRLifecycleState exposes the per-MR checkpoint to the orchestrator
// lifecycle evaluator.
func (s *Service) GetTaskMRLifecycleState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) (*TaskMRLifecycleState, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.GetTaskMRLifecycleState(ctx, taskID, repositoryID, projectPath, mrIID)
}

// SetTaskMRReviewRequestState pass-through to the store.
func (s *Service) SetTaskMRReviewRequestState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, requested bool) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.SetTaskMRReviewRequestState(ctx, taskID, repositoryID, projectPath, mrIID, requested)
}

// SetTaskMRObservedState pass-through to the store.
func (s *Service) SetTaskMRObservedState(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, state string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.SetTaskMRObservedState(ctx, taskID, repositoryID, projectPath, mrIID, state)
}

// RecordTaskMRLifecyclePrompt pass-through to the store.
func (s *Service) RecordTaskMRLifecyclePrompt(ctx context.Context, prompt TaskMRLifecyclePrompt) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.RecordTaskMRLifecyclePrompt(ctx, prompt)
}

// RecordTaskMRAutomationError pass-through to the store (AC25).
func (s *Service) RecordTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.RecordTaskMRAutomationError(ctx, taskID, repositoryID, projectPath, mrIID, message)
}

// ClearTaskMRAutomationError pass-through to the store. Called after a
// successful lifecycle prompt delivery so a recovered delivery failure
// doesn't linger in last_error and read as an active problem.
func (s *Service) ClearTaskMRAutomationError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.ClearTaskMRAutomationError(ctx, taskID, repositoryID, projectPath, mrIID)
}

// RecordTaskMRSyncError pass-through to the store. Called by the poller when
// a lifecycle sync pass fails — kept in a separate column from
// RecordTaskMRAutomationError's delivery error so the two concerns can't
// clobber each other.
func (s *Service) RecordTaskMRSyncError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.RecordTaskMRSyncError(ctx, taskID, repositoryID, projectPath, mrIID, message)
}

// RecordTaskMRFixAttempt pass-through to the store.
func (s *Service) RecordTaskMRFixAttempt(ctx context.Context, attempt TaskMRFixAttempt) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.RecordTaskMRFixAttempt(ctx, attempt)
}

// RefreshTaskMRFixCheckpoint pass-through to the store.
func (s *Service) RefreshTaskMRFixCheckpoint(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, signature, checkpointJSON string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.RefreshTaskMRFixCheckpoint(ctx, taskID, repositoryID, projectPath, mrIID, signature, checkpointJSON)
}

// MarkTaskMRAutoFixExhausted pass-through to the store.
func (s *Service) MarkTaskMRAutoFixExhausted(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int, message string) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.MarkTaskMRAutoFixExhausted(ctx, taskID, repositoryID, projectPath, mrIID, message)
}

// RecordTaskMRMergeAttempt pass-through to the store.
func (s *Service) RecordTaskMRMergeAttempt(ctx context.Context, attempt TaskMRMergeAttempt) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.RecordTaskMRMergeAttempt(ctx, attempt)
}

// ClearTaskMRSyncError pass-through to the store. Called after a successful
// lifecycle sync so a recovered sync failure doesn't linger in
// last_sync_error and read as an active problem.
func (s *Service) ClearTaskMRSyncError(ctx context.Context, taskID, repositoryID, projectPath string, mrIID int) error {
	store := s.requireStore()
	if store == nil {
		return errStoreUnavailable
	}
	return store.ClearTaskMRSyncError(ctx, taskID, repositoryID, projectPath, mrIID)
}

// IsReviewerOnMR reports whether username currently appears in the MR's
// Reviewers list — GitLab's assignment-as-request signal, since GitLab has
// no distinct "review requested" API event (unlike GitHub's requested
// reviewers). Always resolves the client via the strict, workspace-scoped
// helper (AC32); never falls back to the ambient client.
func (s *Service) IsReviewerOnMR(ctx context.Context, taskID, projectPath string, mrIID int, username string) (bool, error) {
	client, err := s.clientForTaskStrict(ctx, taskID)
	if err != nil {
		return false, err
	}
	mr, err := client.GetMR(ctx, projectPath, mrIID)
	if err != nil {
		return false, err
	}
	if mr == nil {
		return false, nil
	}
	for _, reviewer := range mr.Reviewers {
		if strings.EqualFold(reviewer.Username, username) {
			return true, nil
		}
	}
	return false, nil
}

// ListAutomationSubscribedTaskMRs pass-through to the store, used by the
// poller's sync pass (AC22) for any MR whose task has a lifecycle,
// auto-fix, or auto-merge switch on.
func (s *Service) ListAutomationSubscribedTaskMRs(ctx context.Context) ([]*TaskMR, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	return store.ListAutomationSubscribedTaskMRs(ctx)
}
