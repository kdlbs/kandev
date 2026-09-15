package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/orchestrator/executor"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	workflowmove "github.com/kandev/kandev/internal/workflow/move"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// ceilingDeferredTaskLister is the narrow repository capability AC-50b's
// sweep needs. Reached through a type assertion on s.repo, following the
// admittedSessionLister idiom in session_ceiling_config.go, rather than
// growing sessionExecutorStore for a capability only the sweep uses.
type ceilingDeferredTaskLister interface {
	ListTasksWithCeilingDeferred(ctx context.Context) ([]*models.Task, error)
}

// ceilingReplayOutcome is the three-way disposition every per-kind replay
// function reduces to, however differently each of the 7 entry points
// reports it.
type ceilingReplayOutcome int

const (
	// ceilingReplayStillDeferred means the launch asked for admission again
	// and was refused again: the record is left exactly as stored (AC-32),
	// including its original queued_at, for a later pass.
	ceilingReplayStillDeferred ceilingReplayOutcome = iota
	// ceilingReplaySucceeded means the launch was admitted and dispatched:
	// the record is cleared.
	ceilingReplaySucceeded
	// ceilingReplayFailed means the launch failed for a reason unrelated to
	// the ceiling (AC-15d): the record is retained for a later pass and the
	// failure is logged, since — unlike a refusal — nothing else observes it.
	ceilingReplayFailed
)

// drainDeferredCeilingLaunches is AC-15c's pass driver: one sweep tick walks
// every ceiling-deferred task once, in id-ascending order (AC-50c), retrying
// each in turn. A candidate that fails for a non-ceiling reason is skipped
// for the rest of this pass (AC-15d) rather than retried immediately or
// stopping the pass.
func (s *Service) drainDeferredCeilingLaunches(ctx context.Context) {
	lister, ok := s.repo.(ceilingDeferredTaskLister)
	if !ok {
		return
	}
	tasks, err := lister.ListTasksWithCeilingDeferred(ctx)
	if err != nil {
		s.logger.Zap().Warn("could not list ceiling-deferred tasks for the retry sweep", zap.Error(err))
		return
	}
	for _, task := range tasks {
		s.retryOneDeferredCeilingLaunch(ctx, task)
	}
}

// retryOneDeferredCeilingLaunch reads one task's deferral, evaluates AC-17b's
// drop reasons, and otherwise dispatches to the kind's own replay.
func (s *Service) retryOneDeferredCeilingLaunch(ctx context.Context, task *models.Task) {
	if task == nil || task.ID == "" {
		return
	}
	raw, _ := task.Metadata[models.MetaKeyDeferredLaunch].(map[string]interface{})
	deferral, err := models.ReadCeilingDeferral(raw)
	if err != nil {
		var unreplayable *models.UnreplayableCeilingRecordError
		if errors.As(err, &unreplayable) {
			s.dropCeilingDeferral(ctx, task, "", deferral, ceilingReasonDroppedUnreplayableRecord,
				fmt.Sprintf("the deferred launch record could not be replayed: %v", err))
		}
		// Any other read failure (most likely: a concurrent writer already
		// cleared the record between the list and this read) is not this
		// task's fault; leave it for the next tick to see the current state.
		return
	}

	if reasonCode, detail, drop := s.evaluateCeilingDropReasons(ctx, task, deferral); drop {
		s.dropCeilingDeferral(ctx, task, sessionIDFromCeilingPayload(deferral), deferral, reasonCode, detail)
		return
	}

	switch s.replayCeilingDeferral(ctx, task, deferral) {
	case ceilingReplaySucceeded:
		s.clearCeilingDeferredRecord(ctx, task.ID)
	case ceilingReplayFailed:
		s.logger.Zap().Warn("ceiling retry replay failed for a non-ceiling reason; will retry on a later sweep",
			zap.String("task_id", task.ID), zap.String("kind", string(deferral.Kind)))
	case ceilingReplayStillDeferred:
		// Left exactly as stored: the replay's own admission gate already
		// re-persisted this same record via deferCeilingRefusal's
		// equivalence check (AC-32/AC-55a). AC-49g: retry the card note if
		// an earlier attempt to write it failed.
		s.attemptCeilingSurfaceWrite(ctx, task.ID)
	}
}

