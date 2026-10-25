package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
	taskrepo "github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/pkg/pluginsdk"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const (
	managedInputListDefaultLimit = 50
	managedInputListMaximumLimit = 200
	managedInputDispatchScope    = "managed_conversation_dispatch"
	managedInputDispatchPending  = "pending"
	managedInputDispatchAccepted = "accepted"
)

type managedInputServiceDependencies struct {
	storage         messagequeue.ManagedInputStorage
	resolveIdentity func(context.Context, string, string) (messagequeue.QueueSessionIdentity, error)
	maxPerSession   func() int
	notifyQueue     func(context.Context, string, string)
	stopExecution   func(context.Context, string, string, string) (bool, error)
}

type managedInputTarget struct {
	task       *models.Task
	session    *models.TaskSession
	descriptor pluginsdk.ManagedAgentConversationDescriptor
	identity   messagequeue.QueueSessionIdentity
}

type managedInputDispatchRecord struct {
	OperationID   string                               `json:"operation_id"`
	PayloadDigest string                               `json:"payload_digest"`
	Status        pluginsdk.ManagedAgentDispatchStatus `json:"status"`
	State         string                               `json:"state"`
}

func (s *AgentConversationService) managedInputDependencies() managedInputServiceDependencies {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return managedInputServiceDependencies{
		storage: s.managedInputStorage, resolveIdentity: s.managedInputIdentityResolver,
		maxPerSession: s.managedInputMaxPerSession, notifyQueue: s.managedInputQueueNotifier,
		stopExecution: s.managedInputExecutionStopper,
	}
}

func (s *AgentConversationService) notifyManagedInputQueue(ctx context.Context, taskID, sessionID string) {
	s.mu.RLock()
	notify := s.managedInputQueueNotifier
	s.mu.RUnlock()
	if notify != nil {
		notify(ctx, taskID, sessionID)
	}
}

// EnqueueManagedInput durably admits one host-minted input into the shared
// queue. The queue repository owns occurrence deduplication, coalescing,
// capacity, and FIFO sequence assignment.
func (s *AgentConversationService) EnqueueManagedInput(
	ctx context.Context,
	installationID, hostInputID string,
	input pluginsdk.ManagedAgentInputEnqueue,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentInputReceipt, bool, error) {
	if !validManagedInputEnqueueRequest(installationID, hostInputID, input, operationID, payloadDigest) {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.InvalidArgument, "invalid managed input enqueue")
	}
	deps := s.managedInputDependencies()
	if deps.storage == nil || deps.resolveIdentity == nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Unavailable, "managed input storage is unavailable")
	}
	if deps.notifyQueue == nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Unavailable, "managed input queue notifier is unavailable")
	}

	unlock := s.lockEnsureKey(managedConversationIdentity(installationID, input.WorkspaceID, input.InstanceKey))
	target, err := s.resolveManagedInputTarget(ctx, deps, installationID, input.WorkspaceID,
		input.InstanceKey, input.ExpectedConversationRevision, input.ApprovalRevision, input.ManifestDigest)
	if err != nil {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, err
	}
	contentDigest := managedInputPayloadDigest(input.Payload)
	request := messagequeue.ManagedInputRequest{
		ID: hostInputID, OccurrenceKey: input.OccurrenceKey, PayloadDigest: contentDigest,
		Payload: input.Payload, Origin: messagequeue.ManagedInputOrigin(input.Origin),
		CoalesceKey: input.CoalesceKey, ConversationRevision: int64(input.ExpectedConversationRevision),
	}
	maxPerSession := messagequeue.DefaultMaxPerSession
	if deps.maxPerSession != nil {
		maxPerSession = deps.maxPerSession()
	}
	stored, replayed, err := deps.storage.AdmitManagedInput(ctx, target.identity, request, maxPerSession)
	if err != nil {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, managedInputStorageError("admit", err)
	}
	if stored.ID != hostInputID {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Internal, "managed input receipt identity mismatch")
	}
	receipt, err := managedInputReceiptToSDK(stored)
	unlock()
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, err
	}
	if receipt.State == pluginsdk.ManagedAgentInputAccepted {
		// Admission is the durable boundary. Queue wake-up can start an agent
		// turn, so it must not hold the receipt open until that work completes.
		go deps.notifyQueue(context.WithoutCancel(ctx), target.task.ID, target.session.ID)
	}
	return receipt, replayed, nil
}

