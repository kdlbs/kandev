package orchestrator

import (
	"context"
	"errors"
	"strings"
	"sync"

	dynamicruntime "github.com/kandev/kandev/internal/agent/runtime/dynamic"
	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
	"go.uber.org/zap"
)

// promptAttemptEvidence is deliberately process-local. It fences recovery to
// the concrete execution and prompt generation that produced the failure; the
// durable dynamic continuation package is stored with the route generation
// separately.
type promptAttemptEvidence struct {
	mu               sync.Mutex
	executionID      string
	promptGeneration uint64
	evidenceKnown    bool
	output           bool
	// providerDiagnosticCode records an ACP agent-message diagnostic which is
	// followed by the matching prompt RPC failure. It is not model output, so it
	// must not make an otherwise pre-result provider failure unsafe to route.
	providerDiagnosticCode routingerr.Code
	// providerDiagnosticText is the normalized text of the recorded diagnostic.
	// It is written and cleared together with providerDiagnosticCode: a code
	// match alone is not sufficient evidence that the terminal failure IS the
	// diagnostic, since prose narrating the same failure classifies identically.
	providerDiagnosticText string
	effect                 bool
	dynamic                bool
	streakResetInProgress  bool
	streakResetComplete    bool
	initiator              authn.Identity
	initiatorKnown         bool
}

type pendingDynamicStreakReset struct {
	mu                    sync.Mutex
	event                 watcher.AgentEventData
	attempt               *promptAttemptEvidence
	routeGeneration       int64
	logicalProfileID      string
	executionProfileID    string
	capturedRouteIdentity bool
	inProgress            bool
	version               uint64
}

type pendingDynamicStreakResetSnapshot struct {
	event                 watcher.AgentEventData
	attempt               *promptAttemptEvidence
	routeGeneration       int64
	logicalProfileID      string
	executionProfileID    string
	capturedRouteIdentity bool
	version               uint64
}

// normalizeDiagnosticText applies streams.SanitizeProviderMessage so a raw
// diagnostic chunk and the already-sanitized terminal failure message it
// precedes normalize to the same text when their content is otherwise
// identical. The terminal ProviderError.Message reaches this comparison
// already sanitized (URLs/identifiers/credentials stripped) by the adapter
// extractor that built it; applying the same transform here, rather than
// only a cosmetic whitespace/punctuation trim, keeps both sides of the
// containment check symmetric instead of leaving raw content on the
// diagnostic side that the terminal side has already redacted.
func normalizeDiagnosticText(s string) string {
	return streams.SanitizeProviderMessage(s)
}