// sessionIDFromCeilingPayload extracts the session id a deferral's payload
// names, for the six kinds that carry one. CeilingLaunchStart carries none:
// no session exists yet at seam 1.
func sessionIDFromCeilingPayload(deferral models.CeilingDeferral) string {
	if deferral.Kind == models.CeilingLaunchStart {
		return ""
	}
	return stringField(deferral.Payload, metaKeySessionID)
}

// evaluateCeilingDropReasons is AC-17b's closed set of drop reasons. Reasons
// (a)-(c) are universal; (d) and (e) are scoped to the "start" kind only —
// concrete tracing showed only startTask's own gate embeds the terminal-PR
// guard, and none of the other six entry points call shouldAutoStartStep
// internally, so applying either check to an unrelated kind risks dropping a
// legitimate continuation that has nothing to do with step-entry auto-start
// semantics. Reason (f), the unreplayable record, is handled by the caller
// before this is ever reached.
func (s *Service) evaluateCeilingDropReasons(
	ctx context.Context, task *models.Task, deferral models.CeilingDeferral,
) (reasonCode, detail string, drop bool) {
	if task.ArchivedAt != nil {
		return ceilingReasonDroppedTaskIneligible, "task is archived", true
	}
	if task.State == v1.TaskStateCancelled {
		return ceilingReasonDroppedTaskIneligible, "task is cancelled", true
	}
	if sessionID := sessionIDFromCeilingPayload(deferral); sessionID != "" {
		if _, err := s.repo.GetTaskSession(ctx, sessionID); err != nil {
			if errors.Is(err, models.ErrTaskSessionNotFound) {
				return ceilingReasonDroppedTaskIneligible, "session no longer exists", true
			}
			// An unrelated read failure isn't grounds to drop the record;
			// the replay attempt below will hit the same failure and retain it.
			return "", "", false
		}
	}
	if deferral.Kind == models.CeilingLaunchStart {
		if !s.shouldAutoStartStep(ctx, task.WorkflowStepID) {
			return ceilingReasonDroppedTaskIneligible, "workflow step no longer auto-starts", true
		}
		if s.shouldSkipTerminalPRAutoStart(ctx, task) {
			return ceilingReasonDroppedLaunchGateDeclined, "terminal PR auto-start gate declined the launch", true
		}
	}
	return "", "", false
}

// dropCeilingDeferral is AC-17b/AC-17c's disposition: clear the record, log
// the drop (AC-22/22a) and, where a session exists to attach it to, write a
// minimal one-shot drop note. The full AC-49 suppression-and-bounded-retry
// card surface for an ONGOING refusal is separate, later scope; this is only
// the terminal "this will never be retried again" notice.
func (s *Service) dropCeilingDeferral(
	ctx context.Context, task *models.Task, sessionID string, deferral models.CeilingDeferral, reasonCode, detail string,
) {
	s.logger.Zap().Warn("dropping a ceiling-deferred launch",
		zap.String("task_id", task.ID),
		zap.String("kind", string(deferral.Kind)),
		zap.String(ceilingFieldReasonCode, reasonCode),
		zap.String("detail", detail))

	if sessionID != "" && s.messageCreator != nil {
		if err := s.messageCreator.CreateSessionMessage(
			ctx, task.ID,
			fmt.Sprintf("The queued launch for this session was dropped: %s.", detail),
			sessionID, string(v1.MessageTypeStatus), "",
			map[string]interface{}{metaKeyVariant: metaVariantCeiling, ceilingFieldReasonCode: reasonCode},
			false,
		); err != nil {
			s.logger.Zap().Warn("could not write the ceiling drop card note",
				zap.String("task_id", task.ID), zap.String("session_id", sessionID), zap.Error(err))
		}
	}

	s.clearCeilingDeferredRecord(ctx, task.ID)
	s.publishTaskUpdatedByID(ctx, task.ID)
}

