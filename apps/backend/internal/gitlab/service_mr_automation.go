package gitlab

import (
	"context"
	"errors"
	"fmt"
	"strings"

	promptcfg "github.com/kandev/kandev/config/prompts"
)

// defaultMRAutoFixPromptName is the embedded default prompt template name
// (apps/backend/config/prompts/mr-auto-fix.md), mirroring GitHub's
// defaultCIAutoFixPromptName.
const defaultMRAutoFixPromptName = "mr-auto-fix"

// ErrTaskMRNotLinked reports a patch naming an MR that is not linked to the
// task. Exported so the HTTP controller and MCP handler can map it to a
// client error instead of a 500.
var ErrTaskMRNotLinked = errors.New("gitlab: merge request is not linked to this task")

// ErrTaskMRIdentityIncomplete reports a patch that set some but not all of
// repository_id/project_path/mr_iid. Sending a partial identity is a caller
// mistake, and treating it as "no identity" would silently fan the switch
// change out to every linked MR.
var ErrTaskMRIdentityIncomplete = fmt.Errorf(
	"%w: repository_id, project_path and mr_iid must all be set", ErrTaskMRNotLinked,
)

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
	mrOptions, err := s.taskMRAutomationOptionsList(ctx, taskID)
	if err != nil {
		return nil, err
	}
	states, err := store.ListTaskMRLifecycleStates(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.taskMRAutomationResponseFromOptions(ctx, opts, mrOptions, states, workspaceID), nil
}

// taskMRAutomationOptionsList returns one entry per MR currently linked to
// the task, synthesizing all-off defaults for a linked MR that has no stored
// row yet. A stored row whose MR is no longer linked is dropped: it can no
// longer be configured or evaluated, and surfacing it would let a detached
// MR's leftover switch drag the aggregate down.
func (s *Service) taskMRAutomationOptionsList(ctx context.Context, taskID string) ([]*TaskMRAutomationOptionsForMR, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	stored, err := store.ListTaskMRAutomationOptions(ctx, taskID)
	if err != nil {
		return nil, err
	}
	mrs, err := store.ListTaskMRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	byIdentity := make(map[MRIdentity]*TaskMRAutomationOptionsForMR, len(stored))
	for _, row := range stored {
		byIdentity[row.Identity()] = row
	}
	out := make([]*TaskMRAutomationOptionsForMR, 0, len(mrs))
	for _, mr := range mrs {
		id := MRIdentity{RepositoryID: mr.RepositoryID, ProjectPath: mr.ProjectPath, MRIID: mr.MRIID}
		if row, ok := byIdentity[id]; ok {
			out = append(out, row)
			continue
		}
		out = append(out, &TaskMRAutomationOptionsForMR{
			TaskID: taskID, RepositoryID: id.RepositoryID,
			ProjectPath: id.ProjectPath, MRIID: id.MRIID,
		})
	}
	return out, nil
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
	id := MRIdentity{RepositoryID: repositoryID, ProjectPath: projectPath, MRIID: mrIID}
	// Only this MR's own switches drive this evaluation — a sibling MR's
	// configuration must never enable automation here.
	mrOpts, err := store.GetTaskMRAutomationOptionsForMR(ctx, taskID, id)
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
		Options: s.taskMRAutomationResponseFromOptions(
			ctx, &evaluationOpts, []*TaskMRAutomationOptionsForMR{mrOpts}, nil, workspaceID,
		),
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
	ctx context.Context, opts *TaskMRAutomationOptions, mrOptions []*TaskMRAutomationOptionsForMR,
	states []*TaskMRLifecycleState, workspaceID string,
) *TaskMRAutomationResponse {
	effectivePrompt, usingDefault := s.effectiveMRAutoFixPrompt(ctx, opts)
	aggregate := aggregateMRAutomationOptions(mrOptions)
	return &TaskMRAutomationResponse{
		TaskID:                  opts.TaskID,
		AutomationRevision:      opts.AutomationRevision,
		AutoFixEnabled:          aggregate.AutoFixEnabled,
		AutoMergeEnabled:        aggregate.AutoMergeEnabled,
		AutoFixPromptOverride:   opts.AutoFixPromptOverride,
		AutoFixMaxRounds:        TaskMRAutoFixMaxRounds,
		EffectiveAutoFixPrompt:  effectivePrompt,
		UsingDefaultPrompt:      usingDefault,
		PromptOnReviewRequested: aggregate.PromptOnReviewRequested,
		PromptOnMerged:          aggregate.PromptOnMerged,
		PromptOnClosed:          aggregate.PromptOnClosed,
		ReviewReviewerUsername:  opts.ReviewReviewerUsername,
		UpdatedAt:               opts.UpdatedAt,
		MRStates:                states,
		MROptions:               mrOptions,
		WorkspaceID:             workspaceID,
	}
}

