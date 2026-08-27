package github

import (
	"context"
	"fmt"
	"strings"

	promptcfg "github.com/kandev/kandev/config/prompts"
)

// errTaskPRIdentityIncomplete signals a patch that set exactly one of
// repository_id/pr_number instead of both or neither.
var errTaskPRIdentityIncomplete = fmt.Errorf("%w: repository_id and pr_number must both be set", ErrTaskPRNotLinked)

// GetTaskCIOptionsResponse returns task CI automation options plus effective prompt text.
func (s *Service) GetTaskCIOptionsResponse(ctx context.Context, taskID string) (*TaskCIOptionsResponse, error) {
	if s.store == nil {
		return nil, errStoreUnavailable
	}
	workspaceID, err := s.resolveAuthorizedTaskWorkspace(ctx, taskID)
	if err != nil {
		return nil, err
	}
	opts, err := s.store.GetTaskCIOptions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	response, err := s.buildTaskCIOptionsResponse(ctx, opts)
	if response != nil {
		response.WorkspaceID = workspaceID
	}
	return response, err
}

// UpdateTaskCIOptions updates task CI automation options and returns the
// response shape. patch.RepositoryID/PRNumber optionally target one linked
// PR for the five automation switches; omitted identity fans the switches
// out to every PR currently linked to the task (unchanged behavior for
// existing MCP callers). AutoFixPromptOverride and ReviewReviewerLogin are
// always applied task-level regardless of PR identity.
func (s *Service) UpdateTaskCIOptions(ctx context.Context, taskID string, patch TaskCIOptionsPatch) (*TaskCIOptionsResponse, error) {
	if s.store == nil {
		return nil, errStoreUnavailable
	}
	workspaceID, err := s.resolveAuthorizedTaskWorkspace(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if (patch.RepositoryID == nil) != (patch.PRNumber == nil) {
		return nil, errTaskPRIdentityIncomplete
	}
	prPatch := patch.PRAutomationPatch()
	var targets []*TaskPR
	if patch.RepositoryID != nil || prPatch.HasAny() {
		targets, err = s.resolveTaskPRAutomationTargets(ctx, taskID, patch.RepositoryID, patch.PRNumber)
		if err != nil {
			return nil, err
		}
	}
	if err := s.populateReviewReviewer(ctx, taskID, &patch); err != nil {
		return nil, err
	}
	reviewerChanged, err := s.taskPRReviewerChanged(ctx, taskID, patch.ReviewReviewerLogin)
	if err != nil {
		return nil, err
	}
	var opts *TaskCIOptions
	if prPatch.HasAny() {
		opts, err = s.store.UpdateTaskCIOptionsWithPRAutomation(
			ctx, taskID, patch, targets, prPatch, reviewerChanged,
		)
	} else {
		opts, err = s.store.UpdateTaskCIOptions(ctx, taskID, patch)
	}
	if err != nil {
		return nil, err
	}
	response, err := s.buildTaskCIOptionsResponse(ctx, opts)
	if response != nil {
		response.WorkspaceID = workspaceID
	}
	return response, err
}

// resolveAuthorizedTaskWorkspace resolves ownership through the task service
// before any user-facing CI-option read or mutation. Unit tests and legacy
// auth-disabled embeddings without a task store retain their unscoped path;
// an installed authorizer without the resolver fails closed.
func (s *Service) resolveAuthorizedTaskWorkspace(ctx context.Context, taskID string) (string, error) {
	taskStore := s.getTaskIssueStore()
	if taskStore == nil {
		if s.workspaceAuthorizer != nil {
			return "", errStoreUnavailable
		}
		return "", nil
	}
	task, err := taskStore.GetTask(ctx, taskID)
	if err != nil {
		return "", err
	}
	if task == nil {
		return "", ErrTaskNotFound
	}
	workspaceID := strings.TrimSpace(task.WorkspaceID)
	if workspaceID == "" {
		return "", ErrGitHubWorkspaceRequired
	}
	if err := s.authorizeWorkspaceAccess(ctx, workspaceID); err != nil {
		return "", err
	}
	return workspaceID, nil
}

// taskPRReviewerChanged compares the newly resolved task-level reviewer
// login (if the patch is setting one) against the value already stored, so
// callers can re-baseline review-request checkpoints precisely when the
// connected account actually changed.
func (s *Service) taskPRReviewerChanged(ctx context.Context, taskID string, newLogin *string) (bool, error) {
	if newLogin == nil {
		return false, nil
	}
	previous, err := s.store.GetTaskCIOptions(ctx, taskID)
	if err != nil {
		return false, err
	}
	return !strings.EqualFold(previous.ReviewReviewerLogin, strings.TrimSpace(*newLogin)), nil
}

// resolveTaskPRAutomationTargets resolves which linked PR(s) a patch
// applies to. Nil identity fans out to every linked PR (AC7); a named
// identity that isn't currently linked is rejected with ErrTaskPRNotLinked
// (AC8) rather than silently creating an orphan automation row.
func (s *Service) resolveTaskPRAutomationTargets(
	ctx context.Context, taskID string, repositoryID *string, prNumber *int,
) ([]*TaskPR, error) {
	prs, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if repositoryID == nil && prNumber == nil {
		return prs, nil
	}
	for _, pr := range prs {
		if pr.RepositoryID == *repositoryID && pr.PRNumber == *prNumber {
			return []*TaskPR{pr}, nil
		}
	}
	return nil, fmt.Errorf("%w: repository_id=%s pr_number=%d", ErrTaskPRNotLinked, *repositoryID, *prNumber)
}

func (s *Service) populateReviewReviewer(
	ctx context.Context,
	taskID string,
	patch *TaskCIOptionsPatch,
) error {
	if patch.PromptOnReviewRequested == nil {
		return nil
	}
	if *patch.PromptOnReviewRequested {
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
	// Disabling review-request on the target PR(s) only blanks the shared
	// task-level reviewer login when no other linked PR still depends on
	// it — otherwise this would silently break that PR's automation.
	stillNeeded, err := s.anyOtherPRHasReviewRequestEnabled(ctx, taskID, patch.RepositoryID, patch.PRNumber)
	if err != nil {
		return err
	}
	if stillNeeded {
		return nil
	}
	empty := ""
	patch.ReviewReviewerLogin = &empty
	return nil
}

func (s *Service) anyOtherPRHasReviewRequestEnabled(
	ctx context.Context, taskID string, excludeRepositoryID *string, excludePRNumber *int,
) (bool, error) {
	fanOut := excludeRepositoryID == nil && excludePRNumber == nil
	options, err := s.taskPRAutomationOptionsList(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, opt := range options {
		if !opt.PromptOnReviewRequested {
			continue
		}
		if fanOut {
			// The fan-out disable targets every linked PR, so nothing
			// survives with the switch on regardless of stored state.
			continue
		}
		if opt.RepositoryID == *excludeRepositoryID && opt.PRNumber == *excludePRNumber {
			continue
		}
		return true, nil
	}
	return false, nil
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

// RecordTaskCIMergeQueueObservation persists active queue membership and the
// latest removal cause so automation can recover safely after a restart.
func (s *Service) RecordTaskCIMergeQueueObservation(ctx context.Context, observation TaskCIMergeQueueObservation) error {
	if s.store == nil {
		return errStoreUnavailable
	}
	return s.store.RecordTaskCIMergeQueueObservation(ctx, observation)
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

// HasEnabledTaskPRAgentPrompts reports whether any PR linked to the task has
// a lifecycle prompt switch enabled. Checks the per-PR table, not the legacy
// task row, so a task with mixed per-PR configuration is still detected.
func (s *Service) HasEnabledTaskPRAgentPrompts(ctx context.Context, taskID string) (bool, error) {
	options, err := s.taskPRAutomationOptionsList(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, opt := range options {
		if opt.PromptOnReviewRequested || opt.PromptOnMerged || opt.PromptOnClosed {
			return true, nil
		}
	}
	return false, nil
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
	opts, err := s.store.GetTaskPRAutomationOptions(ctx, taskID, repositoryID, prNumber)
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
	prOptions, err := s.taskPRAutomationOptionsList(ctx, opts.TaskID)
	if err != nil {
		return nil, err
	}
	effectivePrompt, usingDefault := s.effectiveCIAutoFixPrompt(ctx, opts)
	reviewPrompt := effectiveTaskPRPrompt("pr-review-requested")
	mergedPrompt := effectiveTaskPRPrompt("pr-merged-final")
	closedPrompt := effectiveTaskPRPrompt("pr-closed-final")
	aggregate := aggregateTaskPRAutomationOptions(prOptions)
	return &TaskCIOptionsResponse{
		TaskID:                  opts.TaskID,
		AutoFixEnabled:          aggregate.autoFixEnabled,
		AutoMergeEnabled:        aggregate.autoMergeEnabled,
		AutoFixPromptOverride:   opts.AutoFixPromptOverride,
		AutoFixMaxRounds:        TaskCIAutoFixMaxRounds,
		EffectiveAutoFixPrompt:  effectivePrompt,
		UsingDefaultPrompt:      usingDefault,
		PromptOnReviewRequested: aggregate.promptOnReviewRequested,
		PromptOnMerged:          aggregate.promptOnMerged,
		PromptOnClosed:          aggregate.promptOnClosed,
		ReviewReviewerLogin:     opts.ReviewReviewerLogin,
		EffectiveReviewPrompt:   reviewPrompt,
		EffectiveMergedPrompt:   mergedPrompt,
		EffectiveClosedPrompt:   closedPrompt,
		UpdatedAt:               opts.UpdatedAt,
		PRStates:                prStates,
		PROptions:               prOptions,
	}, nil
}

// taskPRAutomationOptionsList returns one entry per PR currently linked to
// the task (AC5), synthesizing all-off defaults for a linked PR that has no
// stored row yet — mirroring the placeholder pattern in taskCIPRStates.
func (s *Service) taskPRAutomationOptionsList(ctx context.Context, taskID string) ([]*TaskPRAutomationOptions, error) {
	stored, err := s.store.ListTaskPRAutomationOptions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	prs, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	allPRs, err := s.store.ListTaskPRsByTaskIncludingDetached(ctx, taskID)
	if err != nil {
		return nil, err
	}
	merged := mergeTaskPRRows(stored, prs,
		func(opt *TaskPRAutomationOptions) string { return taskCIPRStateKey(opt.RepositoryID, opt.PRNumber) },
		func(pr *TaskPR) *TaskPRAutomationOptions {
			return &TaskPRAutomationOptions{TaskID: taskID, RepositoryID: pr.RepositoryID, PRNumber: pr.PRNumber}
		},
	)
	active := make(map[string]struct{}, len(prs))
	for _, pr := range prs {
		active[taskCIPRStateKey(pr.RepositoryID, pr.PRNumber)] = struct{}{}
	}
	detached := make(map[string]struct{})
	for _, pr := range allPRs {
		if pr.DetachedAt != nil {
			detached[taskCIPRStateKey(pr.RepositoryID, pr.PRNumber)] = struct{}{}
		}
	}
	filtered := make([]*TaskPRAutomationOptions, 0, len(prs))
	for _, option := range merged {
		key := taskCIPRStateKey(option.RepositoryID, option.PRNumber)
		if _, ok := detached[key]; ok {
			continue
		}
		if _, ok := active[key]; ok {
			filtered = append(filtered, option)
			continue
		}
		// Keep legacy/single-repo option rows that predate a durable TaskPR
		// association. Only a matching detached association is excluded.
		if _, known := detached[key]; !known {
			filtered = append(filtered, option)
		}
	}
	return filtered, nil
}

// mergeTaskPRRows overlays stored per-PR rows onto every PR currently linked
// to the task, synthesizing a default row (via makeDefault) for a linked PR
// with no stored row yet, and keeping any stored row for a PR no longer
// linked (e.g. detached) at the end. Task CI state keeps those historical rows
// for diagnostics, while task automation options filters them before exposing
// aggregates or lifecycle eligibility. Shared by taskCIPRStates and
// taskPRAutomationOptionsList, which differ only in row type.
func mergeTaskPRRows[T any](stored []T, prs []*TaskPR, keyOf func(T) string, makeDefault func(*TaskPR) T) []T {
	byKey := make(map[string]T, len(stored))
	for _, row := range stored {
		byKey[keyOf(row)] = row
	}
	out := make([]T, 0, max(len(prs), len(stored)))
	seen := make(map[string]struct{}, len(prs))
	for _, pr := range prs {
		key := taskCIPRStateKey(pr.RepositoryID, pr.PRNumber)
		if row, ok := byKey[key]; ok {
			out = append(out, row)
		} else {
			out = append(out, makeDefault(pr))
		}
		seen[key] = struct{}{}
	}
	for _, row := range stored {
		if _, ok := seen[keyOf(row)]; !ok {
			out = append(out, row)
		}
	}
	return out
}

type taskPRAutomationAggregate struct {
	autoFixEnabled, autoMergeEnabled                        bool
	promptOnReviewRequested, promptOnMerged, promptOnClosed bool
}

// aggregateTaskPRAutomationOptions reports a switch as "enabled" only when
// every linked PR has it on and at least one PR is linked — the top-level
// booleans answer "did my task-wide enable take", kept for MCP/API read
// compatibility (R3). Prefer PROptions for anything PR-specific.
func aggregateTaskPRAutomationOptions(options []*TaskPRAutomationOptions) taskPRAutomationAggregate {
	if len(options) == 0 {
		return taskPRAutomationAggregate{}
	}
	agg := taskPRAutomationAggregate{
		autoFixEnabled: true, autoMergeEnabled: true,
		promptOnReviewRequested: true, promptOnMerged: true, promptOnClosed: true,
	}
	for _, opt := range options {
		agg.autoFixEnabled = agg.autoFixEnabled && opt.AutoFixEnabled
		agg.autoMergeEnabled = agg.autoMergeEnabled && opt.AutoMergeEnabled
		agg.promptOnReviewRequested = agg.promptOnReviewRequested && opt.PromptOnReviewRequested
		agg.promptOnMerged = agg.promptOnMerged && opt.PromptOnMerged
		agg.promptOnClosed = agg.promptOnClosed && opt.PromptOnClosed
	}
	return agg
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
	prs, err := s.store.ListTaskPRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return mergeTaskPRRows(stored, prs,
		func(state *TaskCIPRAutomationState) string {
			return taskCIPRStateKey(state.RepositoryID, state.PRNumber)
		},
		func(pr *TaskPR) *TaskCIPRAutomationState {
			return &TaskCIPRAutomationState{TaskID: taskID, RepositoryID: pr.RepositoryID, PRNumber: pr.PRNumber}
		},
	), nil
}

func taskCIPRStateKey(repositoryID string, prNumber int) string {
	return fmt.Sprintf("%s#%d", repositoryID, prNumber)
}
