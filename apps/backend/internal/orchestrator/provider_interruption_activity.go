package orchestrator

import (
	"context"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
)

const (
	metaKeyRecoveryActivityConfirmed = "recovery_activity_confirmed"
	continuationActivityResumedText  = "Agent resumed"
	// continuationActivityToolLimit bounds the foreground tool identities kept
	// as resumed-activity evidence for one accepted prompt.
	continuationActivityToolLimit = 64
)

// continuationActivityEvent is the bounded stream evidence this observer
// consumes. It deliberately avoids runtime implementation types so the
// orchestrator keeps the ARCH-RUNTIME-IMPORT boundary.
type continuationActivityEvent struct {
	Type                        string
	PromptGeneration            uint64
	Text                        string
	Role                        string
	ProviderDiagnosticCandidate bool
	ToolCallID                  string
	ParentToolCallID            string
	Normalized                  *streams.NormalizedPayload
}

// continuationActivity records bounded evidence that the current accepted
// continuation prompt resumed work. It never grants recovery ownership or
// dispatch authority.
type continuationActivity struct {
	executionID string
	generation  uint64
	output      bool
	tools       map[string]struct{}
	published   bool
}

func (a *continuationActivity) reset(executionID string, generation uint64) {
	a.executionID, a.generation = executionID, generation
	a.output = false
	a.tools = nil
}

func (a *continuationActivity) matches(executionID string, generation uint64) bool {
	return a.executionID != "" && a.executionID == executionID && a.generation == generation
}

func (a *continuationActivity) confirmed() bool {
	return a.output || len(a.tools) > 0
}

func (a *continuationActivity) record(executionID string, generation uint64, output bool, toolID string) {
	if !a.matches(executionID, generation) {
		a.reset(executionID, generation)
	}
	if output {
		a.output = true
	}
	if toolID == "" {
		return
	}
	if a.tools == nil {
		a.tools = make(map[string]struct{})
	}
	if len(a.tools) < continuationActivityToolLimit {
		a.tools[toolID] = struct{}{}
	}
}

func (a *continuationActivity) ownsTool(executionID string, generation uint64, toolID string) bool {
	if !a.matches(executionID, generation) {
		return false
	}
	_, ok := a.tools[toolID]
	return ok
}

// continuationActivityConfirmedLocked requires the evidence to belong to the
// exact accepted prompt. The caller must hold entry.mu.
func continuationActivityConfirmedLocked(entry *transientRetryEntry) bool {
	return entry.mode == recoveryModeContinue && entry.continuation != nil &&
		entry.acceptedExecution != "" && entry.acceptedGeneration != 0 &&
		entry.activity.matches(entry.acceptedExecution, entry.acceptedGeneration) &&
		entry.activity.confirmed()
}

func continuationActivityRecordable(entry *transientRetryEntry) bool {
	return entry != nil && entry.mode == recoveryModeContinue && entry.continuation != nil
}

// observeContinuationActivity captures activity from the accepted continuation
// prompt. It runs after handleAgentStreamEvent's execution and cancellation
// guards, so stale executions never reach this boundary.
func (s *Service) observeContinuationActivity(
	ctx context.Context,
	taskID, sessionID, eventExecutionID string,
	event continuationActivityEvent,
) {
	entry, ok := s.continuationActivityTarget(sessionID, eventExecutionID, event)
	if !ok {
		return
	}
	output, toolID, ok := continuationActivityEvidence(event)
	if !ok || !s.recordContinuationActivityEvidence(entry, event, eventExecutionID, output, toolID) {
		return
	}
	if continuationActivityNeedsPublication(entry) {
		s.publishContinuationActivity(ctx, taskID, sessionID, entry)
	}
}

func (s *Service) continuationActivityTarget(
	sessionID, eventExecutionID string,
	event continuationActivityEvent,
) (*transientRetryEntry, bool) {
	if sessionID == "" || eventExecutionID == "" {
		return nil, false
	}
	if event.ParentToolCallID != "" || normalizedIsBackgroundTask(event.Normalized) {
		return nil, false
	}
	entry := s.currentContinuationActivityEntry(sessionID)
	if entry == nil {
		return nil, false
	}
	if !s.continuationPromptAttemptMatches(sessionID, eventExecutionID, event.PromptGeneration) {
		return nil, false
	}
	if event.Type == agentEventToolCall || event.Type == agentEventToolUpdate {
		if !s.foregroundContinuationTool(sessionID, event.ToolCallID, eventExecutionID) {
			return nil, false
		}
	}
	return entry, true
}

func (s *Service) recordContinuationActivityEvidence(
	entry *transientRetryEntry,
	event continuationActivityEvent,
	eventExecutionID string,
	output bool,
	toolID string,
) bool {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	if event.Type == agentEventToolUpdate {
		return entry.activity.ownsTool(eventExecutionID, event.PromptGeneration, toolID) &&
			continuationActivityConfirmedLocked(entry)
	}
	entry.activity.record(eventExecutionID, event.PromptGeneration, output, toolID)
	return continuationActivityConfirmedLocked(entry)
}