// clearCeilingDeferredRecord is AC-17's "clear the whole record" disposition,
// applied on both a successful replay and a drop, under the same
// compare-and-set retry loop deferCeilingRefusal itself uses.
func (s *Service) clearCeilingDeferredRecord(ctx context.Context, taskID string) {
	for attempt := 0; attempt < deferredLaunchCASRetryBudget; attempt++ {
		existingRaw, prior, err := s.repo.GetTaskDeferredLaunch(ctx, taskID)
		if err != nil {
			s.logger.Zap().Warn("could not read deferred launch while clearing a ceiling record",
				zap.String("task_id", taskID), zap.Error(err))
			return
		}
		record := stripCeilingRecordKeys(existingRaw)
		stored, lostCompare, err := s.repo.SetTaskDeferredLaunchIfUnchanged(ctx, taskID, prior, record)
		if err != nil {
			s.logger.Zap().Warn("could not clear a ceiling deferred record",
				zap.String("task_id", taskID), zap.Error(err))
			return
		}
		if stored {
			return
		}
		if lostCompare {
			continue
		}
		return
	}
	s.logger.Zap().Warn("could not clear a ceiling deferred record after repeated compare-and-set losses",
		zap.String("task_id", taskID))
}

// stripCeilingRecordKeys removes every ceiling_-prefixed key from a
// deferred_launch record, leaving any coexisting WIP-overflow or
// dependency-chain intent untouched: the ceiling_ prefix (IsCeilingRecordKey)
// is exactly how the two halves of the shared record partition without
// either side tracking the other's keys. SetTaskDeferredLaunchIfUnchanged
// rejects a nil value, so a record with no keys at all is an empty object,
// never nil.
func stripCeilingRecordKeys(existing interface{}) map[string]interface{} {
	current, ok := existing.(map[string]interface{})
	if !ok {
		return map[string]interface{}{}
	}
	stripped := make(map[string]interface{}, len(current))
	for key, value := range current {
		if models.IsCeilingRecordKey(key) {
			continue
		}
		stripped[key] = value
	}
	return stripped
}

// replayCeilingDeferral dispatches to the one replay function matching the
// deferral's kind. The closed set is exhaustive; a kind outside it was
// already rejected as unreplayable by ReadCeilingDeferral before this point.
func (s *Service) replayCeilingDeferral(ctx context.Context, task *models.Task, deferral models.CeilingDeferral) ceilingReplayOutcome {
	switch deferral.Kind {
	case models.CeilingLaunchStart:
		return s.replayCeilingLaunchStart(ctx, task, deferral.Payload)
	case models.CeilingLaunchStartCreated:
		return s.replayCeilingLaunchStartCreated(ctx, task, deferral.Payload)
	case models.CeilingLaunchPromptEnsure:
		return s.replayCeilingLaunchPromptEnsure(ctx, task, deferral.Payload)
	case models.CeilingLaunchWorkflowStepEnsure:
		return s.replayCeilingLaunchWorkflowStepEnsure(ctx, task, deferral.Payload)
	case models.CeilingLaunchQueueDrainEnsure:
		return s.replayCeilingLaunchQueueDrainEnsure(ctx, task, deferral.Payload)
	case models.CeilingLaunchResume:
		return s.replayCeilingLaunchResume(ctx, task, deferral.Payload)
	case models.CeilingLaunchDynamicRelaunch:
		return s.replayCeilingLaunchDynamicRelaunch(ctx, task, deferral.Payload)
	default:
		return ceilingReplayFailed
	}
}

// stringField and boolField read a primitive out of a generically-decoded
// payload map, defaulting to the zero value for an absent or mistyped key.
func stringField(payload map[string]interface{}, key string) string {
	v, _ := payload[key].(string)
	return v
}

func boolField(payload map[string]interface{}, key string) bool {
	v, _ := payload[key].(bool)
	return v
}

// int64Field reads an integer out of a generically-decoded payload map. A
// JSON round trip through map[string]interface{} always decodes numbers as
// float64, so this cannot type-assert int64 directly like boolField does.
func int64Field(payload map[string]interface{}, key string) int64 {
	switch v := payload[key].(type) {
	case float64:
		return int64(v)
	case int64:
		return v
	default:
		return 0
	}
}