func (s *Service) beginPromptAttempt(
	sessionID, executionID string,
	promptGeneration uint64,
	dynamic bool,
) {
	if sessionID == "" {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	s.dynamicAttemptEvidence.Store(sessionID, &promptAttemptEvidence{
		executionID:      executionID,
		promptGeneration: promptGeneration,
		evidenceKnown:    true,
		dynamic:          dynamic,
	})
	// A new prompt must publish its complete execution identity before it opens
	// a retired retry lifecycle to provider events. Initial launches have no
	// execution yet; bindPromptAttempt clears the fence after launch acceptance.
	if executionID != "" {
		s.clearTransientRetryNoticeFenceLocked(sessionID, state)
	}
	state.mu.Unlock()
	release()
}

func (s *Service) beginDynamicAttempt(sessionID string) {
	s.beginPromptAttempt(sessionID, "", 1, true)
}

func (s *Service) beginInteractivePromptAttempt(
	ctx context.Context,
	sessionID, executionID string,
	dynamic bool,
) {
	s.flushPendingDynamicStreakReset(ctx, sessionID, nil)
	generation := s.nextPromptGeneration(ctx, sessionID)
	s.beginPromptAttempt(sessionID, executionID, generation, dynamic)
	s.capturePromptAttemptInitiator(ctx, sessionID, executionID, generation)
}

func (s *Service) beginInitialPromptAttempt(ctx context.Context, sessionID string, dynamic bool) {
	s.flushPendingDynamicStreakReset(ctx, sessionID, nil)
	s.beginPromptAttempt(sessionID, "", 1, dynamic)
	s.capturePromptAttemptInitiator(ctx, sessionID, "", 1)
}

func (s *Service) bindPromptAttemptToExecution(ctx context.Context, sessionID, executionID string) {
	generation := s.promptGenerationForSession(ctx, sessionID)
	s.bindPromptAttempt(sessionID, executionID, generation)
	s.capturePromptAttemptInitiator(ctx, sessionID, executionID, generation)
}

func (s *Service) capturePromptAttemptInitiator(
	ctx context.Context,
	sessionID, executionID string,
	promptGeneration uint64,
) {
	initiator, ok := authn.IdentityFromContext(ctx)
	if !ok || sessionID == "" {
		return
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if !evidence.evidenceKnown || evidence.dynamic ||
		(executionID != "" && evidence.executionID != "" && evidence.executionID != executionID) ||
		(promptGeneration != 0 && evidence.promptGeneration != 0 && evidence.promptGeneration != promptGeneration) {
		return
	}
	if evidence.initiatorKnown && evidence.initiator != initiator {
		evidence.evidenceKnown = false
		return
	}
	evidence.initiator = initiator
	evidence.initiatorKnown = true
}

func (s *Service) promptAttemptInitiator(data watcher.AgentEventData) (authn.Identity, bool) {
	evidence, ok := s.promptAttemptForSession(data.SessionID)
	if !ok {
		return authn.Identity{}, false
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if !evidence.evidenceKnown || evidence.dynamic || !evidence.initiatorKnown ||
		evidence.executionID != data.AgentExecutionID || evidence.promptGeneration != data.PromptGeneration {
		return authn.Identity{}, false
	}
	return evidence.initiator, true
}

func (s *Service) nextPromptGeneration(ctx context.Context, sessionID string) uint64 {
	generation := s.promptGenerationForSession(ctx, sessionID)
	if generation == ^uint64(0) {
		return 0
	}
	return generation + 1
}

func (s *Service) promptGenerationForSession(ctx context.Context, sessionID string) uint64 {
	if s.agentManager == nil || sessionID == "" {
		return 0
	}
	reader, ok := s.agentManager.(interface {
		GetPromptGenerationForSession(context.Context, string) (uint64, error)
	})
	if !ok {
		return 0
	}
	generation, err := reader.GetPromptGenerationForSession(ctx, sessionID)
	if err != nil {
		return 0
	}
	return generation
}

func (s *Service) isDynamicPromptSession(session *models.TaskSession) bool {
	return s.profileExecutionResolver != nil && session != nil &&
		session.RouteGeneration > 0 && session.ExecutionProfileID != ""
}

func (s *Service) bindPromptAttempt(sessionID, executionID string, promptGeneration uint64) {
	if sessionID == "" || (executionID == "" && promptGeneration == 0) {
		return
	}
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	state.mu.Lock()
	defer func() {
		state.mu.Unlock()
		release()
	}()
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if executionID != "" {
		if evidence.executionID != "" && evidence.executionID != executionID {
			evidence.evidenceKnown = false
			return
		}
		evidence.executionID = executionID
	}
	if promptGeneration != 0 {
		if evidence.promptGeneration != 0 && evidence.promptGeneration != promptGeneration {
			evidence.evidenceKnown = false
			return
		}
		evidence.promptGeneration = promptGeneration
	}
	if executionID != "" {
		// The evidence lock is released only after the identity is complete, while
		// state.mu still excludes late failure handling from reopening the fence.
		s.clearTransientRetryNoticeFenceLocked(sessionID, state)
	}
}

func (s *Service) bindDynamicAttemptExecution(sessionID, executionID string) {
	s.bindPromptAttempt(sessionID, executionID, 0)
}

func (s *Service) observePromptAttempt(
	sessionID, executionID string,
	promptGeneration uint64,
	output, effect bool,
) {
	if sessionID == "" || (!output && !effect) {
		return
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if !evidence.promptIdentityMatchesLocked(executionID, promptGeneration) {
		return
	}
	evidence.output = evidence.output || output
	evidence.effect = evidence.effect || effect
	if output {
		// Any ordinary output makes the turn unsafe to replay. A provider
		// diagnostic is recorded only by observeProviderDiagnostic below.
		evidence.providerDiagnosticCode = ""
		evidence.providerDiagnosticText = ""
	}
}

func (s *Service) observeProviderDiagnostic(
	sessionID, executionID string,
	promptGeneration uint64,
	providerID string,
	message string,
) {
	classified := routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhasePromptSend,
		ProviderID: providerID,
		Stderr:     message,
	})
	if classified.Confidence != routingerr.ConfHigh || !classified.FallbackAllowed {
		s.observePromptAttempt(sessionID, executionID, promptGeneration, true, false)
		return
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if !evidence.promptIdentityMatchesLocked(executionID, promptGeneration) {
		return
	}
	if evidence.output || evidence.effect {
		return
	}
	if evidence.providerDiagnosticCode != "" || evidence.providerDiagnosticText != "" {
		return
	}
	evidence.output = true
	evidence.providerDiagnosticCode = classified.Code
	evidence.providerDiagnosticText = normalizeDiagnosticText(message)
}

func (s *Service) observeDynamicAttempt(sessionID, executionID string, output, effect bool) {
	s.observePromptAttempt(sessionID, executionID, 0, output, effect)
}

func validDynamicStreakResetEvent(data watcher.AgentEventData) bool {
	return data.OwnerKind == queueStatusScopeTask && data.TaskID != "" && data.SessionID != "" &&
		data.AgentExecutionID != "" && data.PromptGeneration != 0
}

func (s *Service) markDynamicStreakResetPending(data watcher.AgentEventData) {
	if !validDynamicStreakResetEvent(data) {
		return
	}
	attempt, ok := s.promptAttemptForSession(data.SessionID)
	if !ok || !promptAttemptHasDynamicResetEvidence(attempt, data) {
		return
	}
	candidate := &pendingDynamicStreakReset{event: data, attempt: attempt, version: 1}
	value, loaded := s.pendingDynamicStreakResets.LoadOrStore(data.SessionID, candidate)
	if !loaded {
		return
	}
	pending, ok := value.(*pendingDynamicStreakReset)
	if ok {
		updatePendingDynamicStreakReset(pending, data, attempt)
	}
}

func promptAttemptHasDynamicResetEvidence(attempt *promptAttemptEvidence, data watcher.AgentEventData) bool {
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	return attempt.dynamic && attempt.evidenceKnown &&
		(attempt.outputObservedLocked(data) || attempt.effect) &&
		attempt.promptIdentityMatchesForClearLocked(data.AgentExecutionID, data.PromptGeneration) &&
		!attempt.streakResetComplete
}

func updatePendingDynamicStreakReset(
	pending *pendingDynamicStreakReset,
	data watcher.AgentEventData,
	attempt *promptAttemptEvidence,
) {
	pending.mu.Lock()
	defer pending.mu.Unlock()
	if pending.event.AgentExecutionID == data.AgentExecutionID &&
		pending.event.PromptGeneration == data.PromptGeneration && pending.attempt == attempt {
		return
	}
	pending.event = data
	pending.attempt = attempt
	pending.routeGeneration = 0
	pending.logicalProfileID = ""
	pending.executionProfileID = ""
	pending.capturedRouteIdentity = false
	pending.version++
}

func (s *Service) flushPendingDynamicStreakReset(
	ctx context.Context,
	sessionID string,
	currentBoundary *watcher.AgentEventData,
) bool {
	if sessionID == "" || s.repo == nil || s.profileExecutionResolver == nil {
		return false
	}
	if !s.canFlushPendingDynamicResetAtBoundary(sessionID, currentBoundary) {
		return false
	}
	value, ok := s.pendingDynamicStreakResets.Load(sessionID)
	if !ok {
		return true
	}
	pending, ok := value.(*pendingDynamicStreakReset)
	if !ok {
		return false
	}
	snapshot, ok := claimPendingDynamicStreakReset(pending)
	if !ok {
		return false
	}
	return s.flushClaimedPendingDynamicStreakReset(ctx, sessionID, currentBoundary, pending, snapshot)
}

func (s *Service) canFlushPendingDynamicResetAtBoundary(
	sessionID string,
	boundary *watcher.AgentEventData,
) bool {
	if boundary == nil {
		return true
	}
	if !validDynamicStreakResetEvent(*boundary) || boundary.SessionID != sessionID {
		return false
	}
	attempt, found := s.promptAttemptForSession(sessionID)
	if !found {
		return false
	}
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	return attempt.promptIdentityMatchesForClearLocked(
		boundary.AgentExecutionID, boundary.PromptGeneration,
	) && attempt.evidenceKnown
}

func (s *Service) flushClaimedPendingDynamicStreakReset(
	ctx context.Context,
	sessionID string,
	boundary *watcher.AgentEventData,
	pending *pendingDynamicStreakReset,
	snapshot pendingDynamicStreakResetSnapshot,
) bool {
	if boundary != nil && boundary.TaskID != snapshot.event.TaskID {
		finishPendingDynamicStreakReset(pending, snapshot, false)
		return false
	}
	if snapshot.attempt != nil {
		snapshot.attempt.mu.Lock()
		snapshot.attempt.streakResetInProgress = true
		snapshot.attempt.mu.Unlock()
	}
	complete, err := s.persistPendingDynamicStreakReset(
		ctx, sessionID, pending, snapshot,
	)
	finishPendingDynamicStreakReset(pending, snapshot, complete)
	if complete {
		pending.mu.Lock()
		if pending.version == snapshot.version {
			s.pendingDynamicStreakResets.CompareAndDelete(sessionID, pending)
		}
		pending.mu.Unlock()
	}
	return complete && err == nil
}

func claimPendingDynamicStreakReset(
	pending *pendingDynamicStreakReset,
) (pendingDynamicStreakResetSnapshot, bool) {
	pending.mu.Lock()
	if pending.inProgress {
		pending.mu.Unlock()
		return pendingDynamicStreakResetSnapshot{}, false
	}
	pending.inProgress = true
	snapshot := pendingDynamicStreakResetSnapshot{
		event: pending.event, attempt: pending.attempt, routeGeneration: pending.routeGeneration,
		logicalProfileID: pending.logicalProfileID, executionProfileID: pending.executionProfileID,
		capturedRouteIdentity: pending.capturedRouteIdentity, version: pending.version,
	}
	pending.mu.Unlock()
	return snapshot, true
}

func (s *Service) persistPendingDynamicStreakReset(
	ctx context.Context,
	sessionID string,
	pending *pendingDynamicStreakReset,
	snapshot pendingDynamicStreakResetSnapshot,
) (bool, error) {
	session, stale, err := s.capturePendingDynamicResetRoute(ctx, sessionID, pending, snapshot)
	if stale || err != nil {
		return stale, err
	}
	task, err := s.repo.GetTask(ctx, snapshot.event.TaskID)
	if err != nil {
		return false, err
	}
	if task == nil || task.ID != snapshot.event.TaskID || task.IsFromOffice {
		return true, nil
	}
	if !s.clearUnclassifiedStreak(ctx, session, false, "current activity") {
		return false, errors.New("could not persist dynamic unclassified streak reset")
	}
	return true, nil
}

func (s *Service) capturePendingDynamicResetRoute(
	ctx context.Context,
	sessionID string,
	pending *pendingDynamicStreakReset,
	snapshot pendingDynamicStreakResetSnapshot,
) (*models.TaskSession, bool, error) {
	if !snapshot.capturedRouteIdentity && !s.pendingDynamicResetSourceIsCurrent(sessionID, snapshot) {
		return nil, false, errors.New("pending streak reset no longer owns the current prompt")
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil {
		return nil, false, err
	}
	if !pendingDynamicResetSessionMatches(session, sessionID, snapshot.event) {
		return pendingDynamicResetStaleResult(snapshot)
	}
	if !hasPendingDynamicResetRouteIdentity(session) {
		return pendingDynamicResetStaleResult(snapshot)
	}
	if snapshot.capturedRouteIdentity {
		if !pendingDynamicResetRouteMatches(session, snapshot) {
			return nil, true, nil
		}
		return session, false, nil
	}
	if session.AgentExecutionID != snapshot.event.AgentExecutionID {
		return nil, false, errors.New("pending streak reset execution identity changed before capture")
	}
	if !capturePendingDynamicResetIdentity(pending, snapshot, session) {
		return nil, false, errors.New("pending streak reset identity changed during flush")
	}
	return session, false, nil
}

func pendingDynamicResetStaleResult(
	snapshot pendingDynamicStreakResetSnapshot,
) (*models.TaskSession, bool, error) {
	if snapshot.capturedRouteIdentity {
		return nil, true, nil
	}
	return nil, false, errors.New("pending streak reset route identity is unavailable")
}

func pendingDynamicResetSessionMatches(
	session *models.TaskSession,
	sessionID string,
	event watcher.AgentEventData,
) bool {
	return session != nil && session.ID == sessionID && session.TaskID == event.TaskID
}

func hasPendingDynamicResetRouteIdentity(session *models.TaskSession) bool {
	return session.RouteGeneration > 0 && session.AgentProfileID != "" && session.ExecutionProfileID != ""
}

func pendingDynamicResetRouteMatches(
	session *models.TaskSession,
	snapshot pendingDynamicStreakResetSnapshot,
) bool {
	return session.RouteGeneration == snapshot.routeGeneration &&
		session.AgentProfileID == snapshot.logicalProfileID &&
		session.ExecutionProfileID == snapshot.executionProfileID
}

func capturePendingDynamicResetIdentity(
	pending *pendingDynamicStreakReset,
	snapshot pendingDynamicStreakResetSnapshot,
	session *models.TaskSession,
) bool {
	pending.mu.Lock()
	defer pending.mu.Unlock()
	if pending.version != snapshot.version {
		return false
	}
	pending.routeGeneration = session.RouteGeneration
	pending.logicalProfileID = session.AgentProfileID
	pending.executionProfileID = session.ExecutionProfileID
	pending.capturedRouteIdentity = true
	return true
}

func (s *Service) pendingDynamicResetSourceIsCurrent(
	sessionID string,
	snapshot pendingDynamicStreakResetSnapshot,
) bool {
	current, currentExists := s.promptAttemptForSession(sessionID)
	owner, ownerSupported := s.agentManager.(interface {
		OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
	})
	managerOwnsSource := ownerSupported && owner.OwnsPromptGeneration(
		sessionID, snapshot.event.AgentExecutionID, snapshot.event.PromptGeneration,
	)
	if currentExists && current == snapshot.attempt && s.currentDynamicPromptAttempt(
		sessionID, snapshot.event.AgentExecutionID, snapshot.event.PromptGeneration,
	) {
		return !ownerSupported || managerOwnsSource
	}
	return false
}

func finishPendingDynamicStreakReset(
	pending *pendingDynamicStreakReset,
	snapshot pendingDynamicStreakResetSnapshot,
	complete bool,
) {
	pending.mu.Lock()
	pending.inProgress = false
	if snapshot.attempt != nil {
		snapshot.attempt.mu.Lock()
		snapshot.attempt.streakResetInProgress = false
		if complete && pending.version == snapshot.version {
			snapshot.attempt.streakResetComplete = true
		}
		snapshot.attempt.mu.Unlock()
	}
	pending.mu.Unlock()
}

func (s *Service) withPromptAttemptEvidence(data watcher.AgentEventData) watcher.AgentEventData {
	if data.SessionID == "" {
		return data
	}
	state, release := s.acquireTransientRetryNoticeState(data.SessionID)
	state.mu.Lock()
	defer func() {
		state.mu.Unlock()
		release()
	}()
	if state.retired.Load() {
		data.EvidenceKnown = false
		data.OutputObserved = false
		data.EffectObserved = false
		return data
	}
	return s.withPromptAttemptEvidenceLocked(data)
}

func (s *Service) withPromptAttemptEvidenceLocked(data watcher.AgentEventData) watcher.AgentEventData {
	if data.SessionID == "" {
		return data
	}
	// Lifecycle captures terminal failure evidence before publishing the
	// failure event. That snapshot is authoritative when stream and lifecycle
	// events travel through separate bus subscriptions; retain it while still
	// using the process-local record to fence the session, execution, and
	// generation identity.
	lifecycleEvidenceKnown := data.EvidenceKnown
	lifecycleOutputObserved := data.OutputObserved
	lifecycleEffectObserved := data.EffectObserved
	lifecycleDiagnosticCandidate := data.ProviderDiagnosticCandidate
	lifecycleDiagnosticText := data.ProviderDiagnosticText
	evidence, ok := s.promptAttemptForSession(data.SessionID)
	if !ok {
		// Lifecycle evidence is only authoritative after the process-local
		// attempt record fences the session, execution, and generation. Without
		// that record, a terminal snapshot could authorize a replacement for an
		// unrelated or already-cleared attempt.
		data.EvidenceKnown = false
		data.OutputObserved = false
		data.EffectObserved = false
		return data
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	if !evidence.promptIdentityMatchesLocked(data.AgentExecutionID, data.PromptGeneration) {
		data.EvidenceKnown = false
		data.OutputObserved = false
		data.EffectObserved = false
		if evidence.dynamic {
			data.DynamicRouteAttempt = true
		}
		return data
	}
	if evidence.dynamic {
		data.DynamicRouteAttempt = true
	}
	if lifecycleEvidenceKnown && lifecycleDiagnosticCandidate && !evidence.output && !evidence.effect {
		s.observeLifecycleProviderDiagnosticLocked(evidence, data.AgentID, lifecycleDiagnosticText)
	}
	outputObserved := evidence.outputObservedLocked(data)
	if lifecycleEvidenceKnown {
		data.EvidenceKnown = true
		data.OutputObserved = lifecycleOutputObserved || outputObserved
		data.EffectObserved = lifecycleEffectObserved || evidence.effect
	} else {
		data.EvidenceKnown = evidence.evidenceKnown
		data.OutputObserved = outputObserved
		data.EffectObserved = evidence.effect
	}
	return data
}

// observeLifecycleProviderDiagnosticLocked imports the bounded diagnostic
// captured by lifecycle into the process-local evidence record. The stream
// event may arrive after the terminal failure because those events use
// separate subscriptions, so an absent or unclassifiable diagnostic fails
// closed as ordinary output.
func (s *Service) observeLifecycleProviderDiagnosticLocked(evidence *promptAttemptEvidence, providerID, message string) {
	message = normalizeDiagnosticText(message)
	if message == "" {
		evidence.output = true
		evidence.providerDiagnosticCode = ""
		evidence.providerDiagnosticText = ""
		return
	}
	classified := routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhasePromptSend,
		ProviderID: providerID,
		Stderr:     message,
	})
	if classified.Confidence != routingerr.ConfHigh || !classified.FallbackAllowed {
		evidence.output = true
		evidence.providerDiagnosticCode = ""
		evidence.providerDiagnosticText = ""
		return
	}
	evidence.output = true
	evidence.providerDiagnosticCode = classified.Code
	evidence.providerDiagnosticText = message
}

// outputObservedLocked reports whether evidence.output should be treated as
// generated model output for data's terminal failure. A recorded provider
// diagnostic clears the fence only when its code matches AND its normalized
// text is contained in the terminal failure's normalized message: a matching
// classification code alone is not enough, since assistant prose narrating a
// failure can classify identically without being the transport diagnostic
// itself. Callers must hold e.mu.
func (e *promptAttemptEvidence) outputObservedLocked(data watcher.AgentEventData) bool {
	if e.providerDiagnosticCode != "" && e.providerDiagnosticText != "" &&
		matchingProviderFailureCode(data) == e.providerDiagnosticCode &&
		strings.Contains(normalizeDiagnosticText(matchingProviderFailureMessage(data)), e.providerDiagnosticText) {
		// Claude ACP emits a human-readable agent_message_chunk immediately
		// before returning the same provider error from session/prompt. The
		// chunk is diagnostic transport, not generated output.
		return false
	}
	return e.output
}

func matchingProviderFailureMessage(data watcher.AgentEventData) string {
	message := data.ErrorMessage
	if data.ProviderError != nil && data.ProviderError.Message != "" {
		message = data.ProviderError.Message
	}
	return message
}

func matchingProviderFailureCode(data watcher.AgentEventData) routingerr.Code {
	message := matchingProviderFailureMessage(data)
	if message == "" {
		return ""
	}
	return routingerr.Classify(routingerr.Input{
		Phase:      routingerr.PhasePromptSend,
		ProviderID: data.AgentID,
		Stderr:     message,
	}).Code
}

func (s *Service) withDynamicAttemptEvidence(data watcher.AgentEventData) watcher.AgentEventData {
	data.DynamicRouteAttempt = true
	return s.withPromptAttemptEvidence(data)
}

func (s *Service) promptAttemptPreResultSafe(data watcher.AgentEventData) bool {
	if data.SessionID == "" || data.DynamicRouteAttempt ||
		data.AgentExecutionID == "" || data.PromptGeneration == 0 ||
		!data.EvidenceKnown || data.OutputObserved || data.EffectObserved {
		return false
	}
	evidence, ok := s.promptAttemptForSession(data.SessionID)
	if !ok {
		return false
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	return !evidence.dynamic && evidence.evidenceKnown &&
		evidence.executionID == data.AgentExecutionID &&
		evidence.promptGeneration == data.PromptGeneration &&
		!evidence.outputObservedLocked(data) && !evidence.effect
}

func (s *Service) clearPromptAttemptEvidence(sessionID, executionID string, promptGeneration uint64) {
	if sessionID == "" {
		return
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return
	}
	evidence.mu.Lock()
	matches := evidence.promptIdentityMatchesForClearLocked(executionID, promptGeneration)
	evidence.mu.Unlock()
	if matches {
		s.dynamicAttemptEvidence.CompareAndDelete(sessionID, evidence)
	}
}

func (s *Service) promptAttemptForSession(sessionID string) (*promptAttemptEvidence, bool) {
	v, ok := s.dynamicAttemptEvidence.Load(sessionID)
	if !ok {
		return nil, false
	}
	evidence, ok := v.(*promptAttemptEvidence)
	return evidence, ok
}

func (e *promptAttemptEvidence) promptIdentityMatchesLocked(executionID string, promptGeneration uint64) bool {
	if e.executionID != "" {
		if executionID == "" {
			e.evidenceKnown = false
			return false
		}
		if e.executionID != executionID {
			// A concrete event from another execution is stale. Leave the current
			// attempt intact so that the stale event cannot poison its evidence.
			return false
		}
	} else if executionID != "" {
		e.executionID = executionID
	}
	if e.promptGeneration != 0 {
		if promptGeneration == 0 {
			e.evidenceKnown = false
			return false
		}
		if e.promptGeneration != promptGeneration {
			// As with execution IDs, a concrete older generation is a delayed
			// event and must not invalidate the current prompt's evidence.
			return false
		}
	} else if promptGeneration != 0 {
		e.promptGeneration = promptGeneration
	}
	return true
}

func (e *promptAttemptEvidence) promptIdentityMatchesForClearLocked(executionID string, promptGeneration uint64) bool {
	if e.executionID != "" && (executionID == "" || e.executionID != executionID) {
		return false
	}
	if e.promptGeneration != 0 && (promptGeneration == 0 || e.promptGeneration != promptGeneration) {
		return false
	}
	return true
}

func dynamicPreResultSafe(data watcher.AgentEventData) bool {
	return data.DynamicRouteAttempt && data.EvidenceKnown && !data.OutputObserved && !data.EffectObserved
}

func (s *Service) clearDynamicUnclassifiedStreakForEvent(
	ctx context.Context,
	data watcher.AgentEventData,
	requireLocalEvidence bool,
) bool {
	if s.repo == nil || s.profileExecutionResolver == nil || !s.currentUnclassifiedStreakEvent(data, requireLocalEvidence) {
		return false
	}
	attempt, ok := s.claimUnclassifiedStreakReset(data, requireLocalEvidence)
	if !ok {
		return false
	}
	resetComplete := false
	defer func() {
		if attempt != nil {
			attempt.mu.Lock()
			attempt.streakResetInProgress = false
			attempt.streakResetComplete = resetComplete
			attempt.mu.Unlock()
		}
	}()
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || !validUnclassifiedStreakEventSession(session, data) {
		return false
	}
	if !s.taskAllowsUnclassifiedStreakClear(ctx, data.TaskID) {
		return false
	}
	resetComplete = s.clearUnclassifiedStreak(ctx, session, false, "current activity")
	return resetComplete
}

func (s *Service) clearDynamicUnclassifiedStreakForCompletion(
	ctx context.Context,
	data watcher.AgentEventData,
) bool {
	if !s.clearDynamicUnclassifiedStreakForEvent(ctx, data, false) {
		return false
	}
	s.deletePendingDynamicStreakReset(data.SessionID)
	return true
}

func (s *Service) claimUnclassifiedStreakReset(
	data watcher.AgentEventData,
	requireLocalEvidence bool,
) (*promptAttemptEvidence, bool) {
	attempt, ok := s.promptAttemptForSession(data.SessionID)
	if !ok {
		return nil, !requireLocalEvidence
	}
	attempt.mu.Lock()
	defer attempt.mu.Unlock()
	if !attempt.promptIdentityMatchesForClearLocked(data.AgentExecutionID, data.PromptGeneration) ||
		requireLocalEvidence && (!attempt.dynamic || !attempt.evidenceKnown) ||
		attempt.streakResetInProgress || attempt.streakResetComplete {
		return nil, false
	}
	attempt.streakResetInProgress = true
	return attempt, true
}

func (s *Service) currentUnclassifiedStreakEvent(data watcher.AgentEventData, requireLocalEvidence bool) bool {
	if data.OwnerKind != queueStatusScopeTask || data.TaskID == "" || data.SessionID == "" ||
		data.AgentExecutionID == "" || data.PromptGeneration == 0 {
		return false
	}
	if requireLocalEvidence {
		return s.currentDynamicPromptAttempt(data.SessionID, data.AgentExecutionID, data.PromptGeneration)
	}
	generationOwner, ok := s.agentManager.(interface {
		OwnsPromptGeneration(sessionID, executionID string, generation uint64) bool
	})
	return ok && generationOwner.OwnsPromptGeneration(data.SessionID, data.AgentExecutionID, data.PromptGeneration)
}

func validUnclassifiedStreakEventSession(session *models.TaskSession, data watcher.AgentEventData) bool {
	return session != nil && session.TaskID == data.TaskID && session.AgentExecutionID == data.AgentExecutionID &&
		session.RouteGeneration > 0 && session.ExecutionProfileID != ""
}

func (s *Service) taskAllowsUnclassifiedStreakClear(ctx context.Context, taskID string) bool {
	task, err := s.repo.GetTask(ctx, taskID)
	return err == nil && task != nil && task.ID == taskID && !task.IsFromOffice
}

func (s *Service) clearDynamicUnclassifiedStreakForStop(ctx context.Context, sessionID string) {
	if s.repo == nil || s.profileExecutionResolver == nil || sessionID == "" {
		return
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || session.TaskID == "" || session.AgentProfileID == "" ||
		session.ExecutionProfileID == "" || session.RouteGeneration <= 0 {
		return
	}
	if !s.taskAllowsUnclassifiedStreakClear(ctx, session.TaskID) {
		return
	}
	if s.clearUnclassifiedStreak(ctx, session, false, "stop") {
		s.deletePendingDynamicStreakReset(sessionID)
	}
}

func (s *Service) deletePendingDynamicStreakReset(sessionID string) {
	value, ok := s.pendingDynamicStreakResets.Load(sessionID)
	if !ok {
		return
	}
	pending, ok := value.(*pendingDynamicStreakReset)
	if !ok {
		return
	}
	pending.mu.Lock()
	deleted := s.pendingDynamicStreakResets.CompareAndDelete(sessionID, pending)
	attempt := pending.attempt
	pending.mu.Unlock()
	if deleted && attempt != nil {
		attempt.mu.Lock()
		attempt.streakResetComplete = true
		attempt.mu.Unlock()
	}
}

func (s *Service) clearDynamicStartupStreakForBootReady(
	ctx context.Context,
	data watcher.AgentEventData,
	session *models.TaskSession,
) {
	if s.repo == nil || s.profileExecutionResolver == nil || session == nil || data.OwnerKind != queueStatusScopeTask ||
		data.TaskID == "" || data.TaskID != session.TaskID || data.AgentExecutionID == "" ||
		data.AgentExecutionID != session.AgentExecutionID || session.RouteGeneration <= 0 ||
		session.ExecutionProfileID == "" {
		return
	}
	if !s.taskAllowsUnclassifiedStreakClear(ctx, data.TaskID) {
		return
	}
	s.clearUnclassifiedStreak(ctx, session, true, "boot-ready")
}

func (s *Service) clearUnclassifiedStreak(
	ctx context.Context,
	session *models.TaskSession,
	startup bool,
	reason string,
) bool {
	var err error
	if startup {
		err = s.profileExecutionResolver.ClearUnclassifiedStartupStreak(
			ctx, session.ID, session.RouteGeneration, session.ExecutionProfileID,
		)
	} else {
		err = s.profileExecutionResolver.ClearUnclassifiedStreak(
			ctx, session.ID, session.RouteGeneration, session.ExecutionProfileID,
		)
	}
	if errors.Is(err, dynamicruntime.ErrStaleGeneration) || errors.Is(err, dynamicruntime.ErrRouteStateNotFound) {
		return true
	}
	if err != nil {
		s.logger.Debug("could not clear dynamic unclassified streak after "+reason,
			zap.String("session_id", session.ID), zap.Error(err))
		return false
	}
	return true
}
