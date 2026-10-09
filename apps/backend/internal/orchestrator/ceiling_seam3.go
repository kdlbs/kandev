package orchestrator

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// seam3Refusal is ensureSessionRunning's distinguishable ceiling-refusal
// sentinel (AC-47): a plain error cannot be told apart from a materialization
// failure, so each of seam 3's four call shapes needs to recognize this exact
// type to choose its own disposition. ensureSessionRunning's own signature
// carries no replay fields, so it never persists a deferral itself — it only
// decides admit/refuse and reports the refusal for its caller to record.
type seam3Refusal struct {
	reasonCode string
	// population, populationKnown and ceiling are the admission decision's own
	// reading, carried so a caller-side disposition can pass them on to
	// deferCeilingRefusal for the AC-49 card carrier's content.
	population      int
	populationKnown bool
	ceiling         int
	// deferred is set by the caller-side disposition once a ceiling_deferred
	// record has actually been written for this refusal (AC-47c2): a caller
	// with two possible dispositions (promptTask's prompt_ensure vs AC-47c1's
	// refuse-unchanged) inspects it to decide whether it or the ceiling now
	// owns restoration.
	deferred bool
}

func (e *seam3Refusal) Error() string {
	return fmt.Sprintf("session ceiling refused seam 3 admission (%s)", e.reasonCode)
}

func isSeam3Refusal(err error) (*seam3Refusal, bool) {
	var refusal *seam3Refusal
	if errors.As(err, &refusal) {
		return refusal, true
	}
	return nil, false
}

// errSeam3WorkflowStepEnsureDeferred is returned by StartSessionForWorkflowStep
// when its own AC-47f pre-consultation is refused: the workflow_step_ensure
// record has already been written, and (per AC-47f1) advanceTaskWorkflowStep
// was never called, so the card is left exactly where it was.
var errSeam3WorkflowStepEnsureDeferred = errors.New("session ceiling deferred the workflow step launch")

// seam3CallShape distinguishes tryEnsureExecution's two production callers
// (AC-47e): an explicit token supplied by each caller, never inferred from
// context or goroutine identity.
type seam3CallShape string

const (
	// seam3CallShapeViewing is EnsureSession's call shape (AC-47a): nothing to
	// replay and nobody waiting, so a refusal is swallowed, not deferred.
	seam3CallShapeViewing seam3CallShape = "viewing"
	// seam3CallShapeQueueDrain is scheduleAutoResumeForWorkflowQueue's call
	// shape (AC-47d): a refusal defers, carrying the queued message id.
	seam3CallShapeQueueDrain seam3CallShape = "queue_drain"
)

// admitSeam3 is ensureSessionRunning's own gate: consulted once, after the
// already-running early return and before the first attemptColdResume
// (AC-4e1 — the loop's retry is recovering an already-admitted launch, not
// requesting a new one).
func (s *Service) admitSeam3(
	ctx context.Context, taskID, sessionID string, origin launchOrigin,
) (*sessionKeyedCeilingReservation, *seam3Refusal) {
	decision := s.sessionCeiling.admit(ctx, admissionRequest{
		taskID: taskID, sessionID: sessionID, origin: origin, seam: "ensureSessionRunning",
	})
	if decision.admitted {
		return &sessionKeyedCeilingReservation{
			controller: s.sessionCeiling, key: decision.reservationKey,
			manualOverride: decision.manualOverride, population: decision.population,
			populationKnown: decision.populationKnown, ceiling: decision.ceiling,
		}, nil
	}
	return nil, &seam3Refusal{
		reasonCode: decision.reasonCode, population: decision.population,
		populationKnown: decision.populationKnown, ceiling: decision.ceiling,
	}
}

// admitOrDeferWorkflowStepEnsure is StartSessionForWorkflowStep's AC-47f/AC-47f1
// pre-consultation: an ordinary admission request for the session id it
// already holds, consulted before advanceTaskWorkflowStep mutates the task.
// On refusal it writes the workflow_step_ensure record itself (AC-47's
// caller-owns-the-disposition split extends to this pre-check, not only to
// the in-function gate) so the caller can return without ever calling
// advanceTaskWorkflowStep.
func (s *Service) admitOrDeferWorkflowStepEnsure(
	ctx context.Context, taskID, sessionID, workflowStepID string,
) (reservation *sessionKeyedCeilingReservation, deferred bool, err error) {
	return s.admitOrDeferWorkflowStepEnsureWithBinding(
		ctx, taskID, sessionID, workflowStepID, ceilingEntryBindingFromContext(ctx),
	)
}