// remintCeilingLaunchCredentials refreshes the short-lived Office runtime
// credentials redactedCeilingLaunchEnv stripped before persisting, using
// whichever CeilingLaunchCredentialReminter was wired (nil for a non-Office
// deployment, or when no reminter is registered — env round-trips unchanged
// in that case). A re-mint failure is not fatal to the replay attempt: it
// logs and falls back to the captured env, which is only stale — not
// wrong — for a non-Office launch, since only Office launches carry these
// credential keys in the first place.
func (s *Service) remintCeilingLaunchCredentials(ctx context.Context, taskID string, env map[string]string) map[string]string {
	if s.ceilingCredentialReminter == nil || len(env) == 0 {
		return env
	}
	refreshed, err := s.ceilingCredentialReminter.RemintCeilingLaunchCredentials(ctx, taskID, env)
	if err != nil {
		s.logger.Zap().Warn("ceiling replay: credential re-mint failed; replaying with the captured env",
			zap.String("task_id", taskID), zap.Error(err))
		return env
	}
	return refreshed
}

// decodeCeilingPayloadField reconstructs a typed replay field from its
// generically-decoded form. The original payload was built by placing typed
// Go values directly into a map[string]interface{} that the repository then
// json.Marshaled; marshaling this generic form back to bytes and unmarshaling
// into target recovers the original shape through the same struct tags. A
// malformed or absent value leaves target at its zero value.
func decodeCeilingPayloadField(raw interface{}, target interface{}) {
	if raw == nil {
		return
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return
	}
	_ = json.Unmarshal(data, target)
}

// replayCeilingLaunchStart replays an AC-42 "start" record by calling
// startTask with the payload seam1StartPayload originally captured.
func (s *Service) replayCeilingLaunchStart(ctx context.Context, task *models.Task, payload map[string]interface{}) ceilingReplayOutcome {
	var attachments []v1.MessageAttachment
	decodeCeilingPayloadField(payload[metaKeyAttachments], &attachments)
	var env map[string]string
	decodeCeilingPayloadField(payload["env"], &env)
	var route *executor.RouteOverride
	decodeCeilingPayloadField(payload["route"], &route)
	var additionalSkillSlugs []string
	decodeCeilingPayloadField(payload["additional_skill_slugs"], &additionalSkillSlugs)
	var entryOptions *workflowmove.EntryOptions
	decodeCeilingPayloadField(payload["entry_options"], &entryOptions)

	env = s.remintCeilingLaunchCredentials(ctx, task.ID, env)

	opts := startTaskOptions{
		ProfileExplicit:      boolField(payload, "profile_explicit"),
		Env:                  env,
		Route:                route,
		AdditionalSkillSlugs: additionalSkillSlugs,
		EntryOptions:         entryOptions,
		WorkflowEntryID:      int64Field(payload, "workflow_entry_id"),
		// Origin must round-trip rather than be re-derived from auto_start:
		// an AC-13d caller (Office-routed launch) passes autoStart=false with
		// an explicit automatic Origin override, and re-deriving from
		// auto_start alone would replay it as a manual override that bypasses
		// the ceiling.
		Origin: launchOrigin(stringField(payload, "origin")),
	}
	if spawnRaw, ok := payload["spawn_origin"].(map[string]interface{}); ok {
		opts.SpawnOrigin = &SpawnOrigin{
			TaskID:      stringField(spawnRaw, metaKeyTaskID),
			SessionID:   stringField(spawnRaw, metaKeySessionID),
			SessionName: stringField(spawnRaw, "session_name"),
		}
	}

	execution, err := s.startTask(
		ctx, task.ID,
		stringField(payload, metaKeyAgentProfileID),
		stringField(payload, "executor_id"),
		stringField(payload, metaKeyExecutorProfile),
		stringField(payload, "priority"),
		stringField(payload, metaKeyPrompt),
		stringField(payload, metaKeyWorkflowStepID),
		boolField(payload, metaKeyPlanMode),
		boolField(payload, "auto_start"),
		attachments,
		opts,
	)
	// startTask reports a repeat refusal as ErrCeilingLaunchDeferred, not as
	// (nil, nil) — ceilingReplayOutcomeFromExecution's execution==nil case
	// would otherwise never fire and a still-refused replay would be
	// misclassified as ceilingReplayFailed (a non-ceiling failure), skipping
	// the AC-49g card-surface retry and logging a misleading warning.
	if errors.Is(err, ErrCeilingLaunchDeferred) {
		return ceilingReplayStillDeferred
	}
	return ceilingReplayOutcomeFromExecution(execution, err)
}