// GetManagedInput reads one receipt only within the current approved
// installation, workspace, and managed conversation.
func (s *AgentConversationService) GetManagedInput(
	ctx context.Context,
	installationID string,
	query pluginsdk.ManagedAgentInputQuery,
) (pluginsdk.ManagedAgentInputReceipt, error) {
	if installationID == "" || !validManagedInputReadScope(query.WorkspaceID, query.InstanceKey, query.ApprovalRevision, query.ManifestDigest) ||
		query.HostInputID == "" || len(query.HostInputID) > messagequeue.MaxQueueAdmissionIDLength {
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(codes.InvalidArgument, "invalid managed input query")
	}
	deps := s.managedInputDependencies()
	if deps.storage == nil || deps.resolveIdentity == nil {
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(codes.Unavailable, "managed input storage is unavailable")
	}
	target, err := s.resolveManagedInputTarget(ctx, deps, installationID, query.WorkspaceID,
		query.InstanceKey, 0, query.ApprovalRevision, query.ManifestDigest)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, err
	}
	stored, err := deps.storage.GetManagedInput(ctx, target.identity, query.HostInputID)
	if err != nil {
		return pluginsdk.ManagedAgentInputReceipt{}, managedInputStorageError("read", err)
	}
	return managedInputReceiptToSDK(stored)
}

// ListManagedInputs returns a bounded page ordered by the queue's durable
// sequence, after the supplied exclusive cursor.
func (s *AgentConversationService) ListManagedInputs(
	ctx context.Context,
	installationID string,
	query pluginsdk.ManagedAgentInputListQuery,
) (pluginsdk.ManagedAgentInputPage, error) {
	if installationID == "" || !validManagedInputReadScope(query.WorkspaceID, query.InstanceKey, query.ApprovalRevision, query.ManifestDigest) ||
		query.Limit > managedInputListMaximumLimit {
		return pluginsdk.ManagedAgentInputPage{}, status.Error(codes.InvalidArgument, "invalid managed input list query")
	}
	limit := int(query.Limit)
	if limit == 0 {
		limit = managedInputListDefaultLimit
	}
	deps := s.managedInputDependencies()
	if deps.storage == nil || deps.resolveIdentity == nil {
		return pluginsdk.ManagedAgentInputPage{}, status.Error(codes.Unavailable, "managed input storage is unavailable")
	}
	target, err := s.resolveManagedInputTarget(ctx, deps, installationID, query.WorkspaceID,
		query.InstanceKey, 0, query.ApprovalRevision, query.ManifestDigest)
	if err != nil {
		return pluginsdk.ManagedAgentInputPage{}, err
	}
	stored, err := deps.storage.ListManagedInputs(ctx, target.identity)
	if err != nil {
		return pluginsdk.ManagedAgentInputPage{}, managedInputStorageError("list", err)
	}
	page := pluginsdk.ManagedAgentInputPage{
		Inputs:             make([]pluginsdk.ManagedAgentInputReceipt, 0, limit),
		NextSequenceCursor: query.SequenceCursor,
	}
	for _, item := range stored {
		if item.Sequence < 0 {
			return pluginsdk.ManagedAgentInputPage{}, status.Error(codes.Internal, "managed input sequence is invalid")
		}
		if uint64(item.Sequence) <= query.SequenceCursor {
			continue
		}
		if len(page.Inputs) == limit {
			page.HasMore = true
			break
		}
		receipt, mapErr := managedInputReceiptToSDK(item)
		if mapErr != nil {
			return pluginsdk.ManagedAgentInputPage{}, mapErr
		}
		page.Inputs = append(page.Inputs, receipt)
		page.NextSequenceCursor = receipt.Sequence
	}
	return page, nil
}

