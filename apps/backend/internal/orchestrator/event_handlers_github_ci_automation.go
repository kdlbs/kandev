package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/sysprompt"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
)

const (
	ciAutomationOrigin           = "ci_automation"
	ciAutomationCheckSuccess     = "success"
	ciAutomationCheckFailure     = "failure"
	ciAutomationCheckError       = "error"
	ciAutomationCheckCompleted   = "completed"
	ciAutomationCheckPending     = "pending"
	ciAutomationChangesRequested = "changes_requested"
	ciAutomationPRFeedbackToken  = "{{pr.feedback}}"
	ciAutomationFixBlockWindow   = time.Hour
	ciAutomationMaxFixRounds     = github.TaskCIAutoFixMaxRounds
	ciAutomationKindAutoFix      = "ci_auto_fix"
	ciAutomationStateEventSource = "ci_automation_state"
)

var ciAutomationSnapshotFieldReplacer = strings.NewReplacer("\r", " ", "\n", " ", "<", "", ">", "")

type ciAutomationCheckpoint struct {
	FailedChecks  []ciAutomationCheckSnapshot            `json:"failed_checks"`
	Comments      []ciAutomationCommentSnapshot          `json:"comments"`
	QueueRemovals []ciAutomationQueueRemovalSnapshotData `json:"queue_removals,omitempty"`
}

