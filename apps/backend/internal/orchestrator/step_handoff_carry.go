package orchestrator

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// drainQueuedMessageForPromptableSessionWithHandoff is a sibling of
// drainQueuedMessageForPromptableSessionOutcome for the step-entry dispatch
// branches whose only activity is draining a queued message (AC-001.6c). It
// duplicates that function's guard/reserve tail instead of adding a splice
// parameter to the shared drain chain, which has 12 production call sites
// that are not step entries at all (AC-005.6 forbids changing their
// behavior). The handoff is claimed only once a message is confirmed about
// to be dispatched, and is spliced onto a LOCAL COPY of that message's
// content — never onto the stored queue entry — so a skipped or paused
// reservation cannot lose or double-append it on a later drain.
func (s *Service) drainQueuedMessageForPromptableSessionWithHandoff(
	ctx context.Context, taskID, sessionID, stepID string, once *stepHandoffOnce,
) bool {
	lock, release := s.acquireCancelInFlightGuard(sessionID)
	defer release()
	lock.Lock()
	defer lock.Unlock()
	if s.isCancelInFlight(sessionID) {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		s.logger.Warn("failed to reload session before handoff-aware drain",
			zap.String("session_id", sessionID), zap.Error(err))
		return false
	}
	if err := s.checkSessionPromptable(session.TaskID, sessionID, session.State); err != nil {
		return false
	}
	if s.messageQueue == nil || s.isQueuedDispatchInFlight(sessionID) || s.isSteerInFlight(sessionID) {
		return false
	}
	queuedMsg, ok, autoRun := s.messageQueue.ReserveQueuedWithAutoRun(ctx, sessionID)
	if !autoRun || !ok || queuedMsg == nil {
		return false
	}
	if queuedMsg.Content != "" || len(queuedMsg.Attachments) > 0 {
		if handoffText := s.resolveStepHandoffText(ctx, once, taskID, stepID, true); handoffText != "" {
			spliced := *queuedMsg
			spliced.Content = appendStepHandoffToPrompt(queuedMsg.Content, handoffText)
			queuedMsg = &spliced
		}
	}
	return s.dispatchTakenQueuedMessage(ctx, sessionID, queuedMsg, ok)
}

// stepHandoffPromptHeading is the fixed heading the claimed handoff text is
// appended under, last in the composed prompt. Not localized: the same class
// as workflowInstructionsHeading, sent to the model rather than rendered to a
// user.
const stepHandoffPromptHeading = "## Context from the previous workflow step"

// recordStepHandoffCarryToken writes or removes the completion-handoff carry
// token for the step a consuming transition is entering. Call this under the
// exact same gate the caller uses to clear the completion-signal bag (both
// consuming-transition sites already have one) so a non-consuming transition
// never touches the token. A nil signal, or one with a blank handoff, removes
// any existing token instead of writing one. Best-effort: never blocks or
// fails the transition it rides on.
func (s *Service) recordStepHandoffCarryToken(ctx context.Context, taskID, nextStepID string, signal *models.PendingStepCompletionSignal) {
	if taskID == "" || nextStepID == "" || s.repo == nil {
		return
	}
	var handoff string
	if signal != nil {
		handoff = strings.TrimSpace(signal.Handoff)
	}
	if handoff == "" {
		if _, err := s.repo.RemoveTaskMetadataKey(ctx, taskID, models.MetaKeyStepHandoffCarry); err != nil {
			s.logger.Debug("failed to clear step handoff carry token",
				zap.String("task_id", taskID), zap.Error(err))
		}
		return
	}
	token := models.StepHandoffCarryToken{
		Handoff: handoff,
		StepID:  nextStepID,
		Stamp:   uuid.NewString(),
	}
	if err := s.repo.SetTaskMetadataKey(ctx, taskID, models.MetaKeyStepHandoffCarry, token); err != nil {
		s.logger.Debug("failed to write step handoff carry token",
			zap.String("task_id", taskID), zap.String("step_id", nextStepID), zap.Error(err))
	}
}

