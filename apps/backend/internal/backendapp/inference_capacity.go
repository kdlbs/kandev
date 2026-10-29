package backendapp

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

const inferenceStateUnknown = "unknown"
const inferenceSettlementNone = "not_required"

type inferenceClaimSettler interface {
	FinishProviderTurn(context.Context, string, uint64, bool) error
}

type inferenceClientSource interface {
	InferenceClients(context.Context) ([]agentruntime.InferenceClientRef, error)
}

type sharedInferenceConsumer struct {
	capacity  *orchestrator.Service
	settler   inferenceClaimSettler
	sources   []inferenceClientSource
	refreshMu sync.Mutex
	workerMu  sync.Mutex
	workers   map[string]struct{}
}

func inferenceBindingKey(owner, attempt string) string { return "inference:" + owner + ":" + attempt }

func (b *sharedInferenceConsumer) Execute(ctx context.Context, client *agentruntime.InferenceClient, req *utility.PromptRequest, identity agentruntime.InferenceIdentity) (*utility.PromptResponse, error) {
	if req == nil || b.capacity == nil {
		return nil, &agentruntime.InferenceCapacityError{Reason: "owner_unavailable"}
	}
	request := *req
	if identity.CallID != "" {
		request.CallID = identity.CallID
	} else if request.CallID == "" {
		request.CallID = uuid.NewString()
	}
	b.refreshMu.Lock()
	prepared, err := client.PrepareInferenceCapacityAttempt(ctx, request.CallID)
	if err == nil {
		b.markWorker(inferenceBindingKey(prepared.OwnerID, prepared.AttemptID), true)
	}
	b.refreshMu.Unlock()
	if err != nil {
		if errors.Is(err, agentruntime.ErrInferenceOwnershipUnsupported) {
			return b.executeLegacyInference(ctx, client, &request, identity)
		}
		return nil, &agentruntime.InferenceCapacityError{Reason: "ownership_unavailable", Cause: err}
	}
	request.AttemptID = prepared.AttemptID
	binding := b.newBinding(ctx, prepared, identity, &request)
	defer b.markWorker(binding.Key, false)
	defer b.cancelUnsentPrepared(client, binding)
	if all, finite := b.capacity.InferenceCapacityPolicy(); !prepared.Retained && all && finite {
		return nil, &agentruntime.InferenceCapacityError{Reason: "ownership_retention_unsupported", Cause: agentruntime.ErrInferenceOwnershipUnsupported}
	}
	if err := b.waitInferenceCapacity(ctx, client, binding, identity); err != nil {
		return nil, err
	}
	if err := b.validateInferenceSettlement(ctx, binding); err != nil {
		b.cancelAndSettle(client, binding)
		return nil, &agentruntime.InferenceCapacityError{Reason: "provider_allowance", Cause: err}
	}
	response, err := client.InferencePrompt(ctx, &request)
	return b.finishInferencePrompt(client, binding, response, err)
}

func (b *sharedInferenceConsumer) newBinding(ctx context.Context, prepared *utility.AttemptStatus, identity agentruntime.InferenceIdentity, req *utility.PromptRequest) sessioncapacity.InferenceBinding {
	grade, err := b.capacity.InferenceExecutionGrade(ctx, identity.TaskID, identity.LogicalProfileID)
	return sessioncapacity.InferenceBinding{Key: inferenceBindingKey(prepared.OwnerID, prepared.AttemptID), InstanceID: prepared.InstanceID, OwnerID: prepared.OwnerID, AttemptID: prepared.AttemptID, PreparationID: prepared.PreparationID,
		OwnerKind: identity.OwnerKind, TaskID: identity.TaskID, SessionID: identity.SessionID, ParentExecutionID: identity.ParentExecutionID,
		LogicalProfileID: identity.LogicalProfileID, ExecutionProfileID: identity.ExecutionProfileID, CallID: req.CallID, ReviewRunID: identity.ReviewRunID,
		AgentID: req.AgentID, ExecutorType: identity.ExecutorType, Model: req.Model, Grade: grade, GradeKnown: err == nil, State: "reserved", SettlementKind: inferenceSettlementNone}
}

func (b *sharedInferenceConsumer) waitInferenceCapacity(ctx context.Context, client *agentruntime.InferenceClient, binding sessioncapacity.InferenceBinding, identity agentruntime.InferenceIdentity) error {
	for {
		b.refresh(ctx, agentruntime.InferenceClientRef{Client: client, Identity: identity, Release: func() {}})
		wake := b.capacity.InferenceCapacityChanged()
		// Pending preparations own no slot yet. Freeze the current explicit
		// classification only when this reservation is actually admitted.
		grade, gradeErr := b.capacity.InferenceExecutionGrade(ctx, identity.TaskID, identity.LogicalProfileID)
		binding.Grade, binding.GradeKnown = grade, gradeErr == nil
		b.refreshMu.Lock()
		admitted, reason, err := b.capacity.ReserveInferenceCapacity(ctx, binding)
		b.refreshMu.Unlock()
		if admitted {
			return nil
		}
		all, finite := b.capacity.InferenceCapacityPolicy()
		if !all || !finite {
			return nil
		}
		if err != nil {
			return &agentruntime.InferenceCapacityError{Reason: reason, Cause: err}
		}
		select {
		case <-ctx.Done():
			return &agentruntime.InferenceCapacityError{Reason: "wait_cancelled", Cause: ctx.Err()}
		case <-wake:
		}
	}
}

