package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

const (
	deliveryReconciliationWindow       = 10 * time.Second
	deliveryReconciliationAttemptLimit = 3
	deliveryReconciliationAttemptCap   = 3 * time.Second
)

var (
	ErrDeliveryTransportUnavailable = errors.New("agent delivery transport is unavailable")
	ErrDeliveryOwnerMismatch        = errors.New("agent delivery owner no longer matches")
)

type DeliveryReconciliationOutcome string

const (
	DeliveryReconciliationRunningAttached      DeliveryReconciliationOutcome = "running_attached"
	DeliveryReconciliationTerminalSettled      DeliveryReconciliationOutcome = "terminal_settled"
	DeliveryReconciliationUncertain            DeliveryReconciliationOutcome = "uncertain"
	DeliveryReconciliationOwnerMismatch        DeliveryReconciliationOutcome = "owner_mismatch"
	DeliveryReconciliationTransportUnavailable DeliveryReconciliationOutcome = "transport_unavailable"
)

type DeliveryReconciliationPhase string

const (
	DeliveryReconciliationPhaseReconnecting DeliveryReconciliationPhase = "reconnecting"
	DeliveryReconciliationPhaseUncertain    DeliveryReconciliationPhase = "uncertain"
	DeliveryReconciliationPhaseRecovered    DeliveryReconciliationPhase = "recovered"
)

// DeliveryReconciliationIdentity binds every result to the owner that
// accepted the prompt. RuntimeEpoch is local-runtime fencing only; remote
// executor identity is independent of that local generation.
type DeliveryReconciliationIdentity struct {
	SessionID         string
	ExecutionID       string
	Owner             ExecutionOwner
	IncarnationID     string
	HarnessGeneration uint64
	StreamID          string
	SubmissionID      string
	RuntimeEpoch      uint64
	StartupGeneration uint64
	PromptGeneration  uint64
}

type DeliveryReconciliationResult struct {
	Identity DeliveryReconciliationIdentity
	Outcome  DeliveryReconciliationOutcome
	Attempts int
	Err      error
}

func (r DeliveryReconciliationResult) AsError() error {
	if errors.Is(r.Err, context.Canceled) {
		return r.Err
	}
	switch r.Outcome {
	case DeliveryReconciliationRunningAttached, DeliveryReconciliationTerminalSettled:
		return nil
	case DeliveryReconciliationOwnerMismatch:
		return errors.Join(ErrDeliveryOwnerMismatch, r.Err)
	case DeliveryReconciliationTransportUnavailable:
		return errors.Join(ErrDeliveryTransportUnavailable, r.Err)
	default:
		return errors.Join(ErrUncertainPromptDelivery, r.Err)
	}
}

type deliveryReconciliationPeer interface {
	GetStatus(context.Context) (*agentctl.StatusResponse, error)
	GetDeliveryStatus(context.Context, string) (*agentctl.DeliveryStatus, error)
	GetDeliverySubmission(context.Context, string) (*journal.Submission, error)
	HasAgentStream() bool
	AttachAgentStream(context.Context) error
}

type agentctlDeliveryReconciliationPeer struct {
	client *agentctl.Client
	attach func(context.Context) error
}

func (p agentctlDeliveryReconciliationPeer) GetStatus(ctx context.Context) (*agentctl.StatusResponse, error) {
	return p.client.GetStatus(ctx)
}

func (p agentctlDeliveryReconciliationPeer) GetDeliveryStatus(
	ctx context.Context,
	streamID string,
) (*agentctl.DeliveryStatus, error) {
	return p.client.GetDeliveryStatus(ctx, streamID)
}

func (p agentctlDeliveryReconciliationPeer) GetDeliverySubmission(
	ctx context.Context,
	submissionID string,
) (*journal.Submission, error) {
	return p.client.GetDeliverySubmission(ctx, submissionID)
}

func (p agentctlDeliveryReconciliationPeer) HasAgentStream() bool {
	return p.client.HasAgentStream()
}

func (p agentctlDeliveryReconciliationPeer) AttachAgentStream(ctx context.Context) error {
	return p.attach(ctx)
}