// claimStepHandoffCarryText performs the advisory-read-then-claim for the
// step being entered, returning the claimed handoff text and whether a token
// was actually claimed. A repository lacking the capability, a missing or
// mismatched token, or a claim error all report claimed=false with no text,
// so a failed attempt never spends the step entry's one-claim budget.
func (s *Service) claimStepHandoffCarryText(ctx context.Context, taskID, stepID string) (string, bool) {
	if taskID == "" || stepID == "" || s.repo == nil {
		return "", false
	}
	taker, ok := s.repo.(taskMetadataCarryTaker)
	if !ok {
		return "", false
	}
	token, ok := s.advisoryStepHandoffCarryToken(ctx, taskID, stepID)
	if !ok {
		return "", false
	}
	claimedRaw, claimed, err := taker.TakeTaskMetadataKeyIfDestinationStep(
		ctx, taskID, models.MetaKeyStepHandoffCarry, stepID, token.Stamp,
	)
	if err != nil {
		s.logger.Debug("failed to claim step handoff carry token",
			zap.String("task_id", taskID), zap.String("step_id", stepID), zap.Error(err))
		return "", false
	}
	if !claimed {
		return "", false
	}
	var claimedToken models.StepHandoffCarryToken
	if err := json.Unmarshal(claimedRaw, &claimedToken); err != nil {
		return "", false
	}
	return strings.TrimSpace(claimedToken.Handoff), true
}

// advisoryStepHandoffCarryToken performs the non-claiming read used to decide
// whether a claim attempt targeting stepID is worth making. This read is not
// synchronized with a concurrent claim, so the caller still must perform the
// compare-and-swap via TakeTaskMetadataKeyIfDestinationStep rather than acting
// on this result directly.
func (s *Service) advisoryStepHandoffCarryToken(ctx context.Context, taskID, stepID string) (models.StepHandoffCarryToken, bool) {
	task, err := s.repo.GetTask(ctx, taskID)
	if err != nil || task == nil || task.Metadata == nil {
		return models.StepHandoffCarryToken{}, false
	}
	rawValue, present := task.Metadata[models.MetaKeyStepHandoffCarry]
	if !present {
		return models.StepHandoffCarryToken{}, false
	}
	tokenBytes, err := json.Marshal(rawValue)
	if err != nil {
		return models.StepHandoffCarryToken{}, false
	}
	var token models.StepHandoffCarryToken
	if err := json.Unmarshal(tokenBytes, &token); err != nil {
		return models.StepHandoffCarryToken{}, false
	}
	if token.StepID != stepID || strings.TrimSpace(token.Stamp) == "" {
		return models.StepHandoffCarryToken{}, false
	}
	return token, true
}

// appendStepHandoffToPrompt appends the claimed handoff text under the fixed
// heading, last in the composed prompt. Called after reference expansion has
// already run over the rest of the prompt, so the handoff text is never
// itself expanded.
func appendStepHandoffToPrompt(prompt, handoffText string) string {
	if handoffText == "" {
		return prompt
	}
	section := stepHandoffPromptHeading + "\n\n" + handoffText
	if strings.TrimSpace(prompt) == "" {
		return section
	}
	return prompt + "\n\n" + section
}

// stepHandoffOnce memoizes one step entry's handoff claim so a replacement
// launch (after a failed or terminalized first attempt within the same step
// entry) reuses the same text instead of re-claiming: the underlying DB claim
// only ever succeeds once, so a second raw attempt would silently find
// nothing. Only a SUCCESSFUL claim is memoized — a failure (no repository
// capability, no matching token, or a claim error) is not, so a later
// replacement launch in the same entry retries it.
type stepHandoffOnce struct {
	mu      sync.Mutex
	claimed bool
	text    string
}

func newStepHandoffOnce() *stepHandoffOnce {
	return &stepHandoffOnce{}
}

// resolveStepHandoffText returns the handoff text to append for this step
// entry, claiming it at most once. wouldSend reports whether the caller's own
// dispatch branch has already decided it will send the agent something over
// content that excludes the handoff text; when false, nothing is claimed, so
// a dispatch that never happens can never consume a handoff.
func (s *Service) resolveStepHandoffText(ctx context.Context, once *stepHandoffOnce, taskID, stepID string, wouldSend bool) string {
	if once == nil || !wouldSend {
		return ""
	}
	once.mu.Lock()
	defer once.mu.Unlock()
	if once.claimed {
		return once.text
	}
	text, claimed := s.claimStepHandoffCarryText(ctx, taskID, stepID)
	if claimed {
		once.claimed = true
		once.text = text
	}
	return text
}