// CancelManagedInput atomically removes accepted work or stops only the exact
// active execution generation named by the caller.
//
//nolint:cyclop,funlen // Cancellation reconciles durable input and execution state before changing the queue.
func (s *AgentConversationService) CancelManagedInput(
	ctx context.Context,
	installationID string,
	input pluginsdk.ManagedAgentInputCancel,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentInputReceipt, bool, error) {
	if installationID == "" || !validManagedMutationIdentity(input.WorkspaceID, input.InstanceKey,
		input.ExpectedConversationRevision, input.ApprovalRevision, input.ManifestDigest,
		input.HostInputID, operationID, payloadDigest) || strings.TrimSpace(input.ExpectedExecutionID) != input.ExpectedExecutionID ||
		len(input.ExpectedExecutionID) > 256 || input.RequestID == "" || input.IdempotencyKey == "" {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.InvalidArgument, "invalid managed input cancellation")
	}
	deps := s.managedInputDependencies()
	if deps.storage == nil || deps.resolveIdentity == nil {
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Unavailable, "managed input storage is unavailable")
	}
	unlock := s.lockEnsureKey(managedConversationIdentity(installationID, input.WorkspaceID, input.InstanceKey))
	target, err := s.resolveManagedInputTarget(ctx, deps, installationID, input.WorkspaceID,
		input.InstanceKey, input.ExpectedConversationRevision, input.ApprovalRevision, input.ManifestDigest)
	if err != nil {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, err
	}
	stored, err := deps.storage.GetManagedInput(ctx, target.identity, input.HostInputID)
	if err != nil {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, managedInputStorageError("read before cancellation", err)
	}
	if stored.State == messagequeue.ManagedInputStateAccepted {
		if input.ExpectedExecutionID != "" {
			unlock()
			return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.InvalidArgument, "accepted input cancellation must not name an execution")
		}
		cancelled, changed, cancelErr := deps.storage.CancelManagedInput(ctx, target.identity, input.HostInputID)
		if cancelErr != nil {
			unlock()
			return pluginsdk.ManagedAgentInputReceipt{}, false, managedInputStorageError("cancel accepted input", cancelErr)
		}
		stored = cancelled
		receipt, mapErr := managedInputReceiptToSDK(stored)
		unlock()
		if changed && deps.notifyQueue != nil {
			deps.notifyQueue(ctx, target.task.ID, target.session.ID)
		}
		return receipt, changed, mapErr
	}
	if input.ExpectedExecutionID == "" {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.InvalidArgument, "non-accepted input cancellation must name an execution")
	}
	if stored.ExecutionID != input.ExpectedExecutionID {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Aborted, "managed input execution changed")
	}
	if stored.State != messagequeue.ManagedInputStateRunning {
		receipt, mapErr := managedInputReceiptToSDK(stored)
		unlock()
		return receipt, false, mapErr
	}
	if target.session.AgentExecutionID != input.ExpectedExecutionID {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Aborted, "managed input execution is no longer current")
	}
	if deps.stopExecution == nil {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, status.Error(codes.Unavailable, "managed input execution stopper is unavailable")
	}
	stopped, stopErr := deps.stopExecution(ctx, target.task.ID, target.session.ID, input.ExpectedExecutionID)
	if stopErr != nil {
		unlock()
		return pluginsdk.ManagedAgentInputReceipt{}, false, fmt.Errorf("stop managed input execution: %w", stopErr)
	}
	if !stopped {
		receipt, mapErr := managedInputReceiptToSDK(stored)
		unlock()
		return receipt, false, mapErr
	}
	settled, changed, settleErr := deps.storage.SettleManagedInput(ctx, target.identity,
		input.HostInputID, stored.TurnID, input.ExpectedExecutionID,
		messagequeue.ManagedInputStateCancelled, "stop_confirmed")
	if settleErr != nil {
		latest, latestErr := deps.storage.GetManagedInput(ctx, target.identity, input.HostInputID)
		unlock()
		if latestErr == nil && latest.State != messagequeue.ManagedInputStateRunning {
			receipt, mapErr := managedInputReceiptToSDK(latest)
			return receipt, false, mapErr
		}
		return pluginsdk.ManagedAgentInputReceipt{}, false, managedInputStorageError("settle stopped input", settleErr)
	}
	receipt, mapErr := managedInputReceiptToSDK(settled)
	unlock()
	if changed && deps.notifyQueue != nil {
		deps.notifyQueue(ctx, target.task.ID, target.session.ID)
	}
	return receipt, changed, mapErr
}