type deliveryReconciliationCycle struct {
	identity DeliveryReconciliationIdentity
	ctx      context.Context
	done     chan struct{}
	cancel   context.CancelFunc
	waiters  int
	result   DeliveryReconciliationResult
}

func captureDeliveryReconciliationIdentity(execution *AgentExecution) (DeliveryReconciliationIdentity, error) {
	if execution == nil {
		return DeliveryReconciliationIdentity{}, ErrExecutionNotFound
	}
	submissionID := execution.deliverySubmissionIDSnapshot()
	if execution.ID == "" || execution.SessionID == "" || execution.DeliveryMode != DurableDeliveryV1 ||
		execution.DeliveryIncarnationID == "" || execution.DeliveryHarnessGeneration == 0 ||
		execution.DeliveryStreamID == "" || submissionID == "" {
		return DeliveryReconciliationIdentity{}, fmt.Errorf("durable delivery reconciliation identity is incomplete")
	}
	return DeliveryReconciliationIdentity{
		SessionID: execution.SessionID, ExecutionID: execution.ID, Owner: execution.Owner,
		IncarnationID: execution.DeliveryIncarnationID, HarnessGeneration: execution.DeliveryHarnessGeneration,
		StreamID: execution.DeliveryStreamID, SubmissionID: submissionID, RuntimeEpoch: execution.runtimeEpoch,
		StartupGeneration: execution.startupAttemptSnapshot(), PromptGeneration: execution.promptGenerationSnapshot(),
	}, nil
}

// ReconcileAgentDelivery queries and reattaches the exact durable prompt
// accepted by this execution. It never initializes an ACP session or resends
// a prompt.
func (sm *StreamManager) ReconcileAgentDelivery(
	ctx context.Context,
	execution *AgentExecution,
) DeliveryReconciliationResult {
	identity, err := captureDeliveryReconciliationIdentity(execution)
	if err != nil {
		return DeliveryReconciliationResult{Outcome: DeliveryReconciliationUncertain, Err: err}
	}
	client, release := execution.AcquireAgentCtlClient()
	if client == nil {
		return DeliveryReconciliationResult{
			Identity: identity, Outcome: DeliveryReconciliationTransportUnavailable,
			Err: ErrDeliveryTransportUnavailable,
		}
	}
	defer release()
	if identity.RuntimeEpoch != 0 && client.RuntimeEpoch() != 0 && client.RuntimeEpoch() != identity.RuntimeEpoch {
		return DeliveryReconciliationResult{
			Identity: identity, Outcome: DeliveryReconciliationOwnerMismatch,
			Err: ErrDeliveryOwnerMismatch,
		}
	}
	peer := agentctlDeliveryReconciliationPeer{
		client: client,
		attach: func(attachCtx context.Context) error {
			return sm.attachUpdatesStream(attachCtx, execution, client)
		},
	}
	return sm.reconcileDeliveryWithPeer(ctx, execution, peer)
}

func (sm *StreamManager) reconcileDeliveryWithPeer(
	ctx context.Context,
	execution *AgentExecution,
	peer deliveryReconciliationPeer,
) DeliveryReconciliationResult {
	identity, err := captureDeliveryReconciliationIdentity(execution)
	if err != nil {
		return DeliveryReconciliationResult{Outcome: DeliveryReconciliationUncertain, Err: err}
	}
	if peer == nil {
		return DeliveryReconciliationResult{Identity: identity, Outcome: DeliveryReconciliationTransportUnavailable, Err: ErrDeliveryTransportUnavailable}
	}
	sm.reconciliationMu.Lock()
	_, stopped := sm.stoppedReconciliations[identity.ExecutionID]
	sm.reconciliationMu.Unlock()
	if stopped {
		return DeliveryReconciliationResult{Identity: identity, Outcome: DeliveryReconciliationTransportUnavailable, Err: context.Canceled}
	}
	cycle, joined := sm.beginDeliveryReconciliation(ctx, identity)
	if cycle == nil {
		return DeliveryReconciliationResult{Identity: identity, Outcome: DeliveryReconciliationTransportUnavailable, Err: context.Canceled}
	}
	if joined {
		return sm.waitDeliveryReconciliation(ctx, cycle)
	}
	result := sm.runDeliveryReconciliation(cycle.ctx, execution, identity, peer)
	sm.finishDeliveryReconciliation(cycle, result)
	return result
}

