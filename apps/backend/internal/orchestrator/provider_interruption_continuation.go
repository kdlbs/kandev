package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

const recoveryModeContinue = "continue"
const recoveryModeReplay = "replay"
const recoveryDispositionManual = "manual"
const recoveryDispositionExhausted = "exhausted"
const failureKindProviderInterrupted = "provider_interrupted"
const continuationRefusalMissingEvidence = "missing_evidence"
const continuationRefusalUnsupportedRestore = "unsupported_restore"

type continuationPolicy string

const (
	continuationPolicySavedHistoryRestore     continuationPolicy = "saved_history_restore"
	continuationPolicyCapacityLive            continuationPolicy = "capacity_live"
	continuationPolicyAgentManagedPendingLive continuationPolicy = "agent_managed_pending_live"
)

// continuationBinding is an immutable admission snapshot, retained only in process.
type continuationBinding struct {
	policy         continuationPolicy
	nativeID       string
	identity       [32]byte
	workflowStepID string
	initiator      authn.Identity
	initiatorKnown bool
}

func (s *Service) continuationBindingForFailure(ctx context.Context, data watcher.AgentEventData) *continuationBinding {
	var previous *transientRetryEntry
	if value, ok := s.transientRetries.Load(data.SessionID); ok {
		previous, _ = value.(*transientRetryEntry)
	}
	return s.continuationBindingForFailureWithPrevious(ctx, data, previous)
}

func (s *Service) continuationBindingForFailureWithPrevious(
	ctx context.Context,
	data watcher.AgentEventData,
	previous *transientRetryEntry,
) *continuationBinding {
	policy := s.continuationPolicyForFailure(data, previous)
	if policy == "" {
		return nil
	}
	return s.continuationBindingForPolicy(ctx, data, policy)
}

func (s *Service) continuationBindingForPolicy(
	ctx context.Context,
	data watcher.AgentEventData,
	policy continuationPolicy,
) *continuationBinding {
	session, task := s.continuationSessionAndTaskForPolicy(ctx, data, policy)
	if session == nil || task == nil {
		return nil
	}
	nativeID := continuationNativeID(session)
	identity := continuationSessionIdentity(session)
	if nativeID == "" || identity == ([32]byte{}) {
		return nil
	}
	var initiator authn.Identity
	initiatorKnown := false
	if policy == continuationPolicyCapacityLive || policy == continuationPolicyAgentManagedPendingLive {
		initiator, initiatorKnown = s.promptAttemptInitiator(data)
		if !initiatorKnown && (s.sessionPromptCheck != nil || s.sessionAccessCheck != nil) {
			return nil
		}
	}
	return &continuationBinding{
		policy: policy, nativeID: nativeID, identity: identity, workflowStepID: task.WorkflowStepID,
		initiator: initiator, initiatorKnown: initiatorKnown,
	}
}

func (s *Service) continuationSessionAndTaskForPolicy(
	ctx context.Context,
	data watcher.AgentEventData,
	policy continuationPolicy,
) (*models.TaskSession, *models.Task) {
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil || session.TaskID != data.TaskID || session.IsPassthrough || session.AgentProfileID == "" {
		return nil, nil
	}
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil || task.IsFromOffice || task.ArchivedAt != nil ||
		(policy == continuationPolicyCapacityLive && models.IsAutomationTaskOrigin(task.Origin)) {
		return nil, nil
	}
	return session, task
}

func (s *Service) continuationPolicyForFailure(
	data watcher.AgentEventData,
	previous *transientRetryEntry,
) continuationPolicy {
	classified := classifyKanbanFailure(data)
	if classified == nil {
		return ""
	}
	switch classified.Code {
	case routingerr.CodeModelCapacity:
		if s.capacityContinuationFailureHasEvidence(data) {
			return continuationPolicyCapacityLive
		}
		return ""
	default:
		if routingerr.Decide(routingerr.ContextKanban, classified, time.Now().UTC()) != routingerr.DecisionShortRetry {
			return ""
		}
		if continuationPendingToolsV3(data.ContinuationSafety) || s.agentManagedPendingEpisodeMatches(previous, data) {
			if s.agentManagedPendingFailureHasEvidence(data, classified) {
				return continuationPolicyAgentManagedPendingLive
			}
			return ""
		}
		if s.continuationFailureHasEvidence(data) {
			return continuationPolicySavedHistoryRestore
		}
		return ""
	}
}

func (s *Service) continuationFailureHasEvidence(data watcher.AgentEventData) bool {
	return data.OwnerKind == queueStatusScopeTask && !data.DynamicRouteAttempt &&
		data.ContinuationSafety != nil && !data.ContinuationSafety.Pending &&
		data.ContinuationSafety.SafeFor(data.PromptGeneration) && data.EvidenceKnown && s.continuationPromptIdentityMatches(data)
}