// aggregateMRAutomationOptions collapses per-MR switches into the response's
// top-level booleans: a switch reads as on only when every linked MR has it
// on and at least one MR is linked. "Every" rather than "any" keeps the
// aggregate a safe answer to "is this on for the MRs I'd act on" for an MCP
// caller that cannot name one MR.
func aggregateMRAutomationOptions(mrOptions []*TaskMRAutomationOptionsForMR) TaskMRAutomationOptionsForMR {
	if len(mrOptions) == 0 {
		return TaskMRAutomationOptionsForMR{}
	}
	aggregate := TaskMRAutomationOptionsForMR{
		AutoFixEnabled: true, AutoMergeEnabled: true, PromptOnReviewRequested: true,
		PromptOnMerged: true, PromptOnClosed: true,
	}
	for _, opt := range mrOptions {
		aggregate.AutoFixEnabled = aggregate.AutoFixEnabled && opt.AutoFixEnabled
		aggregate.AutoMergeEnabled = aggregate.AutoMergeEnabled && opt.AutoMergeEnabled
		aggregate.PromptOnReviewRequested = aggregate.PromptOnReviewRequested && opt.PromptOnReviewRequested
		aggregate.PromptOnMerged = aggregate.PromptOnMerged && opt.PromptOnMerged
		aggregate.PromptOnClosed = aggregate.PromptOnClosed && opt.PromptOnClosed
	}
	return aggregate
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

// UpdateTaskMRAutomationOptions applies a partial update. The five automation
// switches are applied to the single MR named by
// patch.RepositoryID/ProjectPath/MRIID, or — when no MR is named — fanned out
// to every MR currently linked to the task, which preserves the behavior of
// MCP callers that have no MR identity to send. The auto-fix prompt override
// stays task-level either way.
//
// When the patch turns prompt_on_review_requested on, the workspace's
// authenticated GitLab username is resolved and persisted (AC5); turning it
// off clears the stored username unless another linked MR still needs it.
// Resolution always goes through the strict, non-ambient workspace client
// (AC32).
func (s *Service) UpdateTaskMRAutomationOptions(ctx context.Context, taskID string, patch TaskMRAutomationPatch) (*TaskMRAutomationResponse, error) {
	if err := s.authorizeTaskMRAccess(ctx, taskID); err != nil {
		return nil, err
	}
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	if patch.HasPartialMRIdentity() {
		return nil, ErrTaskMRIdentityIncomplete
	}
	// See the identical check in GetTaskMRAutomationResponse: rejects an
	// unknown task ID before it can create an orphan options row.
	workspaceID, err := store.WorkspaceIDForTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	targets, err := s.resolveTaskMRAutomationTargets(ctx, taskID, patch)
	if err != nil {
		return nil, err
	}
	// The switches are per-MR, so with nothing linked there is no row to write
	// them to. Returning 200 here would report success for a write that stored
	// nothing, and the caller would only discover it much later when the
	// automation it thought it had enabled never fired. (Before the switches
	// became per-MR this persisted on the task row and a later-linked MR
	// inherited it.) The prompt override is task-level and still allowed.
	if patch.SwitchPatch().HasAny() && len(targets) == 0 {
		return nil, fmt.Errorf("%w: the task has no linked merge requests", ErrTaskMRNotLinked)
	}
	reviewerUsername, err := s.resolveReviewerUsernameForPatch(ctx, taskID, patch, targets)
	if err != nil {
		return nil, err
	}
	// One transaction covering the task-level fields and every target: a
	// failure must not leave the reviewer or prompt override committed while
	// the per-MR switches roll back.
	opts, err := store.UpdateTaskMRAutomationOptionsWithMRs(
		ctx, taskID, patch, reviewerUsername, targets, patch.SwitchPatch(),
	)
	if err != nil {
		return nil, err
	}
	mrOptions, err := s.taskMRAutomationOptionsList(ctx, taskID)
	if err != nil {
		return nil, err
	}
	states, err := store.ListTaskMRLifecycleStates(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.taskMRAutomationResponseFromOptions(ctx, opts, mrOptions, states, workspaceID), nil
}

// resolveTaskMRAutomationTargets resolves which linked MR(s) a patch applies
// to. No identity fans out to every linked MR; a named identity that is not
// currently linked is rejected rather than silently creating an orphan
// automation row.
func (s *Service) resolveTaskMRAutomationTargets(
	ctx context.Context, taskID string, patch TaskMRAutomationPatch,
) ([]MRIdentity, error) {
	store := s.requireStore()
	if store == nil {
		return nil, errStoreUnavailable
	}
	mrs, err := store.ListTaskMRsByTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	linked := make([]MRIdentity, 0, len(mrs))
	for _, mr := range mrs {
		linked = append(linked, MRIdentity{
			RepositoryID: mr.RepositoryID, ProjectPath: mr.ProjectPath, MRIID: mr.MRIID,
		})
	}
	if !patch.HasMRIdentity() {
		return linked, nil
	}
	wanted := patch.MRIdentity()
	for _, id := range linked {
		if id == wanted {
			return []MRIdentity{id}, nil
		}
	}
	return nil, fmt.Errorf("%w: project_path=%s mr_iid=%d", ErrTaskMRNotLinked, wanted.ProjectPath, wanted.MRIID)
}

// resolveReviewerUsernameForPatch resolves the task-level reviewer username
// implied by a review-request switch change. Disabling only blanks the shared
// username when no *other* linked MR still has the switch on — otherwise this
// would silently break that MR's automation.
func (s *Service) resolveReviewerUsernameForPatch(
	ctx context.Context, taskID string, patch TaskMRAutomationPatch, targets []MRIdentity,
) (*string, error) {
	if patch.PromptOnReviewRequested == nil {
		return nil, nil
	}
	if *patch.PromptOnReviewRequested {
		username, err := s.resolveAuthenticatedUsernameStrict(ctx, taskID)
		if err != nil {
			return nil, err
		}
		return &username, nil
	}
	stillNeeded, err := s.anyOtherMRHasReviewRequestEnabled(ctx, taskID, targets)
	if err != nil {
		return nil, err
	}
	if stillNeeded {
		return nil, nil
	}
	empty := ""
	return &empty, nil
}

// anyOtherMRHasReviewRequestEnabled reports whether a linked MR outside
// `targets` still has prompt_on_review_requested on.
func (s *Service) anyOtherMRHasReviewRequestEnabled(
	ctx context.Context, taskID string, targets []MRIdentity,
) (bool, error) {
	store := s.requireStore()
	if store == nil {
		return false, errStoreUnavailable
	}
	stored, err := store.ListTaskMRAutomationOptions(ctx, taskID)
	if err != nil {
		return false, err
	}
	targeted := make(map[MRIdentity]struct{}, len(targets))
	for _, id := range targets {
		targeted[id] = struct{}{}
	}
	for _, opt := range stored {
		if !opt.PromptOnReviewRequested {
			continue
		}
		if _, ok := targeted[opt.Identity()]; ok {
			continue
		}
		return true, nil
	}
	return false, nil
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

// HasEnabledTaskMRAgentPrompts reports whether any MR linked to the task has
// a lifecycle switch on, used by cleanup retention (AC24) and by the
// poll/evaluation gate (AC16). Reads the per-MR table, so a task whose MRs
// are configured differently is still detected.
func (s *Service) HasEnabledTaskMRAgentPrompts(ctx context.Context, taskID string) (bool, error) {
	store := s.requireStore()
	if store == nil {
		return false, errStoreUnavailable
	}
	stored, err := store.ListTaskMRAutomationOptions(ctx, taskID)
	if err != nil {
		return false, err
	}
	for _, opt := range stored {
		if opt.PromptOnReviewRequested || opt.PromptOnMerged || opt.PromptOnClosed {
			return true, nil
		}
	}
	return false, nil
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