func (s *Service) admitOrDeferWorkflowStepEnsureWithBinding(
	ctx context.Context, taskID, sessionID, workflowStepID string,
	binding *models.CeilingWorkflowEntryBinding,
) (reservation *sessionKeyedCeilingReservation, deferred bool, err error) {
	if binding == nil && s.workflowStepGetter != nil {
		if step, stepErr := s.workflowStepGetter.GetStep(ctx, workflowStepID); stepErr == nil {
			binding, _ = s.workflowEntryBindingForStep(ctx, taskID, step, sessionID)
		}
	}
	decision := s.sessionCeiling.admit(ctx, admissionRequest{
		taskID: taskID, sessionID: sessionID, origin: launchOriginAutomatic, seam: "startSessionForWorkflowStepPreConsult",
	})
	if decision.admitted {
		return &sessionKeyedCeilingReservation{
			controller: s.sessionCeiling, key: decision.reservationKey,
			manualOverride: decision.manualOverride, population: decision.population,
			populationKnown: decision.populationKnown, ceiling: decision.ceiling,
		}, false, nil
	}
	payload := map[string]interface{}{
		metaKeySessionID:      sessionID,
		metaKeyWorkflowStepID: workflowStepID,
	}
	if binding != nil {
		payload[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(*binding)
	}
	if err := s.deferCeilingRefusal(ctx, taskID, sessionID, models.CeilingLaunchWorkflowStepEnsure, payload, decision.reasonCode,
		decision.population, decision.populationKnown, decision.ceiling); err != nil {
		s.logger.Zap().Error("could not persist a ceiling deferral; the workflow step launch could not be admitted or recorded",
			zap.String("task_id", taskID), zap.String("session_id", sessionID),
			zap.String("workflow_step_id", workflowStepID), zap.Error(err))
		return nil, false, err
	}
	return nil, true, nil
}

// deferSeam3QueueDrainRefusal is tryEnsureExecution's AC-47d disposition for
// its queue-drain call shape: the prompt, attachments and metadata are
// already durable in the message queue, so the record needs only the
// session id and the queued message's own id (AC-47d1's already-drained
// check at retry time is undecidable without it).
func (s *Service) deferSeam3QueueDrainRefusal(ctx context.Context, taskID, sessionID, queuedMessageID string, refusal *seam3Refusal, bindings ...*models.CeilingWorkflowEntryBinding) {
	payload := map[string]interface{}{
		metaKeySessionID:    sessionID,
		"queued_message_id": queuedMessageID,
	}
	if len(bindings) > 0 && bindings[0] != nil {
		payload[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(*bindings[0])
	}
	payload = s.enrichCeilingLaunchPayload(ctx, taskID, sessionID, payload)
	if err := s.deferCeilingRefusal(ctx, taskID, sessionID, models.CeilingLaunchQueueDrainEnsure, payload, refusal.reasonCode,
		refusal.population, refusal.populationKnown, refusal.ceiling); err != nil {
		s.logger.Zap().Error("could not persist a ceiling deferral; the workflow queue drain could not be recorded",
			zap.String("task_id", taskID), zap.String("session_id", sessionID),
			zap.String("queued_message_id", queuedMessageID), zap.Error(err))
	}
}

// seam3PromptEnsureNonReconstructableOptionsSet reports AC-47c1: whether
// options carries any AC-47c(a) field at other than its zero value. Function
// values and in-flight claim or session identity are meaningless outside the
// originating call, and every production caller that sets one already holds
// its own failure path, so a refusal here is returned to the caller unchanged
// rather than deferred.
func seam3PromptEnsureNonReconstructableOptionsSet(options promptTaskOptions) bool {
	return options.afterClaim != nil ||
		options.onAccepted != nil ||
		options.claimEntryID != "" ||
		options.expectedCurrentTurnID != "" ||
		options.promptDispatchRecovery != nil ||
		options.expectedSessionIdentity != nil ||
		options.afterDispatch != nil ||
		options.beforeDispatch != nil ||
		options.afterDispatchAdmission != nil ||
		options.disableDispatchRetry
}

// seam3PromptEnsurePayload builds the AC-42 "prompt_ensure" replay row from
// promptTask's own frame, at the point the gate is consulted.
func seam3PromptEnsurePayload(
	sessionID, prompt, model string, planMode bool, attachments []v1.MessageAttachment,
	dispatchOnly bool, options promptTaskOptions,
) map[string]interface{} {
	payload := map[string]interface{}{
		metaKeySessionID:              sessionID,
		metaKeyPrompt:                 prompt,
		sessionModelConfigKey:         model,
		metaKeyPlanMode:               planMode,
		metaKeyAttachments:            attachments,
		"dispatch_only":               dispatchOnly,
		"lifecycle_prompt":            options.lifecyclePrompt,
		"preserve_prompt_context":     options.preservePromptContext,
		"reserve_turn_until_dispatch": options.reserveTurnUntilDispatch,
		"require_nonterminal_session": options.requireNonterminalSession,
		"prompt_already_composed":     options.promptAlreadyComposed,
		"fallback_launch_prompt":      options.fallbackLaunchPrompt,
		"fallback_retry_prompt":       options.fallbackRetryPrompt,
		"prompt_reference_context":    options.promptReferenceContext,
	}
	if options.ceilingEntryBinding != nil {
		payload[models.CeilingLaunchEntryBindingKey] = ceilingEntryBindingValue(*options.ceilingEntryBinding)
	}
	return payload
}

// disposeSeam3PromptEnsureRefusal is promptTask's AC-47/AC-47c1/AC-47c2
// disposition for a seam-3 refusal: writes a prompt_ensure record from its own
// frame UNLESS the call carries a non-reconstructable AC-47c(a) field, in
// which case the refusal is returned to the caller unchanged (deferred=false)
// so the caller's own restoration path owns it, exactly as AC-47d already
// argues for queue_drain_ensure. The two dispositions are distinguishable via
// the returned bool, per AC-47c2's requirement that a caller not infer which
// happened from the presence of a record it would have to re-read.
func (s *Service) disposeSeam3PromptEnsureRefusal(
	ctx context.Context, taskID, sessionID, prompt, model string, planMode bool,
	attachments []v1.MessageAttachment, dispatchOnly bool, options promptTaskOptions, refusal *seam3Refusal,
) error {
	if seam3PromptEnsureNonReconstructableOptionsSet(options) {
		return nil
	}
	payload := seam3PromptEnsurePayload(sessionID, prompt, model, planMode, attachments, dispatchOnly, options)
	payload = s.enrichCeilingLaunchPayload(ctx, taskID, sessionID, payload)
	if err := s.deferCeilingRefusal(ctx, taskID, sessionID, models.CeilingLaunchPromptEnsure, payload, refusal.reasonCode,
		refusal.population, refusal.populationKnown, refusal.ceiling); err != nil {
		s.logger.Zap().Error("could not persist a ceiling deferral; the prompt could not be admitted or recorded",
			zap.String("task_id", taskID), zap.String("session_id", sessionID), zap.Error(err))
		return err
	}
	refusal.deferred = true
	return nil
}

// classifyEnsureSessionRunningFailureForPrompt is promptTask's single error
// path out of ensureSessionRunning: a seam-3 refusal is disposed of via
// disposeSeam3PromptEnsureRefusal (AC-47/AC-47c1/AC-47c2) and returned to the
// caller unchanged either way, and every other failure keeps promptTask's
// pre-existing classification (runtime-unavailable vs. a generic wrap).
func (s *Service) classifyEnsureSessionRunningFailureForPrompt(
	ctx context.Context, taskID, sessionID, prompt, model string, planMode bool,
	attachments []v1.MessageAttachment, dispatchOnly bool, options promptTaskOptions, err error,
) error {
	if refusal, ok := isSeam3Refusal(err); ok {
		if deferErr := s.disposeSeam3PromptEnsureRefusal(
			ctx, taskID, sessionID, prompt, model, planMode, attachments, dispatchOnly, options, refusal,
		); deferErr != nil {
			return deferErr
		}
		return err
	}
	if errors.Is(err, errSessionAwaitingRuntimeLaunch) {
		return fmt.Errorf("%w: failed to ensure session is running: %w", ErrSessionRuntimeUnavailable, err)
	}
	return fmt.Errorf("failed to ensure session is running: %w", err)
}