func (sm *StreamManager) beginDeliveryReconciliation(
	ctx context.Context,
	identity DeliveryReconciliationIdentity,
) (*deliveryReconciliationCycle, bool) {
	sm.reconciliationMu.Lock()
	defer sm.reconciliationMu.Unlock()
	if _, stopped := sm.stoppedReconciliations[identity.ExecutionID]; stopped {
		return nil, false
	}
	if existing := sm.deliveryReconciliations[identity.ExecutionID]; existing != nil {
		if existing.identity == identity {
			existing.waiters++
			return existing, true
		}
		existing.cancel()
	}
	cycleCtx, cancel := context.WithCancel(ctx)
	var stopCancel func() bool
	if sm.reconciliationContext != nil {
		stopCancel = context.AfterFunc(sm.reconciliationContext, cancel)
	}
	cycle := &deliveryReconciliationCycle{
		identity: identity,
		ctx:      cycleCtx,
		done:     make(chan struct{}),
		cancel: func() {
			if stopCancel != nil {
				stopCancel()
			}
			cancel()
		},
	}
	sm.deliveryReconciliations[identity.ExecutionID] = cycle
	return cycle, false
}

func (sm *StreamManager) waitDeliveryReconciliation(
	ctx context.Context,
	cycle *deliveryReconciliationCycle,
) DeliveryReconciliationResult {
	select {
	case <-cycle.done:
		return cycle.result
	case <-ctx.Done():
		return DeliveryReconciliationResult{
			Identity: cycle.identity, Outcome: DeliveryReconciliationTransportUnavailable, Err: ctx.Err(),
		}
	}
}

func (sm *StreamManager) cancelAllDeliveryReconciliations() {
	sm.reconciliationMu.Lock()
	for _, cycle := range sm.deliveryReconciliations {
		cycle.cancel()
	}
	sm.reconciliationMu.Unlock()
}

func (sm *StreamManager) finishDeliveryReconciliation(
	cycle *deliveryReconciliationCycle,
	result DeliveryReconciliationResult,
) {
	sm.reconciliationMu.Lock()
	cycle.result = result
	if sm.deliveryReconciliations[cycle.identity.ExecutionID] == cycle {
		delete(sm.deliveryReconciliations, cycle.identity.ExecutionID)
	}
	close(cycle.done)
	sm.reconciliationMu.Unlock()
	cycle.cancel()
}

func (sm *StreamManager) cancelDeliveryReconciliation(executionID string) {
	sm.reconciliationMu.Lock()
	if sm.stoppedReconciliations == nil {
		sm.stoppedReconciliations = make(map[string]struct{})
	}
	sm.stoppedReconciliations[executionID] = struct{}{}
	cycle := sm.deliveryReconciliations[executionID]
	if cycle != nil {
		cycle.cancel()
	}
	sm.reconciliationMu.Unlock()
}

