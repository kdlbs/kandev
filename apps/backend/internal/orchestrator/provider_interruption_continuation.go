package orchestrator

import (
	"context"
	"crypto/sha256"
	"encoding/json"

	"github.com/kandev/kandev/internal/agent/runtime/routingerr"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	"github.com/kandev/kandev/internal/orchestrator/watcher"
	"github.com/kandev/kandev/internal/task/models"
)

const recoveryModeContinue = "continue"
const recoveryModeReplay = "replay"
const recoveryDispositionManual = "manual"
const recoveryDispositionExhausted = "exhausted"
const failureKindProviderInterrupted = "provider_interrupted"
const continuationRefusalMissingEvidence = "missing_evidence"

type continuationPolicy string

const (
	continuationPolicySavedHistoryRestore continuationPolicy = "saved_history_restore"
	continuationPolicyCapacityLive        continuationPolicy = "capacity_live"
)

// continuationBinding is an immutable admission snapshot, retained only in process.
type continuationBinding struct {
	policy         continuationPolicy
	nativeID       string
	identity       [32]byte
	workflowStepID string
}

func (s *Service) continuationBindingForFailure(ctx context.Context, data watcher.AgentEventData) *continuationBinding {
	classified := classifyKanbanFailure(data)
	if classified == nil {
		return nil
	}
	policy := continuationPolicy("")
	switch classified.Code {
	case routingerr.CodeAgentTransportLost:
		if !s.continuationFailureHasEvidence(data) {
			return nil
		}
		policy = continuationPolicySavedHistoryRestore
	case routingerr.CodeModelCapacity:
		if !s.capacityContinuationFailureHasEvidence(data) {
			return nil
		}
		policy = continuationPolicyCapacityLive
	default:
		return nil
	}
	session, err := s.repo.GetTaskSession(ctx, data.SessionID)
	if err != nil || session == nil || session.TaskID != data.TaskID || session.IsPassthrough || session.AgentProfileID == "" {
		return nil
	}
	task, err := s.repo.GetTask(ctx, data.TaskID)
	if err != nil || task == nil || task.IsFromOffice || task.ArchivedAt != nil ||
		(policy == continuationPolicyCapacityLive && models.IsAutomationTaskOrigin(task.Origin)) {
		return nil
	}
	nativeID := continuationNativeID(session)
	identity := continuationSessionIdentity(session)
	if nativeID == "" || identity == ([32]byte{}) {
		return nil
	}
	return &continuationBinding{policy: policy, nativeID: nativeID, identity: identity, workflowStepID: task.WorkflowStepID}
}

func (s *Service) continuationFailureHasEvidence(data watcher.AgentEventData) bool {
	return s.config.ProviderInterruptionContinuation && data.OwnerKind == queueStatusScopeTask && !data.DynamicRouteAttempt &&
		data.ContinuationSafety.SafeFor(data.PromptGeneration) && data.EvidenceKnown && s.continuationPromptIdentityMatches(data)
}

func (s *Service) capacityContinuationFailureHasEvidence(data watcher.AgentEventData) bool {
	classified := classifyKanbanFailure(data)
	return classified != nil && classified.Code == routingerr.CodeModelCapacity &&
		data.PromptFailureDisposition == streams.PromptFailureDispositionRetainRuntime &&
		data.PromptFailureDisposition.Valid() && data.OwnerKind == queueStatusScopeTask &&
		!data.DynamicRouteAttempt && data.CapacityContinuation.SafeFor(data.PromptGeneration) &&
		data.EvidenceKnown && s.continuationPromptIdentityMatches(data)
}

func (s *Service) continuationRefusalReason(ctx context.Context, data watcher.AgentEventData) string {
	classified := classifyKanbanFailure(data)
	if classified != nil && classified.Code == routingerr.CodeModelCapacity {
		return s.capacityContinuationRefusalReason(ctx, data)
	}
	if !s.config.ProviderInterruptionContinuation {
		return "disabled"
	}
	safety := data.ContinuationSafety
	if safety == nil || !safety.Known || !data.EvidenceKnown || !s.continuationPromptIdentityMatches(data) {
		return continuationRefusalMissingEvidence
	}
	if safety.Unsafe || safety.Pending {
		return "unsafe_work"
	}
	if safety.Support != streams.ContinuationNativeSavedHistoryV1 || data.OwnerKind != queueStatusScopeTask || data.DynamicRouteAttempt {
		return "unsupported_restore"
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
		return "unsupported_restore"
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