// replayCeilingLaunchStartCreated replays an AC-42d "start_created" record.
func (s *Service) replayCeilingLaunchStartCreated(ctx context.Context, task *models.Task, payload map[string]interface{}) ceilingReplayOutcome {
	var attachments []v1.MessageAttachment
	decodeCeilingPayloadField(payload[metaKeyAttachments], &attachments)
	var references []v1.EntityReference
	decodeCeilingPayloadField(payload["references"], &references)

	options := startCreatedSessionOptions{
		skipTaskDescriptionFallback: boolField(payload, "skip_task_description_fallback"),
		promptAlreadyComposed:       boolField(payload, "prompt_already_composed"),
		retryPrompt:                 stringField(payload, "retry_prompt"),
		canvasGuidanceResolved:      boolField(payload, "canvas_guidance_resolved"),
		includeCanvasGuidance:       boolField(payload, "include_canvas_guidance"),
		promptReferencesPrepared:    boolField(payload, "prompt_references_prepared"),
		preserveDirectPrompt:        boolField(payload, "preserve_direct_prompt"),
	}

	execution, err := s.startCreatedSession(
		ctx, task.ID,
		stringField(payload, metaKeySessionID),
		stringField(payload, metaKeyAgentProfileID),
		stringField(payload, metaKeyPrompt),
		boolField(payload, "skip_message_record"),
		boolField(payload, metaKeyPlanMode),
		boolField(payload, "auto_start"),
		attachments,
		references,
		stringField(payload, "prompt_reference_context"),
		options,
	)
	return ceilingReplayOutcomeFromExecution(execution, err)
}

// replayCeilingLaunchPromptEnsure replays an AC-42 "prompt_ensure" record.
// promptTask has no seam-3 "deferred" return of its own; the caller (seam 3's
// own ensureSessionRunning gate) reports a refusal as a *seam3Refusal error,
// and an accept-then-fail-after dispatch as an acceptedPromptDispatchError —
// both distinguishable from an ordinary failure.
func (s *Service) replayCeilingLaunchPromptEnsure(ctx context.Context, task *models.Task, payload map[string]interface{}) ceilingReplayOutcome {
	var attachments []v1.MessageAttachment
	decodeCeilingPayloadField(payload[metaKeyAttachments], &attachments)

	options := promptTaskOptions{
		lifecyclePrompt:           boolField(payload, "lifecycle_prompt"),
		preservePromptContext:     boolField(payload, "preserve_prompt_context"),
		reserveTurnUntilDispatch:  boolField(payload, "reserve_turn_until_dispatch"),
		requireNonterminalSession: boolField(payload, "require_nonterminal_session"),
		promptAlreadyComposed:     boolField(payload, "prompt_already_composed"),
		fallbackLaunchPrompt:      stringField(payload, "fallback_launch_prompt"),
		fallbackRetryPrompt:       stringField(payload, "fallback_retry_prompt"),
	}

	_, err := s.promptTask(
		ctx, task.ID,
		stringField(payload, metaKeySessionID),
		stringField(payload, metaKeyPrompt),
		stringField(payload, sessionModelConfigKey),
		boolField(payload, metaKeyPlanMode),
		attachments,
		boolField(payload, "dispatch_only"),
		launchOriginAutomatic,
		options,
	)
	if err == nil {
		return ceilingReplaySucceeded
	}
	if _, ok := isSeam3Refusal(err); ok {
		return ceilingReplayStillDeferred
	}
	if accepted, ok := err.(interface{ DetachedResumeAccepted() bool }); ok && accepted.DetachedResumeAccepted() {
		// agentctl accepted the prompt despite the reported error: the
		// launch did happen, so this is not a ceiling matter any longer.
		return ceilingReplaySucceeded
	}
	return ceilingReplayFailed
}