// DispatchManagedInput performs one immediate turn attempt. Busy and queued
// states are returned before delivery; this method never creates a receipt or
// queue entry.
//
//nolint:cyclop,funlen // Dispatch binds durable input admission to the selected conversation and execution.
func (s *AgentConversationService) DispatchManagedInput(
	ctx context.Context,
	installationID string,
	input pluginsdk.ManagedAgentConversationDispatch,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentDispatchStatus, pluginsdk.ManagedAgentConversationDescriptor, error) {
	if !validManagedInputDispatchRequest(installationID, input, operationID, payloadDigest) {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.InvalidArgument, "invalid immediate managed conversation dispatch")
	}
	deps := s.managedInputDependencies()
	if deps.storage == nil || deps.resolveIdentity == nil {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Unavailable, "managed input storage is unavailable")
	}
	if s.state == nil {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Unavailable, "managed dispatch idempotency store is unavailable")
	}
	unlock := s.lockEnsureKey(managedConversationIdentity(installationID, input.WorkspaceID, input.InstanceKey))
	defer unlock()
	target, err := s.resolveManagedInputTarget(ctx, deps, installationID, input.WorkspaceID,
		input.InstanceKey, input.ExpectedConversationRevision, input.ApprovalRevision, input.ManifestDigest)
	if err != nil {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, err
	}
	if replayStatus, found, replayErr := s.readManagedDispatchOccurrence(ctx, installationID, input,
		operationID, payloadDigest); errors.Is(replayErr, errManagedDispatchOccurrencePending) {
		return pluginsdk.ManagedAgentDispatchBusy, target.descriptor, nil
	} else if replayErr != nil {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, replayErr
	} else if found {
		return replayStatus, target.descriptor, nil
	}
	if managedConversationPaused(target.task) || !managedConversationSessionIdle(target.session) {
		return pluginsdk.ManagedAgentDispatchBusy, target.descriptor, nil
	}
	pending, err := deps.storage.ListManagedInputs(ctx, target.identity)
	if err != nil {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, managedInputStorageError("check immediate dispatch queue", err)
	}
	for _, receipt := range pending {
		if receipt.State == messagequeue.ManagedInputStateAccepted || receipt.State == messagequeue.ManagedInputStateRunning {
			return pluginsdk.ManagedAgentDispatchBusy, target.descriptor, nil
		}
	}
	dispatcher := s.getDispatcher()
	if dispatcher == nil {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Unavailable, "agent conversation dispatch is not ready yet")
	}
	immediate, ok := dispatcher.(agentConversationImmediateDispatcher)
	if !ok {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Unavailable, "immediate managed dispatch is unavailable")
	}
	if replayStatus, claimed, claimErr := s.claimManagedDispatchOccurrence(ctx, installationID, input,
		operationID, payloadDigest); claimErr != nil {
		if errors.Is(claimErr, errManagedDispatchOccurrencePending) {
			return pluginsdk.ManagedAgentDispatchBusy, target.descriptor, nil
		}
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, claimErr
	} else if !claimed {
		return replayStatus, target.descriptor, nil
	}
	messageID := conversationIdentity("managed-dispatch", installationID, input.WorkspaceID, input.InstanceKey, input.OccurrenceKey)
	dispatchStatus, err := immediate.DispatchImmediate(ctx, target.task.ID, target.session,
		composeConversationPrompt(target.task, input.Payload), "plugin-installation:"+installationID, messageID)
	if err != nil {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if deleteErr := s.state.Delete(cleanupCtx, installationID, managedInputDispatchScope,
			managedInputDispatchScopeID(input), input.OccurrenceKey); deleteErr != nil {
			return "", pluginsdk.ManagedAgentConversationDescriptor{}, errors.Join(
				fmt.Errorf("dispatch managed conversation input: %w", err),
				fmt.Errorf("release failed dispatch occurrence: %w", deleteErr),
			)
		}
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, fmt.Errorf("dispatch managed conversation input: %w", err)
	}
	if dispatchStatus == pluginsdk.ManagedAgentDispatchBusy {
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := s.state.Delete(cleanupCtx, installationID, managedInputDispatchScope,
			managedInputDispatchScopeID(input), input.OccurrenceKey); err != nil {
			return "", pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Unavailable, "busy dispatch result could not be released")
		}
		return pluginsdk.ManagedAgentDispatchBusy, target.descriptor, nil
	}
	if dispatchStatus != pluginsdk.ManagedAgentDispatchStarted && dispatchStatus != pluginsdk.ManagedAgentDispatchSent {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Unavailable, "managed dispatch outcome is not recognized")
	}
	if err := s.saveManagedDispatchOccurrence(ctx, installationID, input, operationID, payloadDigest, dispatchStatus); err != nil {
		return "", pluginsdk.ManagedAgentConversationDescriptor{}, status.Error(codes.Unavailable, "managed dispatch result could not be persisted")
	}
	return dispatchStatus, target.descriptor, nil
}