type ciAutomationCheckSnapshot struct {
	Name       string `json:"name"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
	Output     string `json:"output,omitempty"`
}

type ciAutomationCommentSnapshot struct {
	ID   int64  `json:"id"`
	Body string `json:"body,omitempty"`
	Path string `json:"path,omitempty"`
	Line int    `json:"line,omitempty"`
}

const (
	ciAutomationQueueRemovalCauseChecksFailed     = "checks_failed"
	ciAutomationQueueRemovalCauseChecksTimedOut   = "checks_timed_out"
	ciAutomationQueueRemovalCauseConflict         = "conflict"
	ciAutomationQueueRemovalCauseManual           = "manual"
	ciAutomationQueueRemovalCauseBranchProtection = "branch_protection"
	ciAutomationQueueRemovalCauseUnknown          = "unknown"
)

type ciAutomationQueueRemovalSnapshotData struct {
	EventID      string    `json:"event_id"`
	Cause        string    `json:"cause"`
	Reason       string    `json:"reason,omitempty"`
	RemovedAt    time.Time `json:"removed_at,omitempty"`
	BeforeCommit string    `json:"before_commit,omitempty"`
	CheckCommit  string    `json:"check_commit,omitempty"`
	Conflict     bool      `json:"conflict,omitempty"`
}

func (s *Service) handleTaskPRCIAutomation(ctx context.Context, pr *github.TaskPR) error {
	return s.handleTaskPRCIAutomationWithRefresh(ctx, pr, true)
}

func (s *Service) handleTaskPRCIAutomationWithRefresh(ctx context.Context, pr *github.TaskPR, refresh bool) error {
	if s.githubService == nil || pr == nil {
		return nil
	}
	task, err := s.repo.GetTask(ctx, pr.TaskID)
	if err != nil {
		if errors.Is(err, taskrepo.ErrTaskNotFound) {
			return nil
		}
		return err
	}
	if task == nil || task.ArchivedAt != nil {
		return nil
	}
	options, err := s.githubService.GetTaskCIOptionsResponse(ctx, pr.TaskID)
	if err != nil {
		s.logger.Debug("load CI automation options failed", zap.String("task_id", pr.TaskID), zap.Error(err))
		return nil
	}
	options = resolvePRScopedCIOptions(options, pr)
	freshlySynced := ciAutomationHasFreshPRStatus(pr)
	if refresh && (options.AutoFixEnabled || options.AutoMergeEnabled) {
		refreshed, synced, syncErr := s.refreshTaskPRForCIAutomation(ctx, pr)
		if syncErr != nil {
			s.handleTaskPRLifecycleAutomation(ctx, pr, options)
			s.recordCIAutomationError(ctx, pr, fmt.Sprintf("sync PR status: %v", syncErr))
			s.publishTaskCIOptionsState(ctx, pr.TaskID)
			return nil
		}
		pr = refreshed
		freshlySynced = synced
	}
	if options.AutoFixEnabled || options.AutoMergeEnabled {
		s.recordTaskPRMergeQueueObservation(ctx, pr)
	}
	autoFixBlockedMerge := false
	autoFixError := ""
	if options.AutoFixEnabled && ciAutomationCanAutoFixFromFeedback(pr) {
		autoFixBlockedMerge, autoFixError = s.handleTaskPRCIAutoFix(ctx, pr, options)
	}
	ciError := ciAutomationMergeOutcome{}
	if !autoFixBlockedMerge && options.AutoMergeEnabled && ciAutomationReadyToMerge(pr) {
		if !freshlySynced {
			ciError = ciAutomationMergeOutcome{message: "PR status is not freshly synced for auto-merge"}
		} else {
			ciError = s.handleTaskPRCIAutoMerge(ctx, pr)
		}
	}
	s.handleTaskPRLifecycleAutomation(ctx, pr, options)
	if autoFixError != "" {
		s.recordCIAutomationError(ctx, pr, autoFixError)
		s.publishTaskCIOptionsState(ctx, pr.TaskID)
	}
	if ciError.message != "" && !ciError.journaled {
		s.recordCIAutoMergeError(ctx, pr, ciError.message)
		s.publishTaskCIOptionsState(ctx, pr.TaskID)
	}
	return nil
}

// resolvePRScopedCIOptions overlays this PR's own automation switches onto a
// copy of the task-scoped response, so the auto-fix, auto-merge, and
// lifecycle-prompt automation below acts on this PR's per-PR configuration
// instead of the task-wide aggregate. Task-level fields (reviewer login,
// effective prompts, prompt override) are left as returned.
func resolvePRScopedCIOptions(options *github.TaskCIOptionsResponse, pr *github.TaskPR) *github.TaskCIOptionsResponse {
	if options == nil || pr == nil {
		return options
	}
	scoped := *options
	for _, opt := range options.PROptions {
		if opt == nil || opt.RepositoryID != pr.RepositoryID || opt.PRNumber != pr.PRNumber {
			continue
		}
		scoped.AutoFixEnabled = opt.AutoFixEnabled
		scoped.AutoMergeEnabled = opt.AutoMergeEnabled
		scoped.PromptOnReviewRequested = opt.PromptOnReviewRequested
		scoped.PromptOnMerged = opt.PromptOnMerged
		scoped.PromptOnClosed = opt.PromptOnClosed
		break
	}
	return &scoped
}

func (s *Service) handleTaskPRLifecycleAutomation(ctx context.Context, pr *github.TaskPR, options *github.TaskCIOptionsResponse) {
	if !options.PromptOnReviewRequested && !options.PromptOnMerged && !options.PromptOnClosed {
		return
	}
	automation, ok := s.githubService.(taskPRAgentAutomationService)
	if !ok {
		return
	}
	delivered, err := s.evalTaskPRLifecycle(ctx, pr, options, automation)
	if err != nil {
		s.logger.Debug("task PR lifecycle automation failed",
			zap.String("task_id", pr.TaskID),
			zap.String("repository_id", pr.RepositoryID),
			zap.Int("pr_number", pr.PRNumber),
			zap.Error(err))
		s.recordCIAutomationError(ctx, pr, fmt.Sprintf("lifecycle automation: %v", err))
		s.publishTaskCIOptionsState(ctx, pr.TaskID)
		return
	}
	if delivered {
		s.publishTaskCIOptionsState(ctx, pr.TaskID)
	}
}

func (s *Service) evalTaskPRLifecycle(
	ctx context.Context,
	pr *github.TaskPR,
	options *github.TaskCIOptionsResponse,
	automation taskPRAgentAutomationService,
) (bool, error) {
	terminal := pr.State == taskPRAgentEventMerged || pr.State == taskPRAgentEventClosed
	if !terminal && options.PromptOnReviewRequested {
		login, _, err := automation.RebindTaskPRReviewer(ctx, pr.TaskID)
		if err != nil {
			return false, err
		}
		options.ReviewReviewerLogin = login
	}
	checkpoint, err := automation.GetTaskCIPRState(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber)
	if err != nil {
		return false, err
	}
	reviewRequested, err := currentTaskPRReviewRequest(ctx, automation, pr, options)
	if err != nil {
		return false, err
	}
	decision := decideTaskPRAgentPrompt(pr.State, options, checkpoint, reviewRequested)
	if decision.Event == "" {
		return false, stampTaskPRAgentObservations(ctx, automation, pr, decision)
	}
	prompt, err := taskPRAgentLifecyclePrompt(decision.Event, pr)
	if err != nil {
		return false, fmt.Errorf("build %s prompt: %w", decision.Event, err)
	}
	sessionID, err := s.dispatchTaskPRAgentPrompt(ctx, pr, prompt, decision.Event)
	if errors.Is(err, errTaskPRAgentInactive) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("dispatch %s prompt: %w", decision.Event, err)
	}
	err = automation.RecordTaskPRLifecyclePrompt(ctx, github.TaskPRLifecyclePrompt{
		TaskID:          pr.TaskID,
		RepositoryID:    pr.RepositoryID,
		PRNumber:        pr.PRNumber,
		Event:           decision.Event,
		SessionID:       sessionID,
		PromptedAt:      time.Now().UTC(),
		ReviewRequested: decision.ReviewRequested != nil && *decision.ReviewRequested,
		ObservedState:   decision.ObservedState,
	})
	return err == nil, err
}

func (s *Service) refreshTaskPRForCIAutomation(ctx context.Context, pr *github.TaskPR) (*github.TaskPR, bool, error) {
	if pr == nil {
		return nil, false, nil
	}
	prs, err := s.githubService.TriggerPRSyncAll(ctx, pr.TaskID)
	if refreshed := ciAutomationFindMatchingPR(prs, pr); refreshed != nil {
		return refreshed, ciAutomationHasFreshPRStatus(refreshed), nil
	}
	if err != nil {
		return nil, false, err
	}
	return pr, false, nil
}

func ciAutomationFindMatchingPR(prs []*github.TaskPR, target *github.TaskPR) *github.TaskPR {
	if target == nil {
		return nil
	}
	for _, pr := range prs {
		if pr == nil || pr.TaskID != target.TaskID || pr.PRNumber != target.PRNumber {
			continue
		}
		if target.RepositoryID != "" && pr.RepositoryID == target.RepositoryID {
			return pr
		}
		if target.RepositoryID == "" && pr.Owner == target.Owner && pr.Repo == target.Repo {
			return pr
		}
	}
	return nil
}

func ciAutomationHasFreshPRStatus(pr *github.TaskPR) bool {
	return ciAutomationHasFreshPRStatusAt(pr, time.Now())
}

func ciAutomationHasFreshPRStatusAt(pr *github.TaskPR, now time.Time) bool {
	if pr == nil || pr.LastSyncedAt == nil {
		return false
	}
	return now.Sub(*pr.LastSyncedAt) <= github.PRSyncFreshnessWindow
}

func (s *Service) handleTaskPRCIAutoFix(ctx context.Context, pr *github.TaskPR, options *github.TaskCIOptionsResponse) (bool, string) {
	state, err := s.githubService.GetTaskCIPRState(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber)
	if err != nil {
		return true, fmt.Sprintf("load CI automation state: %v", err)
	}
	if state != nil && state.AutoFixExhaustedAt != nil {
		return !ciAutomationReadyToMerge(pr), ""
	}
	feedback, err := s.githubService.GetPRFeedbackForAutomation(
		ctx, pr.WorkspaceID, pr.Owner, pr.Repo, pr.PRNumber,
	)
	if err != nil {
		return true, fmt.Sprintf("fetch PR feedback: %v", err)
	}
	if !ciAutomationCanPromptForFeedback(pr, feedback) {
		return false, ""
	}
	feedback = ciAutomationFilterFeedbackForPR(pr, feedback)
	previous := decodeCIAutomationCheckpoint(state)
	delta := ciAutomationBuildDelta(feedback, previous)
	checkpoint := ciAutomationCurrentCheckpoint(feedback)
	queueRemoval, queueRecovery := ciAutomationNewQueueRemoval(pr, state)
	if queueRecovery {
		delta.QueueRemovals = append(delta.QueueRemovals, *queueRemoval)
		checkpoint.QueueRemovals = append(checkpoint.QueueRemovals, *queueRemoval)
	}
	checkpointJSON, signature := encodeCIAutomationCheckpoint(checkpoint)
	if ciAutomationCheckpointEmpty(delta) {
		return s.handleTaskPRCIAutoFixEmptyDelta(ctx, pr, state, previous, signature, checkpointJSON), ""
	}
	if state != nil && state.LastFixSignature == signature {
		return ciAutomationDuplicateFixAttemptBlocksMerge(state), ""
	}
	allowNewRound := !ciAutomationFixRoundsExhausted(state)
	prompt := ciAutomationRenderPrompt(options.EffectiveAutoFixPrompt, pr, delta)
	session, err := s.resolveCIAutoFixSession(ctx, pr.TaskID, state)
	if err != nil || session == nil {
		return s.handleCIAutoFixWithoutSession(ctx, pr, allowNewRound)
	}
	// Passthrough CI-fix sessions skip "@name" expansion: the prompt is typed
	// straight into the agent CLI's TTY with no <kandev-system> stripping, so a
	// hidden expansion block would leak into the terminal verbatim.
	prompt = s.expandPromptReferences(ctx, prompt, session.IsPassthrough)
	result, err := s.dispatchCIAutomationPromptForPR(ctx, session, pr, prompt, signature, allowNewRound)
	if errors.Is(err, errCIAutoFixRoundCapReached) {
		s.markCIAutoFixExhausted(ctx, pr)
		return true, ""
	}
	if err != nil {
		return true, err.Error()
	}
	queueRemovalEventID, queueRemovalCause := "", ""
	if queueRecovery {
		queueRemovalEventID = queueRemoval.EventID
		queueRemovalCause = queueRemoval.Cause
	}
	if err := s.githubService.RecordTaskCIFixAttempt(context.WithoutCancel(ctx), github.TaskCIFixAttempt{
		TaskID:              pr.TaskID,
		RepositoryID:        pr.RepositoryID,
		PRNumber:            pr.PRNumber,
		Signature:           signature,
		CheckpointJSON:      checkpointJSON,
		SessionID:           session.ID,
		EnqueuedAt:          time.Now().UTC(),
		IncrementRound:      queueRecovery || result.consumesRound(),
		QueueRemovalEventID: queueRemovalEventID,
		QueueRemovalCause:   queueRemovalCause,
	}); err != nil {
		s.logger.Debug("record CI auto-fix attempt failed", zap.String("task_id", pr.TaskID), zap.Error(err))
	} else {
		s.publishTaskCIOptionsState(ctx, pr.TaskID)
	}
	return true, ""
}

func (s *Service) handleCIAutoFixWithoutSession(ctx context.Context, pr *github.TaskPR, allowNewRound bool) (bool, string) {
	if !allowNewRound {
		s.markCIAutoFixExhausted(ctx, pr)
		return true, ""
	}
	return true, "no promptable task session for CI auto-fix"
}

func ciAutomationCanPromptForFeedback(pr *github.TaskPR, feedback *github.PRFeedback) bool {
	return ciAutomationCanAutoFixFromFeedbackPR(feedback) && ciAutomationChecksSettledForAutoFix(pr, feedback)
}

// resolveCIAutoFixSession adapts the provider-agnostic resolveAutoFixSession
// (ci_automation_dispatch.go, C5) to GitHub's checkpoint state shape.
func (s *Service) resolveCIAutoFixSession(ctx context.Context, taskID string, state *github.TaskCIPRAutomationState) (*models.TaskSession, error) {
	var lastFixSessionID *string
	if state != nil {
		lastFixSessionID = state.LastFixSessionID
	}
	return s.resolveAutoFixSession(ctx, taskID, lastFixSessionID)
}

func (s *Service) handleTaskPRCIAutoFixEmptyDelta(ctx context.Context, pr *github.TaskPR, state *github.TaskCIPRAutomationState, previous ciAutomationCheckpoint, signature, checkpointJSON string) bool {
	if state != nil && state.LastFixSignature == signature && ciAutomationDuplicateFixAttemptBlocksMerge(state) {
		return true
	}
	if state != nil && len(previous.FailedChecks)+len(previous.Comments) > 0 {
		if err := s.githubService.RefreshTaskCIFixCheckpoint(context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber, signature, checkpointJSON); err != nil {
			s.logger.Debug("record CI auto-fix checkpoint refresh failed", zap.String("task_id", pr.TaskID), zap.Error(err))
		}
	}
	return false
}

type ciAutomationMergeOutcome struct {
	message   string
	journaled bool
}

func (s *Service) handleTaskPRCIAutoMerge(ctx context.Context, pr *github.TaskPR) ciAutomationMergeOutcome {
	if ciAutomationHasActiveMergeQueueEntry(pr) {
		return ciAutomationMergeOutcome{}
	}
	signature := ciAutomationMergeSignature(pr)
	state, err := s.githubService.GetTaskCIPRState(ctx, pr.TaskID, pr.RepositoryID, pr.PRNumber)
	if err != nil {
		s.logger.Debug("load CI automation merge state failed; durable reservation will decide", zap.String("task_id", pr.TaskID), zap.Error(err))
	} else if state != nil {
		if outcome, handled := s.ciAutomationExistingMergeOutcome(ctx, pr, signature, state); handled {
			return outcome
		}
	}
	attempt := github.TaskCIMergeAttempt{
		TaskID:           pr.TaskID,
		RepositoryID:     pr.RepositoryID,
		PRNumber:         pr.PRNumber,
		Signature:        signature,
		AttemptedAt:      time.Now().UTC(),
		AttemptedHeadSHA: pr.HeadSHA,
	}
	if err := s.githubService.RecordTaskCIMergeAttempt(context.WithoutCancel(ctx), attempt); err != nil {
		if errors.Is(err, github.ErrTaskCIMergeAttemptAlreadyReserved) {
			return ciAutomationMergeOutcome{}
		}
		return ciAutomationMergeOutcome{message: fmt.Sprintf("reserve merge PR attempt: %v", err)}
	}
	return s.executeCIAutoMerge(ctx, pr, signature)
}

func (s *Service) ciAutomationExistingMergeOutcome(
	ctx context.Context,
	pr *github.TaskPR,
	signature string,
	state *github.TaskCIPRAutomationState,
) (ciAutomationMergeOutcome, bool) {
	if state.LastMergeSignature != signature {
		return ciAutomationQueueAttemptOutcome(pr, state)
	}
	if state.LastMergeResult == github.TaskCIMergeResultInFlight &&
		state.LastMergeAttemptAt != nil && time.Since(*state.LastMergeAttemptAt) > ciAutomationDetachedTimeout {
		message := "merge PR: previous automatic merge attempt did not reach a terminal state"
		if recordErr := s.githubService.RecordTaskCIMergeAttemptResult(
			context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber,
			signature, github.TaskCIMergeResultFailed, message,
		); recordErr != nil {
			return ciAutomationMergeOutcome{message: fmt.Sprintf("%s; record expired merge attempt: %v", message, recordErr)}, true
		}
		if !state.MergeRetryPending {
			return ciAutomationMergeOutcome{message: message, journaled: true}, true
		}
	}
	if !state.MergeRetryPending {
		return ciAutomationMergeOutcome{}, true
	}
	return ciAutomationQueueAttemptOutcome(pr, state)
}

func ciAutomationQueueAttemptOutcome(
	pr *github.TaskPR,
	state *github.TaskCIPRAutomationState,
) (ciAutomationMergeOutcome, bool) {
	if pr.HeadSHA != "" && state.LastQueueAttemptHeadSHA == pr.HeadSHA && !state.MergeRetryPending {
		return ciAutomationMergeOutcome{}, true
	}
	return ciAutomationMergeOutcome{}, false
}

func (s *Service) executeCIAutoMerge(
	ctx context.Context,
	pr *github.TaskPR,
	signature string,
) ciAutomationMergeOutcome {
	if err := s.githubService.MergePRForAutomation(
		ctx, pr.WorkspaceID, pr.Owner, pr.Repo, pr.PRNumber, "", pr.HeadSHA,
	); err != nil {
		message := fmt.Sprintf("merge PR: %v", err)
		if recordErr := s.githubService.RecordTaskCIMergeAttemptResult(
			context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber,
			signature, github.TaskCIMergeResultFailed, message,
		); recordErr != nil {
			return ciAutomationMergeOutcome{message: fmt.Sprintf("%s; record failed merge attempt: %v", message, recordErr)}
		}
		return ciAutomationMergeOutcome{message: message, journaled: true}
	}
	if err := s.githubService.RecordTaskCIMergeAttemptResult(
		context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber,
		signature, github.TaskCIMergeResultAccepted, "",
	); err != nil {
		return ciAutomationMergeOutcome{message: fmt.Sprintf("record accepted merge PR attempt: %v", err)}
	}
	return ciAutomationMergeOutcome{}
}

func (s *Service) recordTaskPRMergeQueueObservation(ctx context.Context, pr *github.TaskPR) {
	if pr == nil || (pr.MergeQueueLastRemovalID == "" && !ciAutomationHasActiveMergeQueueEntry(pr) && pr.State != githubPRStateMerged) {
		return
	}
	removal, _ := ciAutomationQueueRemovalSnapshot(pr)
	observation := github.TaskCIMergeQueueObservation{
		TaskID:                 pr.TaskID,
		RepositoryID:           pr.RepositoryID,
		PRNumber:               pr.PRNumber,
		RemovalEventID:         pr.MergeQueueLastRemovalID,
		RemovalObservedHeadSHA: pr.HeadSHA,
		Accepted:               pr.State == githubPRStateMerged,
	}
	if ciAutomationHasActiveMergeQueueEntry(pr) {
		observation.ActiveQueueHeadSHA = pr.MergeQueueEntryHeadSHA
		if observation.ActiveQueueHeadSHA == "" {
			observation.ActiveQueueHeadSHA = pr.HeadSHA
		}
		observation.MergeSignature = ciAutomationMergeSignature(pr)
	}
	if removal != nil {
		observation.RemovalCause = removal.Cause
	}
	if err := s.githubService.RecordTaskCIMergeQueueObservation(context.WithoutCancel(ctx), observation); err != nil {
		s.logger.Debug("record merge queue observation failed",
			zap.String("task_id", pr.TaskID), zap.Int("pr_number", pr.PRNumber), zap.Error(err))
	}
}

// dispatchCIAutomationPromptForPR adapts the provider-agnostic
// dispatchCIAutomationPrompt (ci_automation_dispatch.go, C5) to GitHub PR
// vocabulary: the prompt gets the "@ci-auto-fix" mention prefix, the
// coalesce key and message metadata are derived from the PR.
func (s *Service) dispatchCIAutomationPromptForPR(
	ctx context.Context, session *models.TaskSession, pr *github.TaskPR, prompt, signature string, allowNewRound bool,
) (ciAutomationDispatchResult, error) {
	return s.dispatchCIAutomationPrompt(ctx, session, ciAutomationDispatchParams{
		ChatPrompt:    ciAutomationChatPrompt(prompt),
		CoalesceKey:   ciAutomationCoalesceKey(pr),
		Metadata:      ciAutomationMessageMetadataForPR(pr, signature),
		AllowNewRound: allowNewRound,
	})
}

func ciAutomationMessageMetadata() map[string]interface{} {
	meta := NewUserMessageMeta().WithAutoStart(true).ToMap()
	if meta == nil {
		meta = map[string]interface{}{}
	}
	meta["origin"] = ciAutomationOrigin
	return meta
}

func ciAutomationMessageMetadataForPR(pr *github.TaskPR, signature string) map[string]interface{} {
	meta := ciAutomationMessageMetadata()
	meta["automation_kind"] = ciAutomationKindAutoFix
	meta["ci_auto_fix_key"] = ciAutomationCoalesceKey(pr)
	meta["feedback_signature"] = signature
	if pr != nil {
		meta["repository_id"] = pr.RepositoryID
		meta["owner"] = pr.Owner
		meta["repo"] = pr.Repo
		meta["pr_number"] = pr.PRNumber
	}
	return meta
}

func ciAutomationCoalesceKey(pr *github.TaskPR) string {
	if pr == nil {
		return "ci-auto-fix||0"
	}
	return fmt.Sprintf("ci-auto-fix|%s|%s|%d", pr.TaskID, pr.RepositoryID, pr.PRNumber)
}

func ciAutomationChatPrompt(prompt string) string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return "@ci-auto-fix"
	}
	return "@ci-auto-fix\n\n" + prompt
}

func (s *Service) recordCIAutomationError(ctx context.Context, pr *github.TaskPR, message string) {
	s.logger.Warn("CI automation error",
		zap.String("task_id", pr.TaskID),
		zap.String("repository_id", pr.RepositoryID),
		zap.String("owner", pr.Owner),
		zap.String("repo", pr.Repo),
		zap.Int("pr_number", pr.PRNumber),
		zap.String("error", message))
	if err := s.githubService.RecordTaskCIError(context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber, message); err != nil {
		s.logger.Debug("record CI automation error failed", zap.String("task_id", pr.TaskID), zap.Error(err))
	}
}

func (s *Service) recordCIAutoMergeError(ctx context.Context, pr *github.TaskPR, message string) {
	s.logger.Warn("CI auto-merge error",
		zap.String("task_id", pr.TaskID),
		zap.String("repository_id", pr.RepositoryID),
		zap.Int("pr_number", pr.PRNumber),
		zap.String("error", message))
	if err := s.githubService.RecordTaskCIAutoMergeError(
		context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber, message,
	); err != nil {
		s.logger.Debug("record CI auto-merge error failed", zap.String("task_id", pr.TaskID), zap.Error(err))
	}
}

func ciAutomationCanAutoFixFromFeedback(pr *github.TaskPR) bool {
	return pr != nil && pr.State != taskPRAgentEventClosed && pr.State != taskPRAgentEventMerged
}

func ciAutomationCanAutoFixFromFeedbackPR(feedback *github.PRFeedback) bool {
	if feedback == nil || feedback.PR == nil {
		return true
	}
	return feedback.PR.State != taskPRAgentEventClosed && feedback.PR.State != taskPRAgentEventMerged
}

func ciAutomationChecksSettledForAutoFix(pr *github.TaskPR, feedback *github.PRFeedback) bool {
	if pr != nil && !ciAutomationChecksRollupSettled(pr.ChecksState) {
		return false
	}
	if feedback == nil {
		return true
	}
	for _, check := range feedback.Checks {
		if check.Status != ciAutomationCheckCompleted {
			return false
		}
	}
	return true
}

func ciAutomationChecksRollupSettled(state string) bool {
	// GitHub GraphQL rollups can expose values like EXPECTED before concrete
	// check runs exist. Keep the whitelist narrow so unknown rollup states wait.
	switch strings.TrimSpace(strings.ToLower(state)) {
	case "", ciAutomationCheckSuccess, ciAutomationCheckFailure, ciAutomationCheckError:
		return true
	default:
		return false
	}
}

func ciAutomationFilterFeedbackForPR(pr *github.TaskPR, feedback *github.PRFeedback) *github.PRFeedback {
	if feedback == nil || pr == nil {
		return feedback
	}
	filtered := *feedback
	filtered.Comments = make([]github.PRComment, 0, len(feedback.Comments))
	includeBotPRComments := ciAutomationFeedbackHasActionableSignal(pr, feedback)
	for _, comment := range feedback.Comments {
		if ciAutomationShouldIncludeFeedbackComment(pr, comment, includeBotPRComments) {
			filtered.Comments = append(filtered.Comments, comment)
		}
	}
	return &filtered
}

func ciAutomationFeedbackHasActionableSignal(pr *github.TaskPR, feedback *github.PRFeedback) bool {
	if pr.UnresolvedReviewThreads > 0 {
		return true
	}
	for _, check := range feedback.Checks {
		if check.Status == ciAutomationCheckCompleted && ciAutomationCheckConclusionNeedsFix(check.Conclusion) {
			return true
		}
	}
	return false
}

func ciAutomationShouldIncludeFeedbackComment(pr *github.TaskPR, comment github.PRComment, includeBotPRComments bool) bool {
	if comment.Path == "" && comment.Line == 0 {
		return !comment.AuthorIsBot || includeBotPRComments
	}
	return pr.UnresolvedReviewThreads > 0
}

func ciAutomationReadyToMerge(pr *github.TaskPR) bool {
	if pr == nil || pr.State != githubPRStateOpen {
		return false
	}
	if pr.ChecksState != ciAutomationCheckSuccess || pr.MergeableState != "clean" {
		return false
	}
	if pr.ReviewState == ciAutomationChangesRequested || pr.PendingReviewCount > 0 || pr.UnresolvedReviewThreads > 0 {
		return false
	}
	if pr.RequiredReviews != nil && pr.ReviewCount < *pr.RequiredReviews {
		return false
	}
	return true
}

func ciAutomationBuildDelta(feedback *github.PRFeedback, previous ciAutomationCheckpoint) ciAutomationCheckpoint {
	prevChecks := make(map[string]struct{}, len(previous.FailedChecks))
	for _, check := range previous.FailedChecks {
		prevChecks[ciAutomationCheckKey(check)] = struct{}{}
	}
	prevComments := make(map[int64]ciAutomationCommentSnapshot, len(previous.Comments))
	for _, comment := range previous.Comments {
		prevComments[comment.ID] = comment
	}
	var delta ciAutomationCheckpoint
	if feedback == nil {
		return delta
	}
	for _, check := range feedback.Checks {
		if check.Status != ciAutomationCheckCompleted || !ciAutomationCheckConclusionNeedsFix(check.Conclusion) {
			continue
		}
		snap := ciAutomationCheckSnapshot{Name: check.Name, Conclusion: check.Conclusion, HTMLURL: check.HTMLURL, Output: check.Output}
		if _, seen := prevChecks[ciAutomationCheckKey(snap)]; !seen {
			delta.FailedChecks = append(delta.FailedChecks, snap)
		}
	}
	for _, comment := range feedback.Comments {
		snap := ciAutomationCommentSnapshot{
			ID: comment.ID, Body: comment.Body, Path: comment.Path, Line: comment.Line,
		}
		if previous, seen := prevComments[comment.ID]; seen && previous == snap {
			continue
		}
		delta.Comments = append(delta.Comments, snap)
	}
	return delta
}

func ciAutomationCheckConclusionNeedsFix(conclusion string) bool {
	return conclusion == ciAutomationCheckFailure ||
		conclusion == "timed_out" ||
		conclusion == "cancelled" ||
		conclusion == "action_required"
}

func ciAutomationDuplicateFixAttemptBlocksMerge(state *github.TaskCIPRAutomationState) bool {
	return ciAutomationDuplicateFixAttemptBlocksMergeAt(state, time.Now())
}

func ciAutomationFixRoundsExhausted(state *github.TaskCIPRAutomationState) bool {
	if state == nil {
		return false
	}
	return state.AutoFixExhaustedAt != nil || state.AutoFixRoundCount >= ciAutomationMaxFixRounds
}

func (s *Service) markCIAutoFixExhausted(ctx context.Context, pr *github.TaskPR) {
	if pr == nil {
		return
	}
	message := fmt.Sprintf("CI auto-fix paused after %d rounds for this PR", ciAutomationMaxFixRounds)
	s.logger.Warn("CI automation auto-fix round cap reached",
		zap.String("task_id", pr.TaskID),
		zap.String("repository_id", pr.RepositoryID),
		zap.Int("pr_number", pr.PRNumber),
		zap.Int("max_rounds", ciAutomationMaxFixRounds))
	if err := s.githubService.MarkTaskCIAutoFixExhausted(context.WithoutCancel(ctx), pr.TaskID, pr.RepositoryID, pr.PRNumber, message); err != nil {
		s.logger.Debug("record CI auto-fix exhaustion failed", zap.String("task_id", pr.TaskID), zap.Error(err))
		return
	}
	s.publishTaskCIOptionsState(ctx, pr.TaskID)
}

func (s *Service) publishTaskCIOptionsState(ctx context.Context, taskID string) {
	if s.githubService == nil || s.eventBus == nil || taskID == "" {
		return
	}
	resp, err := s.githubService.GetTaskCIOptionsResponse(context.WithoutCancel(ctx), taskID)
	if err != nil {
		s.logger.Debug("load task CI options for state publish failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	event := bus.NewEvent(events.GitHubTaskCIOptionsUpdated, ciAutomationStateEventSource, resp)
	if err := s.eventBus.Publish(context.WithoutCancel(ctx), events.GitHubTaskCIOptionsUpdated, event); err != nil {
		s.logger.Debug("publish task CI options state failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

func ciAutomationDuplicateFixAttemptBlocksMergeAt(state *github.TaskCIPRAutomationState, now time.Time) bool {
	if state == nil || state.LastFixEnqueuedAt == nil {
		return false
	}
	return now.Sub(*state.LastFixEnqueuedAt) <= ciAutomationFixBlockWindow
}

func ciAutomationCheckKey(check ciAutomationCheckSnapshot) string {
	return check.Name + "|" + check.Conclusion + "|" + check.HTMLURL + "|" + check.Output
}

func ciAutomationCurrentCheckpoint(feedback *github.PRFeedback) ciAutomationCheckpoint {
	return ciAutomationBuildDelta(feedback, ciAutomationCheckpoint{})
}

func ciAutomationCheckpointEmpty(checkpoint ciAutomationCheckpoint) bool {
	return len(checkpoint.FailedChecks) == 0 && len(checkpoint.Comments) == 0 && len(checkpoint.QueueRemovals) == 0
}

func ciAutomationNewQueueRemoval(pr *github.TaskPR, state *github.TaskCIPRAutomationState) (*ciAutomationQueueRemovalSnapshotData, bool) {
	snapshot, actionable := ciAutomationQueueRemovalSnapshot(pr)
	if !actionable || snapshot == nil || !ciAutomationQueueRemovalBelongsToCurrentHead(pr, state) {
		return nil, false
	}
	if state != nil && state.LastQueueFixEventID == snapshot.EventID {
		return nil, false
	}
	return snapshot, true
}

func ciAutomationQueueRemovalBelongsToCurrentHead(pr *github.TaskPR, state *github.TaskCIPRAutomationState) bool {
	if pr == nil || strings.TrimSpace(pr.HeadSHA) == "" || state == nil {
		return false
	}
	attemptHead := strings.TrimSpace(state.LastQueueAttemptHeadSHA)
	// A removal-only observation may establish a current-head baseline, but it
	// does not prove that Kandev ever queued this head. Require the merge
	// signature written by an actual merge attempt or active-entry adoption so
	// an old removal cannot spend a repair round after automation is enabled.
	return attemptHead != "" && attemptHead == strings.TrimSpace(pr.HeadSHA) && strings.TrimSpace(state.LastMergeSignature) != ""
}

func ciAutomationHasActiveMergeQueueEntry(pr *github.TaskPR) bool {
	if pr == nil || (pr.State != "" && !strings.EqualFold(strings.TrimSpace(pr.State), "open")) {
		return false
	}
	return strings.TrimSpace(pr.MergeQueueState) != ""
}

func ciAutomationQueueRemovalSnapshot(pr *github.TaskPR) (*ciAutomationQueueRemovalSnapshotData, bool) {
	if pr == nil || strings.TrimSpace(pr.MergeQueueLastRemovalID) == "" {
		return nil, false
	}
	cause := ciAutomationQueueRemovalCause(pr.MergeQueueLastRemovalReason, pr)
	return &ciAutomationQueueRemovalSnapshotData{
		EventID:      pr.MergeQueueLastRemovalID,
		Cause:        cause,
		Reason:       ciAutomationSanitizeSnapshotField(pr.MergeQueueLastRemovalReason),
		RemovedAt:    valueOrZeroTime(pr.MergeQueueLastRemovedAt),
		BeforeCommit: ciAutomationSanitizeSnapshotField(pr.MergeQueueLastRemovalBeforeSHA),
		Conflict:     ciAutomationQueueRemovalConflictEvidence(pr),
	}, ciAutomationQueueRemovalCauseActionable(cause)
}

func valueOrZeroTime(value *time.Time) time.Time {
	if value == nil {
		return time.Time{}
	}
	return value.UTC()
}

func ciAutomationQueueRemovalCause(reason string, pr *github.TaskPR) string {
	normalized := normalizeCIAutomationQueueRemovalReason(reason)
	switch normalized {
	case "checks_failed", "check_failed", "checks_failure", "ci_checks_failed", "ci_check_failed", "checks_failed_on_merge_group", "checks_failed_on_merge_queue":
		return ciAutomationQueueRemovalCauseChecksFailed
	case "checks_timed_out", "check_timed_out", "checks_timeout", "timeout", "timed_out", "checks_timed_out_on_merge_group":
		return ciAutomationQueueRemovalCauseChecksTimedOut
	case "merge_conflict", "merge_conflicts", "conflict", "unmergeable":
		return ciAutomationQueueRemovalCauseConflict
	case "manual", "removed_manually", "user_removed":
		return ciAutomationQueueRemovalCauseManual
	case "branch_protection", "branch_protection_failed", "required_branch_protection":
		return ciAutomationQueueRemovalCauseBranchProtection
	}
	if ciAutomationQueueRemovalConflictEvidence(pr) {
		return ciAutomationQueueRemovalCauseConflict
	}
	return ciAutomationQueueRemovalCauseUnknown
}

func normalizeCIAutomationQueueRemovalReason(reason string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range strings.ToLower(strings.TrimSpace(reason)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func ciAutomationQueueRemovalConflictEvidence(pr *github.TaskPR) bool {
	if pr == nil {
		return false
	}
	mergeable := strings.ToLower(strings.TrimSpace(pr.MergeableState))
	queueState := strings.ToLower(strings.TrimSpace(pr.MergeQueueState))
	return mergeable == "dirty" || strings.Contains(mergeable, "conflict") ||
		queueState == "unmergeable" || strings.Contains(queueState, "unmergeable")
}

func ciAutomationQueueRemovalCauseActionable(cause string) bool {
	return cause == ciAutomationQueueRemovalCauseChecksFailed ||
		cause == ciAutomationQueueRemovalCauseChecksTimedOut ||
		cause == ciAutomationQueueRemovalCauseConflict
}

func ciAutomationRenderPrompt(base string, pr *github.TaskPR, delta ciAutomationCheckpoint) string {
	if base = strings.TrimSpace(base); base != "" {
		return ciAutomationRenderPromptTemplate(base, ciAutomationRenderSnapshot(pr, delta))
	}
	return ""
}

func ciAutomationRenderPromptTemplate(base, snapshot string) string {
	if !strings.Contains(base, ciAutomationPRFeedbackToken) {
		return sysprompt.Wrap(base)
	}
	segments := strings.Split(base, ciAutomationPRFeedbackToken)
	parts := make([]string, 0, len(segments)*2)
	for i, segment := range segments {
		if segment = strings.TrimSpace(segment); segment != "" {
			parts = append(parts, sysprompt.Wrap(segment))
		}
		if i < len(segments)-1 && strings.TrimSpace(snapshot) != "" {
			parts = append(parts, snapshot)
		}
	}
	return strings.Join(parts, "\n\n")
}

func ciAutomationRenderSnapshot(pr *github.TaskPR, delta ciAutomationCheckpoint) string {
	if pr == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("PR: ")
	b.WriteString(fmt.Sprintf("%s/%s#%d", ciAutomationSanitizeSnapshotField(pr.Owner), ciAutomationSanitizeSnapshotField(pr.Repo), pr.PRNumber))
	if len(delta.FailedChecks) > 0 {
		b.WriteString("\n\nNew or changed failing checks:")
		for _, check := range delta.FailedChecks {
			b.WriteString(fmt.Sprintf("\n- %s: %s", ciAutomationSanitizeSnapshotField(check.Name), ciAutomationSanitizeSnapshotField(check.Conclusion)))
			if check.HTMLURL != "" {
				b.WriteString(" (")
				b.WriteString(ciAutomationSanitizeSnapshotField(check.HTMLURL))
				b.WriteString(")")
			}
		}
	}
	if len(delta.Comments) > 0 {
		b.WriteString("\n\nNew or changed review comments:")
		for _, comment := range delta.Comments {
			b.WriteString(fmt.Sprintf("\n- %s:%d %s", ciAutomationSanitizeSnapshotField(comment.Path), comment.Line, ciAutomationSanitizeSnapshotField(strings.TrimSpace(comment.Body))))
		}
	}
	if len(delta.QueueRemovals) > 0 {
		b.WriteString("\n\nMerge queue removal recovery:")
		for _, removal := range delta.QueueRemovals {
			b.WriteString("\n- event ")
			b.WriteString(ciAutomationSanitizeSnapshotField(removal.EventID))
			b.WriteString(": cause ")
			b.WriteString(ciAutomationSanitizeSnapshotField(removal.Cause))
			if removal.Reason != "" {
				b.WriteString("; reason ")
				b.WriteString(ciAutomationSanitizeSnapshotField(removal.Reason))
			}
			if removal.Conflict {
				b.WriteString("; conflict state observed")
			}
			if !removal.RemovedAt.IsZero() {
				b.WriteString("; removed at ")
				b.WriteString(removal.RemovedAt.UTC().Format(time.RFC3339))
			}
			if removal.BeforeCommit != "" {
				b.WriteString("; beforeCommit ")
				b.WriteString(ciAutomationSanitizeSnapshotField(removal.BeforeCommit))
				b.WriteString(" (not used as check identity)")
			}
		}
	}
	return b.String()
}

func ciAutomationSanitizeSnapshotField(value string) string {
	return strings.TrimSpace(ciAutomationSnapshotFieldReplacer.Replace(value))
}

func decodeCIAutomationCheckpoint(state *github.TaskCIPRAutomationState) ciAutomationCheckpoint {
	if state == nil || state.LastFixCheckpointJSON == "" {
		return ciAutomationCheckpoint{}
	}
	var checkpoint ciAutomationCheckpoint
	_ = json.Unmarshal([]byte(state.LastFixCheckpointJSON), &checkpoint)
	return checkpoint
}

func encodeCIAutomationCheckpoint(checkpoint ciAutomationCheckpoint) (string, string) {
	data, _ := json.Marshal(checkpoint)
	sum := sha256.Sum256(data)
	return string(data), hex.EncodeToString(sum[:])
}

func ciAutomationMergeSignature(pr *github.TaskPR) string {
	requiredReviews := 0
	requiredReviewsObserved := pr.RequiredReviews != nil
	if pr.RequiredReviews != nil {
		requiredReviews = *pr.RequiredReviews
	}
	payload, _ := json.Marshal(struct {
		TaskID, RepositoryID, HeadSHA, HeadBranch, State string
		PRNumber, Additions, Deletions                   int
		ChecksState, ReviewState, MergeableState         string
		ReviewCount, PendingReviewCount                  int
		RequiredReviewsObserved                          bool
		RequiredReviews, ChecksTotal, ChecksPassing      int
		UnresolvedReviewThreads                          int
	}{
		TaskID: pr.TaskID, RepositoryID: pr.RepositoryID, PRNumber: pr.PRNumber,
		HeadSHA: pr.HeadSHA, HeadBranch: pr.HeadBranch,
		State:     pr.State,
		Additions: pr.Additions, Deletions: pr.Deletions,
		ChecksState: pr.ChecksState, ReviewState: pr.ReviewState, MergeableState: pr.MergeableState,
		ReviewCount: pr.ReviewCount, PendingReviewCount: pr.PendingReviewCount,
		RequiredReviewsObserved: requiredReviewsObserved, RequiredReviews: requiredReviews,
		ChecksTotal: pr.ChecksTotal, ChecksPassing: pr.ChecksPassing,
		UnresolvedReviewThreads: pr.UnresolvedReviewThreads,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