func (sm *StreamManager) runDeliveryReconciliation(
	ctx context.Context,
	execution *AgentExecution,
	identity DeliveryReconciliationIdentity,
	peer deliveryReconciliationPeer,
) DeliveryReconciliationResult {
	result := DeliveryReconciliationResult{Identity: identity, Outcome: DeliveryReconciliationUncertain}
	sm.notifyDeliveryReconciliationPhase(execution, identity, DeliveryReconciliationPhaseReconnecting)
	now := sm.reconciliationNow
	if now == nil {
		now = time.Now
	}
	wait := sm.reconciliationWait
	if wait == nil {
		wait = waitForReconciliation
	}
	deadline := now().Add(deliveryReconciliationWindow)
	for attempt := 1; attempt <= deliveryReconciliationAttemptLimit; attempt++ {
		result.Attempts = attempt
		outcome, retry, attempted, err := sm.runDeliveryReconciliationAttempt(ctx, execution, identity, peer, deadline, now)
		if outcome != "" {
			result.Outcome, result.Err = outcome, err
			if outcome == DeliveryReconciliationRunningAttached {
				sm.notifyDeliveryReconciliationPhase(execution, identity, DeliveryReconciliationPhaseRecovered)
			}
			return result
		}
		if attempted {
			result.Err = err
		}
		if !retry {
			break
		}
		if attempt < deliveryReconciliationAttemptLimit {
			waitDuration := time.Duration(attempt) * time.Second
			if waitDuration > deadline.Sub(now()) {
				waitDuration = deadline.Sub(now())
			}
			if waitDuration > 0 {
				if err := wait(ctx, waitDuration); err != nil {
					result.Outcome, result.Err = DeliveryReconciliationTransportUnavailable, err
					return result
				}
			}
		}
	}
	if result.Err == nil {
		result.Err = ErrUncertainPromptDelivery
	}
	if isDeliveryTransportError(result.Err) {
		result.Outcome = DeliveryReconciliationTransportUnavailable
	}
	sm.notifyDeliveryReconciliationPhase(execution, identity, DeliveryReconciliationPhaseUncertain)
	return result
}

func (sm *StreamManager) runDeliveryReconciliationAttempt(
	ctx context.Context,
	execution *AgentExecution,
	identity DeliveryReconciliationIdentity,
	peer deliveryReconciliationPeer,
	deadline time.Time,
	now func() time.Time,
) (DeliveryReconciliationOutcome, bool, bool, error) {
	if err := ctx.Err(); err != nil {
		return DeliveryReconciliationTransportUnavailable, false, false, err
	}
	if !sm.deliveryReconciliationCurrent(execution, identity) {
		return DeliveryReconciliationOwnerMismatch, false, false, ErrDeliveryOwnerMismatch
	}
	remaining := deadline.Sub(now())
	if remaining <= 0 {
		return "", false, false, nil
	}
	attemptCtx, cancel := context.WithTimeout(ctx, minDuration(remaining, deliveryReconciliationAttemptCap))
	outcome, err := sm.probeDeliveryOwner(attemptCtx, execution, identity, peer)
	cancel()
	if !sm.deliveryReconciliationCurrent(execution, identity) {
		return DeliveryReconciliationOwnerMismatch, false, true, ErrDeliveryOwnerMismatch
	}
	return outcome, outcome == "", true, err
}

func isDeliveryTransportError(err error) bool {
	var networkErr interface{ Timeout() bool }
	return errors.Is(err, context.DeadlineExceeded) || errors.As(err, &networkErr)
}

func (sm *StreamManager) probeDeliveryOwner(
	ctx context.Context,
	execution *AgentExecution,
	identity DeliveryReconciliationIdentity,
	peer deliveryReconciliationPeer,
) (DeliveryReconciliationOutcome, error) {
	status, err := peer.GetStatus(ctx)
	if err != nil {
		return "", err
	}
	if status == nil {
		return "", errors.New("agentctl status response is empty")
	}
	descriptor, err := peer.GetDeliveryStatus(ctx, identity.StreamID)
	if err != nil {
		return "", err
	}
	submission, err := peer.GetDeliverySubmission(ctx, identity.SubmissionID)
	if err != nil {
		return "", err
	}
	if !matchesDeliveryReconciliationOwner(identity, descriptor, submission) {
		return DeliveryReconciliationOwnerMismatch, ErrDeliveryOwnerMismatch
	}
	if submissionHasRetainedTerminal(submission) {
		return sm.reconcileTerminalDelivery(ctx, execution, identity, submission, peer)
	}
	if !status.IsAgentRunning() {
		return "", fmt.Errorf("agent process is not running: %s", status.AgentStatus)
	}
	if !peer.HasAgentStream() {
		if err := peer.AttachAgentStream(ctx); err != nil {
			return "", err
		}
	}
	if !peer.HasAgentStream() {
		return "", ErrDeliveryTransportUnavailable
	}
	return DeliveryReconciliationRunningAttached, nil
}