var errManagedDispatchOccurrencePending = errors.New("managed dispatch occurrence is pending")

func (s *AgentConversationService) readManagedDispatchOccurrence(
	ctx context.Context,
	installationID string,
	input pluginsdk.ManagedAgentConversationDispatch,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentDispatchStatus, bool, error) {
	raw, found, err := s.state.Get(ctx, installationID, managedInputDispatchScope,
		managedInputDispatchScopeID(input), input.OccurrenceKey)
	if err != nil || !found {
		return "", found, err
	}
	var record managedInputDispatchRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return "", true, status.Error(codes.Internal, "managed dispatch occurrence record is invalid")
	}
	if record.PayloadDigest != payloadDigest {
		return "", true, status.Error(codes.Aborted, "managed dispatch occurrence payload conflicts")
	}
	if record.State == managedInputDispatchPending {
		return "", true, errManagedDispatchOccurrencePending
	}
	if record.State != managedInputDispatchAccepted || (record.Status != pluginsdk.ManagedAgentDispatchStarted &&
		record.Status != pluginsdk.ManagedAgentDispatchSent) {
		return "", true, status.Error(codes.Internal, "managed dispatch occurrence state is invalid")
	}
	return record.Status, true, nil
}

func (s *AgentConversationService) claimManagedDispatchOccurrence(
	ctx context.Context,
	installationID string,
	input pluginsdk.ManagedAgentConversationDispatch,
	operationID, payloadDigest string,
) (pluginsdk.ManagedAgentDispatchStatus, bool, error) {
	encoded, err := json.Marshal(managedInputDispatchRecord{
		OperationID: operationID, PayloadDigest: payloadDigest, State: managedInputDispatchPending,
	})
	if err != nil {
		return "", false, status.Error(codes.Internal, "managed dispatch intent could not be encoded")
	}
	claimed, err := s.state.Claim(ctx, installationID, managedInputDispatchScope,
		managedInputDispatchScopeID(input), input.OccurrenceKey, encoded)
	if err != nil {
		return "", false, fmt.Errorf("claim managed dispatch occurrence: %w", err)
	}
	if claimed {
		return "", true, nil
	}
	replayStatus, found, err := s.readManagedDispatchOccurrence(ctx, installationID, input, operationID, payloadDigest)
	if errors.Is(err, errManagedDispatchOccurrencePending) {
		return "", false, errManagedDispatchOccurrencePending
	}
	if err != nil {
		return "", false, err
	}
	if found {
		return replayStatus, false, nil
	}
	return "", false, status.Error(codes.Aborted, "managed dispatch occurrence changed")
}

func (s *AgentConversationService) saveManagedDispatchOccurrence(
	ctx context.Context,
	installationID string,
	input pluginsdk.ManagedAgentConversationDispatch,
	operationID, payloadDigest string,
	dispatchStatus pluginsdk.ManagedAgentDispatchStatus,
) error {
	encoded, err := json.Marshal(managedInputDispatchRecord{
		OperationID: operationID, PayloadDigest: payloadDigest, Status: dispatchStatus,
		State: managedInputDispatchAccepted,
	})
	if err != nil {
		return err
	}
	return s.state.Set(ctx, installationID, managedInputDispatchScope,
		managedInputDispatchScopeID(input), input.OccurrenceKey, encoded)
}

func managedInputDispatchScopeID(input pluginsdk.ManagedAgentConversationDispatch) string {
	return input.WorkspaceID + "/" + input.InstanceKey
}