func continuationPendingToolsV3(snapshot *streams.ContinuationSafetySnapshot) bool {
	return snapshot != nil && snapshot.Support == streams.ContinuationAgentManagedToolsV3 && snapshot.Pending
}

func (s *Service) agentManagedPendingEpisodeMatches(previous *transientRetryEntry, data watcher.AgentEventData) bool {
	if previous == nil || previous.mode != recoveryModeContinue ||
		previous.continuationPolicy != continuationPolicyAgentManagedPendingLive || previous.continuation == nil ||
		previous.retainedRuntime == nil {
		return false
	}
	previous.mu.Lock()
	defer previous.mu.Unlock()
	return previous.started > 0 && previous.acceptedExecution == data.AgentExecutionID &&
		previous.acceptedGeneration == data.PromptGeneration
}

func (s *Service) agentManagedPendingFailureHasEvidence(
	data watcher.AgentEventData,
	classified *routingerr.Error,
) bool {
	return classified != nil && classified.Code == routingerr.CodeAgentTransportLost &&
		data.PromptFailureDisposition == streams.PromptFailureDispositionRetainRuntime &&
		data.PromptFailureDisposition.Valid() && data.OwnerKind == queueStatusScopeTask &&
		!data.DynamicRouteAttempt && data.ContinuationSafety != nil &&
		data.ContinuationSafety.SafeFor(data.PromptGeneration) &&
		data.EvidenceKnown && s.continuationPromptIdentityMatches(data)
}

func (s *Service) capacityContinuationFailureHasEvidence(data watcher.AgentEventData) bool {
	classified := classifyKanbanFailure(data)
	return classified != nil && classified.Code == routingerr.CodeModelCapacity &&
		data.PromptFailureDisposition == streams.PromptFailureDispositionRetainRuntime &&
		data.PromptFailureDisposition.Valid() && data.OwnerKind == queueStatusScopeTask &&
		!data.DynamicRouteAttempt &&
		(data.CapacityContinuation.SafeFor(data.PromptGeneration) || s.capacityContinuationRetryHasEvidence(data)) &&
		data.EvidenceKnown && s.continuationPromptIdentityMatches(data)
}

// An admitted continuation may fail before doing new work. Keep its bound
// continuation instruction instead of replaying the original completed request.
func (s *Service) capacityContinuationRetryHasEvidence(data watcher.AgentEventData) bool {
	snapshot := data.CapacityContinuation
	if snapshot == nil || !snapshot.EvidenceComplete || snapshot.PromptGeneration != data.PromptGeneration ||
		!supportsCapacityContinuation(snapshot.Support) || capacityContinuationHasUnsafeWork(snapshot) ||
		!s.promptAttemptPreResultSafe(data) {
		return false
	}
	value, ok := s.transientRetries.Load(data.SessionID)
	if !ok {
		return false
	}
	entry, ok := value.(*transientRetryEntry)
	if !ok || entry.mode != recoveryModeContinue || entry.continuationPolicy != continuationPolicyCapacityLive ||
		entry.continuation == nil || entry.retainedRuntime == nil {
		return false
	}
	entry.mu.Lock()
	defer entry.mu.Unlock()
	return entry.started > 0 && entry.acceptedExecution == data.AgentExecutionID &&
		entry.acceptedGeneration == data.PromptGeneration
}

func (s *Service) continuationRefusalReason(ctx context.Context, data watcher.AgentEventData) string {
	classified := classifyKanbanFailure(data)
	if classified != nil && classified.Code == routingerr.CodeModelCapacity {
		return s.capacityContinuationRefusalReason(ctx, data)
	}
	safety := data.ContinuationSafety
	if safety == nil || !safety.Known || !data.EvidenceKnown || !s.continuationPromptIdentityMatches(data) {
		return continuationRefusalMissingEvidence
	}
	if safety.Unsafe {
		return "unsafe_work"
	}
	if data.OwnerKind != queueStatusScopeTask || data.DynamicRouteAttempt {
		return continuationRefusalUnsupportedRestore
	}
	switch {
	case safety.Pending:
		if safety.Support != streams.ContinuationAgentManagedToolsV3 {
			return "unsafe_work"
		}
		if !safety.SafeFor(data.PromptGeneration) {
			return continuationRefusalMissingEvidence
		}
		if classified == nil || classified.Code != routingerr.CodeAgentTransportLost ||
			data.PromptFailureDisposition != streams.PromptFailureDispositionRetainRuntime ||
			!data.PromptFailureDisposition.Valid() {
			return continuationRefusalUnsupportedRestore
		}
	case safety.Support != streams.ContinuationNativeSavedHistoryV1 &&
		safety.Support != streams.ContinuationNativeSavedHistoryV2 &&
		safety.Support != streams.ContinuationAgentManagedToolsV3:
		return continuationRefusalUnsupportedRestore
	case !safety.SafeFor(data.PromptGeneration):
		return continuationRefusalMissingEvidence
	}
	if s.continuationBindingForFailure(ctx, data) == nil {
		return continuationRefusalMissingEvidence
	}
	return ""
}