func (b *sharedInferenceConsumer) markWorker(key string, active bool) {
	b.workerMu.Lock()
	defer b.workerMu.Unlock()
	if b.workers == nil {
		b.workers = make(map[string]struct{})
	}
	if active {
		b.workers[key] = struct{}{}
	} else {
		delete(b.workers, key)
	}
}

func (b *sharedInferenceConsumer) ownsWorker(key string) bool {
	b.workerMu.Lock()
	defer b.workerMu.Unlock()
	_, active := b.workers[key]
	return active
}

func inferenceSettlementContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

func (b *sharedInferenceConsumer) cancelUnsentPrepared(client *agentruntime.InferenceClient, binding sessioncapacity.InferenceBinding) {
	ctx, cancel := inferenceSettlementContext()
	defer cancel()
	status, err := client.CancelPreparedInferenceAttempt(ctx, binding.AttemptID)
	if err == nil {
		if b.settle(ctx, binding, status) {
			b.releaseSettledRetention(ctx, client, binding, status)
		}
	}
}

func (b *sharedInferenceConsumer) cancelAndSettle(client *agentruntime.InferenceClient, binding sessioncapacity.InferenceBinding) {
	b.cancelUnsentPrepared(client, binding)
}

func (b *sharedInferenceConsumer) settleInferenceResponse(client *agentruntime.InferenceClient, binding sessioncapacity.InferenceBinding, response *utility.PromptResponse, err error) *utility.AttemptStatus {
	ctx, cancel := inferenceSettlementContext()
	defer cancel()
	if response != nil && response.Attempt != nil {
		return b.settleObservedInference(ctx, client, binding, response.Attempt)
	}
	var failure *agentruntime.InferenceAttemptError
	if errors.As(err, &failure) && failure.Attempt != nil {
		return b.settleObservedInference(ctx, client, binding, failure.Attempt)
	}
	status, observeErr := client.InferenceAttempt(ctx, binding.AttemptID)
	if observeErr == nil {
		return b.settleObservedInference(ctx, client, binding, status)
	}
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	_ = b.capacity.UpdateInferenceCapacity(ctx, func(active map[string]sessioncapacity.InferenceBinding) error {
		if held, ok := active[binding.Key]; ok {
			held.State = inferenceStateUnknown
			active[binding.Key] = held
		}
		return nil
	})
	return nil
}

func (b *sharedInferenceConsumer) settleObservedInference(ctx context.Context, client *agentruntime.InferenceClient, binding sessioncapacity.InferenceBinding, status *utility.AttemptStatus) *utility.AttemptStatus {
	if !b.settle(ctx, binding, status) {
		return nil
	}
	b.releaseSettledRetention(ctx, client, binding, status)
	proof := *status
	return &proof
}

func (b *sharedInferenceConsumer) releaseSettledRetention(ctx context.Context, client *agentruntime.InferenceClient, binding sessioncapacity.InferenceBinding, status *utility.AttemptStatus) {
	if !definitelySettledInference(status) || !status.Retained {
		return
	}
	held, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return
	}
	if _, exists := held[binding.Key]; exists {
		return
	}
	_, _ = client.ReleaseInferenceAttempt(ctx, binding.AttemptID)
}

func (b *sharedInferenceConsumer) settle(ctx context.Context, binding sessioncapacity.InferenceBinding, status *utility.AttemptStatus) bool {
	b.refreshMu.Lock()
	defer b.refreshMu.Unlock()
	return b.settleLocked(ctx, binding, status)
}

func (b *sharedInferenceConsumer) settleLocked(ctx context.Context, binding sessioncapacity.InferenceBinding, status *utility.AttemptStatus) bool {
	if !matchesInferenceProof(binding, status) {
		return false
	}
	bindings, err := b.capacity.InferenceCapacityBindings(ctx)
	if err != nil {
		return false
	}
	held, exists := bindings[binding.Key]
	if !exists {
		return binding.SettlementKind == inferenceSettlementNone && binding.ProviderKey == ""
	}
	if !matchesInferenceProof(held, status) || !sameHeldInference(held, binding) {
		return false
	}
	if err := b.retainInferenceTerminal(ctx, held, status); err != nil {
		return false
	}
	held.TerminalState, held.TerminalPromptDispatch = status.State, status.PromptDispatch
	if err := b.finishInferenceClaim(ctx, held); err != nil {
		return false
	}
	return b.removeSettledInference(ctx, held) == nil
}

func (b *sharedInferenceConsumer) executeLegacyInference(ctx context.Context, client *agentruntime.InferenceClient, req *utility.PromptRequest, identity agentruntime.InferenceIdentity) (*utility.PromptResponse, error) {
	all, finite := b.capacity.InferenceCapacityPolicy()
	if all && finite {
		return nil, &agentruntime.InferenceCapacityError{Reason: "ownership_unsupported", Cause: agentruntime.ErrInferenceOwnershipUnsupported}
	}
	prepared := &utility.AttemptStatus{AttemptID: uuid.NewString(), InstanceID: client.InferenceInstanceID(), OwnerID: "unsupported"}
	if prepared.InstanceID == "" {
		return nil, fmt.Errorf("legacy inference instance identity unavailable")
	}
	binding := b.newBinding(ctx, prepared, identity, req)
	binding.State = inferenceStateUnknown
	b.refreshMu.Lock()
	_, _, _ = b.capacity.ReserveInferenceCapacity(ctx, binding)
	b.refreshMu.Unlock()
	response, err := client.InferencePrompt(ctx, req)
	if inferenceFailureNeedsSettlement(response, err) {
		return response, unconfirmedInferenceFailure(response, err)
	}
	return response, err
}
