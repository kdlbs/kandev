package lifecycle

import (
	"context"
	"sync/atomic"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/types"
	"go.uber.org/zap"
)

const (
	defaultDeliveryHealthProbeInterval = 2 * time.Second
	defaultDeliveryHealthProbeTimeout  = 2 * time.Second
	defaultDeliveryHealthCooldown      = 10 * time.Second
)

// deliveryStreamAttachment fences one attached updates-stream connection. A
// later connection for the same execution replaces it, so an old monitor can
// never act on a successor owner.
type deliveryStreamAttachment struct {
	closed atomic.Bool
	cancel context.CancelFunc
}

func (a *deliveryStreamAttachment) close() {
	if a == nil {
		return
	}
	a.closed.Store(true)
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *deliveryStreamAttachment) isClosed() bool {
	return a == nil || a.closed.Load()
}

// deliveryHealthOwner is the narrow surface the monitor needs from one
// attached durable stream owner. It never holds execution, stream, prompt, or
// manager locks across the probe request.
type deliveryHealthOwner struct {
	execution        *AgentExecution
	probe            func(context.Context) (*agentctl.StatusResponse, error)
	current          func() bool
	reconcile        func(context.Context) DeliveryReconciliationResult
	submission       func() string
	promptGeneration func() uint64
}

// monitorDeliveryHealth observes authenticated delivery health independently
// of output arrival, browser focus, or the stall notice. A correlated failure
// enters the existing reconciliation path immediately.
func (sm *StreamManager) monitorDeliveryHealth(ctx context.Context, owner deliveryHealthOwner) {
	interval := sm.deliveryHealthProbeInterval
	if interval <= 0 {
		interval = defaultDeliveryHealthProbeInterval
	}
	timeout := sm.deliveryHealthProbeTimeout
	if timeout <= 0 {
		timeout = defaultDeliveryHealthProbeTimeout
	}
	cooldown := sm.deliveryHealthRecoveryCooldown
	if cooldown <= 0 {
		cooldown = defaultDeliveryHealthCooldown
	}
	var lastRevision uint64
	var nextAllowed time.Time
	first := true
	for {
		if ctx.Err() != nil {
			return
		}
		if !first && !sm.waitDeliveryHealth(ctx, interval) {
			return
		}
		first = false
		if !owner.current() {
			return
		}
		sm.probeDeliveryHealth(ctx, owner, timeout, cooldown, &lastRevision, &nextAllowed)
	}
}

// startDeliveryHealthMonitor owns one monitor for the current connection. A
// replacement connection cancels the predecessor before the new monitor
// starts, so a stale owner can never change successor state.
func (sm *StreamManager) startDeliveryHealthMonitor(
	ctx context.Context,
	cancel context.CancelFunc,
	execution *AgentExecution,
	client *agentctl.Client,
) *deliveryStreamAttachment {
	attachment := &deliveryStreamAttachment{cancel: cancel}
	if previous, loaded := sm.deliveryAttachments.LoadAndDelete(execution.ID); loaded {
		previous.(*deliveryStreamAttachment).close()
	}
	sm.deliveryAttachments.Store(execution.ID, attachment)
	sm.logger.Debug("delivery health monitor attached",
		zap.String("execution_id", execution.ID),
		zap.String("stream_id", execution.DeliveryStreamID))
	if !sm.start(func() {
		defer sm.releaseDeliveryAttachment(execution.ID, attachment)
		sm.monitorDeliveryHealth(ctx, sm.deliveryHealthOwnerFor(execution, client, attachment))
	}) {
		attachment.close()
		sm.releaseDeliveryAttachment(execution.ID, attachment)
	}
	return attachment
}

func (sm *StreamManager) releaseDeliveryAttachment(executionID string, attachment *deliveryStreamAttachment) {
	if current, ok := sm.deliveryAttachments.Load(executionID); ok && current == attachment {
		sm.deliveryAttachments.Delete(executionID)
	}
}