func (s *Service) capacityContinuationRefusalReason(ctx context.Context, data watcher.AgentEventData) string {
	snapshot := data.CapacityContinuation
	if !capacityContinuationEvidenceBound(s, snapshot, data) {
		return continuationRefusalMissingEvidence
	}
	if capacityContinuationHasUnsafeWork(snapshot) {
		return "unsafe_work"
	}
	if !supportsCapacityContinuation(snapshot.Support) {
		return continuationRefusalUnsupportedRestore
	}
	if !capacityContinuationAdmissionMatches(snapshot, data) {
		return continuationRefusalMissingEvidence
	}
	if s.continuationBindingForFailure(ctx, data) == nil {
		return continuationRefusalMissingEvidence
	}
	return ""
}

func capacityContinuationEvidenceBound(
	s *Service,
	snapshot *streams.CapacityContinuationSnapshot,
	data watcher.AgentEventData,
) bool {
	return snapshot != nil && data.EvidenceKnown && s.continuationPromptIdentityMatches(data) &&
		snapshot.PromptGeneration == data.PromptGeneration
}

func capacityContinuationHasUnsafeWork(snapshot *streams.CapacityContinuationSnapshot) bool {
	return snapshot.PendingTools || snapshot.FailedTools || snapshot.UnknownOutcomes ||
		snapshot.PermissionPending || snapshot.UnaccountedBackground
}

func supportsCapacityContinuation(support streams.CapacityContinuationSupport) bool {
	return support == streams.CapacityContinuationCodexLiveSessionV1 ||
		support == streams.CapacityContinuationMockLiveSessionV1
}

func capacityContinuationAdmissionMatches(
	snapshot *streams.CapacityContinuationSnapshot,
	data watcher.AgentEventData,
) bool {
	return snapshot.SafeFor(data.PromptGeneration) &&
		data.PromptFailureDisposition == streams.PromptFailureDispositionRetainRuntime &&
		data.OwnerKind == queueStatusScopeTask && !data.DynamicRouteAttempt
}

func (s *Service) continuationPromptIdentityMatches(data watcher.AgentEventData) bool {
	evidence, ok := s.promptAttemptForSession(data.SessionID)
	if !ok || data.AgentExecutionID == "" || data.PromptGeneration == 0 {
		return false
	}
	evidence.mu.Lock()
	defer evidence.mu.Unlock()
	return evidence.evidenceKnown && !evidence.dynamic && evidence.executionID == data.AgentExecutionID && evidence.promptGeneration == data.PromptGeneration
}

func continuationNativeID(session *models.TaskSession) string {
	if session.DownstreamACPSessionID != "" {
		return session.DownstreamACPSessionID
	}
	switch acp := session.Metadata["acp"].(type) {
	case map[string]any:
		id, _ := acp["session_id"].(string)
		return id
	case map[string]string:
		return acp["session_id"]
	}
	return ""
}

func continuationSessionIdentity(session *models.TaskSession) [32]byte {
	if session.AgentProfileSnapshot == nil {
		return [32]byte{}
	}
	selection, _ := models.LoadEffectiveSessionRuntimeConfig(session)
	worktrees := make([][]string, 0, len(session.Worktrees))
	for _, worktree := range session.Worktrees {
		if worktree == nil {
			return [32]byte{}
		}
		worktrees = append(worktrees, []string{worktree.RepositoryID, worktree.WorktreeID, worktree.WorktreePath, worktree.WorktreeBranch})
	}
	raw, err := json.Marshal([]any{session.AgentProfileID, session.ExecutionProfileID, session.ExecutorID, session.ExecutorProfileID,
		session.TaskEnvironmentID, session.EnvironmentID, session.WorkspacePath, session.BaseBranch, worktrees,
		session.AgentProfileSnapshot, session.ExecutorSnapshot, session.EnvironmentSnapshot, session.RepositorySnapshot,
		selection, session.Metadata["plan_mode"], session.Metadata["config_mode"]})
	if err != nil {
		return [32]byte{}
	}
	return sha256.Sum256(raw)
}