//nolint:cyclop // Target resolution verifies conversation ownership and the current session relationship.
func (s *AgentConversationService) resolveManagedInputTarget(
	ctx context.Context,
	deps managedInputServiceDependencies,
	installationID, workspaceID, instanceKey string,
	expectedRevision, approvalRevision uint64,
	manifestDigest string,
) (managedInputTarget, error) {
	task, err := s.findRetainedManagedConversation(ctx, installationID, workspaceID, instanceKey)
	if err != nil {
		return managedInputTarget{}, err
	}
	if task == nil || managedConversationDetached(task) {
		return managedInputTarget{}, status.Error(codes.NotFound, "managed conversation not found")
	}
	if expectedRevision > 0 && managedConversationRevision(task) != expectedRevision {
		return managedInputTarget{}, status.Error(codes.Aborted, "managed conversation revision is stale")
	}
	if invalidated, _ := task.Metadata[models.MetaKeyManagedPolicyInvalidated].(bool); invalidated {
		return managedInputTarget{}, status.Error(codes.FailedPrecondition, "managed conversation approval must be refreshed")
	}
	if approvalRevision == 0 || managedConversationUint(task.Metadata[metaKeyManagedApprovalRevision]) != approvalRevision ||
		models.StringFromAny(task.Metadata[metaKeyManagedManifestDigest]) != manifestDigest {
		return managedInputTarget{}, status.Error(codes.PermissionDenied, "managed conversation approval does not match")
	}
	session, err := s.sess.GetPrimarySessionByTaskID(ctx, task.ID)
	if errors.Is(err, taskrepo.ErrNoPrimarySession) || (err == nil && session == nil) {
		return managedInputTarget{}, status.Error(codes.NotFound, "managed conversation session not found")
	}
	if err != nil {
		return managedInputTarget{}, err
	}
	if session.TaskID != task.ID || session.ID == "" || session.QueueIncarnationID == "" {
		return managedInputTarget{}, status.Error(codes.Aborted, "managed conversation session identity is incomplete")
	}
	identity, err := deps.resolveIdentity(ctx, task.ID, session.ID)
	if err != nil {
		return managedInputTarget{}, managedInputStorageError("resolve queue identity", err)
	}
	if identity.TaskID != task.ID || identity.SessionID != session.ID || identity.SessionIncarnationID == "" ||
		identity.SessionIncarnationID != session.QueueIncarnationID {
		return managedInputTarget{}, status.Error(codes.Aborted, "managed conversation session identity changed")
	}
	return managedInputTarget{
		task: task, session: session, descriptor: managedConversationDescriptor(installationID, task, session),
		identity: identity,
	}, nil
}

func managedInputReceiptToSDK(receipt messagequeue.ManagedInputReceipt) (pluginsdk.ManagedAgentInputReceipt, error) {
	if receipt.ID == "" || receipt.Sequence < 0 || receipt.ConversationRevision < 0 {
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(codes.Internal, "managed input receipt is invalid")
	}
	state := pluginsdk.ManagedAgentInputState(receipt.State)
	if receipt.State == messagequeue.ManagedInputStateSuperseded {
		state = pluginsdk.ManagedAgentInputCancelled
	}
	switch state {
	case pluginsdk.ManagedAgentInputAccepted, pluginsdk.ManagedAgentInputRunning,
		pluginsdk.ManagedAgentInputCompleted, pluginsdk.ManagedAgentInputFailed,
		pluginsdk.ManagedAgentInputCancelled, pluginsdk.ManagedAgentInputUncertain:
	default:
		return pluginsdk.ManagedAgentInputReceipt{}, status.Error(codes.Internal, "managed input receipt state is invalid")
	}
	createdAt, updatedAt := "", ""
	if !receipt.CreatedAt.IsZero() {
		createdAt = receipt.CreatedAt.UTC().Format(time.RFC3339Nano)
	}
	if !receipt.UpdatedAt.IsZero() {
		updatedAt = receipt.UpdatedAt.UTC().Format(time.RFC3339Nano)
	}
	return pluginsdk.ManagedAgentInputReceipt{
		HostInputID: receipt.ID, OccurrenceKey: receipt.OccurrenceKey,
		Sequence: uint64(receipt.Sequence), Origin: pluginsdk.ManagedAgentInputOrigin(receipt.Origin),
		Payload: receipt.Payload, CoalesceKey: receipt.CoalesceKey,
		ConversationRevision: uint64(receipt.ConversationRevision), State: state,
		CreatedAt: createdAt, UpdatedAt: updatedAt, QueueEntryID: receipt.ID,
		ExecutionID: receipt.ExecutionID, TurnID: receipt.TurnID, SupersededBy: receipt.SupersededBy,
	}, nil
}