func continuationActivityNeedsPublication(entry *transientRetryEntry) bool {
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return continuationActivityConfirmedLocked(entry) && !entry.activity.published
}

func (s *Service) currentContinuationActivityEntry(sessionID string) *transientRetryEntry {
	value, ok := s.transientRetries.Load(sessionID)
	if !ok {
		return nil
	}
	entry, ok := value.(*transientRetryEntry)
	if !ok || !continuationActivityRecordable(entry) {
		return nil
	}
	return entry
}

func (s *Service) continuationPromptAttemptMatches(sessionID, executionID string, generation uint64) bool {
	if executionID == "" || generation == 0 {
		return false
	}
	evidence, ok := s.promptAttemptForSession(sessionID)
	if !ok {
		return false
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	return evidence.evidenceKnown && !evidence.dynamic &&
		evidence.executionID == executionID && evidence.promptGeneration == generation
}

func continuationActivityEvidence(event continuationActivityEvent) (bool, string, bool) {
	switch event.Type {
	case streams.EventTypeMessageChunk:
		if event.Role == "user" || event.ProviderDiagnosticCandidate {
			return false, "", false
		}
		return strings.TrimSpace(event.Text) != "", "", true
	case streams.EventTypeReasoning:
		return strings.TrimSpace(event.Text) != "", "", true
	case agentEventToolCall, agentEventToolUpdate:
		if event.ToolCallID == "" {
			return false, "", false
		}
		return false, event.ToolCallID, true
	default:
		return false, "", false
	}
}

func (s *Service) foregroundContinuationTool(sessionID, toolID, executionID string) bool {
	return toolID != "" &&
		s.toolOwnership(sessionID, toolID, executionID) == toolOwnershipForeground
}

// publishContinuationActivity serializes the neutral presentation with the
// existing notice lifecycle. The publication latch is set only after a
// successful write, so a later activity event can retry a failed update.
func (s *Service) publishContinuationActivity(
	ctx context.Context,
	taskID, sessionID string,
	entry *transientRetryEntry,
) {
	state, release := s.acquireTransientRetryNoticeState(sessionID)
	if state == nil {
		return
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	defer release()
	if ctx.Err() != nil || state.retired.Load() {
		return
	}
	entry.mu.Lock()
	confirmed := continuationActivityConfirmedLocked(entry)
	published := entry.activity.published
	entry.mu.Unlock()
	if !confirmed || published {
		return
	}
	if current, ok := s.transientRetries.Load(sessionID); !ok || current != entry {
		return
	}
	if err := s.createContinuationActivityStatusMessageLocked(state, ctx, taskID, sessionID, entry); err != nil {
		s.logger.Warn("failed to publish confirmed continuation activity",
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID),
			zap.Error(err))
		return
	}
	entry.mu.Lock()
	entry.activity.published = true
	entry.mu.Unlock()
}

func (s *Service) createContinuationActivityStatusMessageLocked(
	state *transientRetryNoticeState,
	ctx context.Context,
	taskID, sessionID string,
	entry *transientRetryEntry,
) error {
	if s.messageCreator == nil || state.retired.Load() {
		return nil
	}
	entry.mu.Lock()
	data := watcher.AgentEventData{
		TaskID: taskID, SessionID: sessionID,
		RecoveryMode: recoveryModeContinue, RecoveryPhase: "continuing",
		RecoveryAttemptsStarted: entry.started,
	}
	data.ProviderError = &streams.ProviderError{ProviderID: entry.providerID, ModelID: entry.modelID}
	failureCode, attempt, policy := entry.failureCode, entry.attempt, entry.policy
	entry.mu.Unlock()
	content, meta := continuationActivityStatusMessage(data, failureCode, attempt, policy)
	return s.writeTransientRetryStatusMessageLocked(ctx, data, content, meta)
}

func continuationActivityStatusMessage(
	data watcher.AgentEventData,
	failureCode routingerr.Code,
	attempt int,
	policy transientRetryPolicy,
) (string, map[string]interface{}) {
	if failureCode == "" {
		failureCode = routingerr.CodeAgentTransportLost
	}
	_, meta := transientRetryStatusMessage(data, &routingerr.Error{Code: failureCode}, attempt, 0, time.Time{})
	meta[metaKeyVariant] = metaVariantDefault
	meta[metaKeyRecoveryActivityConfirmed] = true
	meta["max_attempts"] = policy.maxAttempts()
	if !policy.windowEndsAt.IsZero() {
		meta["retry_window_ends_at"] = policy.windowEndsAt.UTC().Format(time.RFC3339Nano)
	}
	delete(meta, "retry_in_seconds")
	delete(meta, "retry_at")
	return continuationActivityResumedText, meta
}
