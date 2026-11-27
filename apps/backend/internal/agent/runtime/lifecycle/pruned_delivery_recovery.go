package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

func (m *Manager) recoverPrunedDelivery(ctx context.Context, lease *agentctl.RuntimeLease, control *agentctl.ControlClient, requested AgentDeliveryRecoveryIdentity, identity DeliveryReconciliationIdentity, cursor *models.AgentDeliveryCursor, request journal.RetainedRecoveryRequest) DeliveryReconciliationResult {
	fail := func(reason string, err error) DeliveryReconciliationResult {
		return deliveryRecoveryResult(identity, DeliveryReconciliationBlocked, reason, err)
	}
	gap := requested.HistoryGap
	var received, projected int64
	if cursor != nil {
		received, projected = cursor.ReceivedSequence, cursor.ProjectedSequence
	}
	if gap == nil || gap.ReceivedSequence != received || gap.ProjectedSequence != projected ||
		requested.PayloadHash == "" || gap.FirstSequence != received+1 || gap.LastSequence <= received {
		return fail("delivery_identity_mismatch", ErrDeliveryOwnerMismatch)
	}
	if err := lease.CheckCurrent(); err != nil {
		return fail("runtime_owner_changed", err)
	}
	request.PrunedThrough = uint64(gap.LastSequence)
	request.ExpectedHash = requested.PayloadHash
	evidence, err := control.ReadRetainedDelivery(ctx, request)
	if err != nil {
		return fail("delivery_history_gap", err)
	}
	if !evidence.HistoryGapVerified {
		return fail("delivery_gap_recovery_unsupported", ErrDeliveryRecoveryBlocked)
	}
	if !validPrunedDeliveryEvidence(evidence, identity, requested, received, projected) {
		return fail("delivery_identity_mismatch", ErrDeliveryOwnerMismatch)
	}
	if err := lease.CheckCurrent(); err != nil {
		return fail("runtime_owner_changed", err)
	}
	result := deliveryRecoveryResult(identity, DeliveryReconciliationUncertain, "delivery_history_gap", ErrUncertainPromptDelivery)
	result.ProcessTerminated = true
	return result
}

func validPrunedDeliveryEvidence(evidence *journal.RetainedRecoveryResponse, identity DeliveryReconciliationIdentity, requested AgentDeliveryRecoveryIdentity, received, projected int64) bool {
	stream := evidence.Descriptor.Stream
	if !evidence.ProcessTerminated || len(evidence.Events) != 0 || stream == nil ||
		!matchesDeliveryReconciliationOwner(identity, &evidence.Descriptor, &evidence.Submission) {
		return false
	}
	return evidence.Submission.State == journal.SubmissionInterruptedUnknown && evidence.Submission.Hash == requested.PayloadHash &&
		stream.HighWater <= 1<<63-1 && stream.FirstRetained <= 1<<63-1 &&
		requested.HistoryGap.MatchesPrunedStream(received, projected, int64(stream.HighWater), int64(stream.Acknowledged), int64(stream.FirstRetained))
}