func managedInputStorageError(operation string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, messagequeue.ErrEntryNotFound):
		return status.Error(codes.NotFound, "managed input not found")
	case errors.Is(err, messagequeue.ErrQueueFull):
		return status.Error(codes.ResourceExhausted, "managed conversation input queue is full")
	case errors.Is(err, messagequeue.ErrQueueIDConflict):
		return status.Error(codes.Aborted, "managed input occurrence conflicts with an existing input")
	case errors.Is(err, messagequeue.ErrSessionIdentityMismatch):
		return status.Error(codes.Aborted, "managed conversation session identity changed")
	case errors.Is(err, messagequeue.ErrManagedInputBusy), errors.Is(err, messagequeue.ErrManagedInputNotHead),
		errors.Is(err, messagequeue.ErrManagedInputNotPending), errors.Is(err, messagequeue.ErrManagedInputTransition):
		return status.Error(codes.Aborted, "managed input state changed")
	default:
		return fmt.Errorf("managed input %s: %w", operation, err)
	}
}

func validManagedInputEnqueueRequest(
	installationID, hostInputID string,
	input pluginsdk.ManagedAgentInputEnqueue,
	operationID, payloadDigest string,
) bool {
	return installationID != "" && validManagedMutationIdentity(input.WorkspaceID, input.InstanceKey,
		input.ExpectedConversationRevision, input.ApprovalRevision, input.ManifestDigest,
		hostInputID, operationID, payloadDigest) &&
		input.RequestID != "" && input.IdempotencyKey != "" && input.OccurrenceKey != "" &&
		strings.TrimSpace(input.OccurrenceKey) == input.OccurrenceKey && len(input.OccurrenceKey) <= 512 &&
		input.Payload != "" && len(input.Payload) <= 65536 && !strings.ContainsRune(input.Payload, '\x00') &&
		validManagedInputOrigin(input.Origin) && validManagedInputCoalesce(input.Origin, input.CoalesceKey) &&
		input.ExpectedConversationRevision <= uint64(1<<63-1)
}

func validManagedInputDispatchRequest(
	installationID string,
	input pluginsdk.ManagedAgentConversationDispatch,
	operationID, payloadDigest string,
) bool {
	return installationID != "" && validManagedMutationIdentity(input.WorkspaceID, input.InstanceKey,
		input.ExpectedConversationRevision, input.ApprovalRevision, input.ManifestDigest,
		input.RequestID, operationID, payloadDigest) && input.IdempotencyKey != "" &&
		input.OccurrenceKey != "" && strings.TrimSpace(input.OccurrenceKey) == input.OccurrenceKey &&
		len(input.OccurrenceKey) <= 512 && input.Payload != "" && len(input.Payload) <= 65536 &&
		!strings.ContainsRune(input.Payload, '\x00') && validManagedInputOrigin(input.Origin) &&
		input.CoalesceKey == "" && input.ExpectedConversationRevision <= uint64(1<<63-1)
}

func validManagedInputReadScope(workspaceID, instanceKey string, approvalRevision uint64, manifestDigest string) bool {
	return workspaceID != "" && instanceKey != "" && len(instanceKey) <= 128 &&
		strings.TrimSpace(instanceKey) == instanceKey && approvalRevision > 0 && len(manifestDigest) == 64
}

func validManagedMutationIdentity(
	workspaceID, instanceKey string,
	expectedRevision, approvalRevision uint64,
	manifestDigest, resourceID, operationID, payloadDigest string,
) bool {
	return workspaceID != "" && instanceKey != "" && len(instanceKey) <= 128 &&
		strings.TrimSpace(instanceKey) == instanceKey && expectedRevision > 0 &&
		expectedRevision <= uint64(1<<63-1) && approvalRevision > 0 && len(manifestDigest) == 64 &&
		resourceID != "" && len(resourceID) <= messagequeue.MaxQueueAdmissionIDLength &&
		operationID != "" && len(operationID) <= 256 && payloadDigest != "" && len(payloadDigest) <= 128
}

func validManagedInputOrigin(origin pluginsdk.ManagedAgentInputOrigin) bool {
	switch origin {
	case pluginsdk.ManagedAgentInputHuman, pluginsdk.ManagedAgentInputAutomation,
		pluginsdk.ManagedAgentInputPeriodic, pluginsdk.ManagedAgentInputInteraction:
		return true
	default:
		return false
	}
}

func validManagedInputCoalesce(origin pluginsdk.ManagedAgentInputOrigin, key string) bool {
	if key == "" {
		return true
	}
	return origin == pluginsdk.ManagedAgentInputPeriodic && len(key) <= 128 && strings.TrimSpace(key) == key
}

func managedInputPayloadDigest(payload string) string {
	digest := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(digest[:])
}