// replayCeilingLaunchWorkflowStepEnsure replays an AC-42 "workflow_step_ensure"
// record. StartSessionForWorkflowStep reports its own still-deferred outcome
// via the distinguishable errSeam3WorkflowStepEnsureDeferred sentinel.
func (s *Service) replayCeilingLaunchWorkflowStepEnsure(ctx context.Context, task *models.Task, payload map[string]interface{}) ceilingReplayOutcome {
	err := s.StartSessionForWorkflowStep(
		ctx, task.ID,
		stringField(payload, metaKeySessionID),
		stringField(payload, metaKeyWorkflowStepID),
	)
	switch {
	case err == nil:
		return ceilingReplaySucceeded
	case errors.Is(err, errSeam3WorkflowStepEnsureDeferred):
		return ceilingReplayStillDeferred
	default:
		return ceilingReplayFailed
	}
}

// replayCeilingLaunchQueueDrainEnsure replays an AC-42 "queue_drain_ensure"
// record. tryEnsureExecution reports nothing at all — success is read back
// from the session's own state, the same signal AC-1 population counting
// itself uses.
func (s *Service) replayCeilingLaunchQueueDrainEnsure(ctx context.Context, task *models.Task, payload map[string]interface{}) ceilingReplayOutcome {
	sessionID := stringField(payload, metaKeySessionID)
	s.tryEnsureExecution(ctx, sessionID, seam3CallShapeQueueDrain, launchOriginAutomatic, stringField(payload, "queued_message_id"))

	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return ceilingReplayStillDeferred
	}
	if isAC1SessionState(session.State) {
		return ceilingReplaySucceeded
	}
	return ceilingReplayStillDeferred
}

// replayCeilingLaunchResume replays an AC-42 "resume" record.
func (s *Service) replayCeilingLaunchResume(ctx context.Context, task *models.Task, payload map[string]interface{}) ceilingReplayOutcome {
	execution, err := s.ResumeTaskSessionWithOptions(
		ctx, task.ID,
		stringField(payload, metaKeySessionID),
		executor.ResumeOptions{
			AllowBranchReplacement: boolField(payload, "allow_branch_replacement"),
			Origin:                 string(launchOriginAutomatic),
		},
	)
	return ceilingReplayOutcomeFromExecution(execution, err)
}

// replayCeilingLaunchDynamicRelaunch replays an AC-42f "dynamic_relaunch"
// record. relaunchDynamicTaskAfterFailure reports only a bool: false is
// ambiguous between still-refused and a non-ceiling failure, and both are
// handled identically here — retained for a later pass — since its own
// admission gate re-persists an unchanged record on a repeat refusal either
// way (AC-32/AC-55a).
func (s *Service) replayCeilingLaunchDynamicRelaunch(ctx context.Context, task *models.Task, payload map[string]interface{}) ceilingReplayOutcome {
	data := watcher.AgentEventData{
		TaskID:           stringField(payload, metaKeyTaskID),
		SessionID:        stringField(payload, metaKeySessionID),
		AgentExecutionID: stringField(payload, "agent_execution_id"),
		AgentProfileID:   stringField(payload, metaKeyAgentProfileID),
	}
	if s.relaunchDynamicTaskAfterFailure(ctx, data, stringField(payload, "execution_profile_id"), launchOriginAutomatic) {
		return ceilingReplaySucceeded
	}
	return ceilingReplayStillDeferred
}

// ceilingReplayOutcomeFromExecution is the shared classification for the four
// entry points (start, start_created, resume — and, by the same contract,
// any future kind) that return (*executor.TaskExecution, error): a non-nil
// execution is success, a nil execution with a nil error is still-deferred
// (the seam's own gate re-persisted the record internally), and anything
// else is a non-ceiling failure.
func ceilingReplayOutcomeFromExecution(execution *executor.TaskExecution, err error) ceilingReplayOutcome {
	switch {
	case err != nil:
		return ceilingReplayFailed
	case execution == nil:
		return ceilingReplayStillDeferred
	default:
		return ceilingReplaySucceeded
	}
}