// probeDeliveryHealth performs one bounded, non-overlapping status request and
// enters reconciliation only for a correlated failure of the current owner.
func (sm *StreamManager) probeDeliveryHealth(
	ctx context.Context,
	owner deliveryHealthOwner,
	timeout, cooldown time.Duration,
	lastRevision *uint64,
	nextAllowed *time.Time,
) {
	submissionID := owner.submission()
	promptGeneration := owner.promptGeneration()
	if submissionID == "" {
		return
	}
	probeCtx, cancel := context.WithTimeout(ctx, timeout)
	status, err := owner.probe(probeCtx)
	cancel()
	if err != nil || status == nil {
		sm.logger.Debug("delivery health probe unavailable",
			zap.String("execution_id", owner.execution.ID), zap.Error(err))
		return
	}
	if status.DeliveryHealth.Failure == nil {
		if *lastRevision != 0 {
			// A prior failure is no longer reported, either because the actual
			// terminal committed or the producer recovered. Re-run the existing
			// reconciler so it can publish recovery and clear the durable fence.
			*lastRevision = 0
			result := owner.reconcile(ctx)
			sm.logger.Info("durable delivery failure cleared",
				zap.String("execution_id", owner.execution.ID),
				zap.String("outcome", string(result.Outcome)))
		}
		return
	}
	failure := status.DeliveryHealth.Failure
	sm.logger.Debug("delivery health failure observed",
		zap.String("execution_id", owner.execution.ID),
		zap.String("failure_submission_id", failure.SubmissionID),
		zap.String("current_submission_id", submissionID),
		zap.String("reason", failure.Reason),
		zap.Uint64("revision", failure.Revision))
	if !deliveryFailureCorrelates(owner, failure, submissionID, promptGeneration) {
		sm.logger.Debug("delivery health failure did not correlate with the current owner",
			zap.String("execution_id", owner.execution.ID),
			zap.String("failure_submission_id", failure.SubmissionID),
			zap.String("current_submission_id", submissionID),
			zap.String("failure_stream_id", failure.StreamID),
			zap.String("current_stream_id", owner.execution.DeliveryStreamID),
			zap.Uint64("failure_generation", failure.HarnessGeneration),
			zap.Uint64("current_generation", owner.execution.DeliveryHarnessGeneration),
			zap.Uint64("failure_prompt_generation", failure.PromptGeneration),
			zap.Uint64("current_prompt_generation", promptGeneration))
		return
	}
	if !owner.current() || owner.submission() != submissionID {
		return
	}
	now := sm.deliveryHealthNow()
	if failure.Revision == *lastRevision && now.Before(*nextAllowed) {
		return
	}
	*lastRevision = failure.Revision
	*nextAllowed = now.Add(cooldown)
	result := owner.reconcile(ctx)
	sm.logger.Warn("observed durable delivery failure",
		zap.String("execution_id", owner.execution.ID),
		zap.String("submission_id", failure.SubmissionID),
		zap.String("reason", failure.Reason),
		zap.Uint64("revision", failure.Revision),
		zap.String("outcome", string(result.Outcome)))
}

func deliveryFailureCorrelates(
	owner deliveryHealthOwner,
	failure *types.DeliveryFailureSnapshot,
	submissionID string,
	promptGeneration uint64,
) bool {
	execution := owner.execution
	if execution == nil || failure == nil || failure.SubmissionID == "" {
		return false
	}
	if failure.SessionID != execution.SessionID || failure.IncarnationID != execution.DeliveryIncarnationID ||
		failure.HarnessGeneration != execution.DeliveryHarnessGeneration || failure.StreamID != execution.DeliveryStreamID {
		return false
	}
	if failure.SubmissionID != submissionID {
		return false
	}
	if failure.PromptGeneration != 0 && failure.PromptGeneration != promptGeneration {
		return false
	}
	return true
}

func (sm *StreamManager) deliveryHealthOwnerFor(
	execution *AgentExecution,
	client *agentctl.Client,
	attachment *deliveryStreamAttachment,
) deliveryHealthOwner {
	return deliveryHealthOwner{
		execution: execution,
		probe:     client.GetStatus,
		current: func() bool {
			if execution == nil || attachment.isClosed() {
				return false
			}
			if current, ok := sm.deliveryAttachments.Load(execution.ID); !ok || current != attachment {
				return false
			}
			currentClient, release := execution.AcquireAgentCtlClient()
			if currentClient == nil || currentClient != client {
				release()
				return false
			}
			release()
			return sm.isExecutionCurrent == nil || sm.isExecutionCurrent(execution)
		},
		reconcile: func(ctx context.Context) DeliveryReconciliationResult {
			return sm.ReconcileAgentDelivery(ctx, execution)
		},
		submission:       func() string { return currentDeliverySubmissionID(execution) },
		promptGeneration: execution.promptGenerationSnapshot,
	}
}

func (sm *StreamManager) deliveryHealthNow() time.Time {
	if sm.deliveryHealthClock != nil {
		return sm.deliveryHealthClock()
	}
	return time.Now()
}

// waitDeliveryHealth returns true when the probe interval elapsed, and false
// when the owner, manager shutdown, or stream connection ended.
func (sm *StreamManager) waitDeliveryHealth(ctx context.Context, interval time.Duration) bool {
	if sm.deliveryHealthWait != nil {
		return sm.deliveryHealthWait(ctx, interval) == nil
	}
	timer := time.NewTimer(interval)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	case <-sm.waitCh:
		return false
	case <-sm.stopCh:
		return false
	}
}