func (sm *StreamManager) reconcileTerminalDelivery(
	ctx context.Context,
	execution *AgentExecution,
	identity DeliveryReconciliationIdentity,
	submission *journal.Submission,
	peer deliveryReconciliationPeer,
) (DeliveryReconciliationOutcome, error) {
	settled, err := sm.settleDeliverySubmission(ctx, execution, identity, submission)
	if err != nil {
		return "", err
	}
	if settled {
		return DeliveryReconciliationTerminalSettled, nil
	}
	if peer.HasAgentStream() {
		return "", ErrUncertainPromptDelivery
	}
	if err := peer.AttachAgentStream(ctx); err != nil {
		return "", err
	}
	return "", ErrUncertainPromptDelivery
}

func matchesDeliveryReconciliationOwner(
	identity DeliveryReconciliationIdentity,
	descriptor *agentctl.DeliveryStatus,
	submission *journal.Submission,
) bool {
	return descriptor != nil && descriptor.Durable &&
		descriptor.SessionID == identity.SessionID && descriptor.IncarnationID == identity.IncarnationID &&
		descriptor.HarnessGeneration == identity.HarnessGeneration && descriptor.StreamID == identity.StreamID &&
		submission != nil && submission.ID == identity.SubmissionID && submission.StreamID == identity.StreamID &&
		submission.SessionID == identity.SessionID && submission.IncarnationID == identity.IncarnationID &&
		submission.HarnessGeneration == identity.HarnessGeneration
}

func submissionHasRetainedTerminal(submission *journal.Submission) bool {
	return submission != nil && submission.TerminalEventRetained && submission.TerminalSequence > 0 &&
		(submission.State == journal.SubmissionCompleted || submission.State == journal.SubmissionFailed ||
			submission.State == journal.SubmissionCancelled)
}

func (sm *StreamManager) deliveryReconciliationCurrent(
	execution *AgentExecution,
	identity DeliveryReconciliationIdentity,
) bool {
	if execution == nil || execution.ID != identity.ExecutionID || execution.SessionID != identity.SessionID ||
		execution.Owner != identity.Owner || execution.DeliveryIncarnationID != identity.IncarnationID ||
		execution.DeliveryHarnessGeneration != identity.HarnessGeneration || execution.DeliveryStreamID != identity.StreamID ||
		execution.runtimeEpoch != identity.RuntimeEpoch || execution.startupAttemptSnapshot() != identity.StartupGeneration ||
		execution.promptGenerationSnapshot() != identity.PromptGeneration ||
		execution.deliverySubmissionIDSnapshot() != identity.SubmissionID {
		return false
	}
	return sm.isExecutionCurrent == nil || sm.isExecutionCurrent(execution)
}

func (sm *StreamManager) notifyDeliveryReconciliationPhase(
	execution *AgentExecution,
	identity DeliveryReconciliationIdentity,
	phase DeliveryReconciliationPhase,
) {
	if sm.onDeliveryReconciliationPhase != nil && sm.deliveryReconciliationCurrent(execution, identity) {
		sm.onDeliveryReconciliationPhase(execution, identity, phase)
	}
}

func (sm *StreamManager) settleDeliverySubmission(
	ctx context.Context,
	execution *AgentExecution,
	identity DeliveryReconciliationIdentity,
	submission *journal.Submission,
) (bool, error) {
	if sm.deliverySubmissionSettler == nil {
		return false, nil
	}
	return sm.deliverySubmissionSettler(ctx, execution, identity, submission)
}

func (sm *StreamManager) attachUpdatesStream(
	ctx context.Context,
	execution *AgentExecution,
	client *agentctl.Client,
) error {
	if client.HasAgentStream() {
		return nil
	}
	current, release := execution.AcquireAgentCtlClient()
	if current == nil || current != client {
		release()
		return ErrDeliveryOwnerMismatch
	}
	release()
	ready := make(chan struct{})
	sm.connectUpdatesStreamAsync(execution, ready)
	select {
	case <-ready:
	case <-ctx.Done():
		return ctx.Err()
	}
	if !client.HasAgentStream() {
		return ErrDeliveryTransportUnavailable
	}
	return nil
}

func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}
