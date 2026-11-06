package messagequeue

import (
	"context"
	"fmt"
	"strings"
)

const (
	MaxPeerReportBatchEntries = 8
	MaxPeerReportBatchBytes   = 256 * 1024
	metadataPeerReportBatch   = "peer_report_batch"
)

// IsOrdinaryPeerReport excludes instructions and deliveries whose individual
// reservation, references, or executor submission must remain separable.
func IsOrdinaryPeerReport(message *QueuedMessage) bool {
	if message == nil || message.QueuedBy != QueuedByAgent || PeerReportSenderTaskID(message) == "" {
		return false
	}
	if message.IsDurableDelivery() || message.IsReservedInFlight() || message.IsReservedDelivery() ||
		len(message.Attachments) > 0 || hasBatchSeparableMetadata(message) {
		return false
	}
	if _, ok := normalizeQueueAdmissionIDs(message.Metadata[MetadataQueueAdmissionIDs]); !ok {
		return false
	}
	protocol, submissionID, payloadHash := message.DeliverySubmission()
	if protocol == "" {
		return submissionID == "" && payloadHash == ""
	}
	return protocol == DeliveryProtocolPending && message.dispatchAttemptID != "" &&
		submissionID == "queue:"+message.dispatchAttemptID && payloadHash == ""
}

func hasBatchSeparableMetadata(message *QueuedMessage) bool {
	if composed, _ := message.Metadata[metadataPeerReportBatch].(bool); composed {
		return true
	}
	for _, key := range []string{MetadataEntityReferences, MetadataContextFiles} {
		if value, ok := message.Metadata[key]; ok && value != nil {
			return true
		}
	}
	return false
}

func PeerReportSenderTaskID(message *QueuedMessage) string {
	if message == nil {
		return ""
	}
	return metadataString(message.Metadata, MetadataSenderTaskID)
}

// ComposePeerReportBatch retains the leading reservation and each report's
// sender label. Existing queue admission provenance follows the combined row.
func ComposePeerReportBatch(batch []*QueuedMessage) *QueuedMessage {
	if len(batch) == 0 {
		return nil
	}
	leader := *batch[0]
	if len(batch) == 1 {
		return &leader
	}
	leader.Metadata = make(map[string]interface{}, len(batch[0].Metadata))
	for key, value := range batch[0].Metadata {
		leader.Metadata[key] = value
	}
	leader.Metadata[metadataPeerReportBatch] = true
	var composed strings.Builder
	for index, entry := range batch {
		if index > 0 {
			composed.WriteString("\n\n")
		}
		fmt.Fprintf(&composed, "[peer report %d/%d] from task %s\n%s",
			index+1, len(batch), PeerReportSenderTaskID(entry), entry.Content)
		if admissionIDs, ok := unionQueueAdmissionIDs(leader.Metadata, entry.Metadata); ok && len(admissionIDs) > 0 {
			leader.Metadata[MetadataQueueAdmissionIDs] = admissionIDs
		}
	}
	leader.Content = composed.String()
	return &leader
}

// Keep this capability separate: adding it to pendingQueueDispatchRepository
// would silently disable existing acceptance/acknowledgement implementations.
type peerReportBatchRepository interface {
	FoldPeerReportBatch(context.Context, QueueSessionIdentity, *QueuedMessage, []*QueuedMessage) error
}

func (s *Service) peerReportBatchSupported() bool {
	if !s.PendingQueueDispatchPersistenceAvailable() {
		return true
	}
	_, ok := s.repo.(peerReportBatchRepository)
	return ok
}

// FoldPeerReportBatch transfers fresh folded claims to the leading claim in
// one transaction. Executor-bound or unknown claims are never rewritten.
func (s *Service) FoldPeerReportBatch(
	ctx context.Context, identity QueueSessionIdentity, composed *QueuedMessage, batch []*QueuedMessage,
) error {
	return s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		if err := s.validateSessionIdentity(admittedCtx, identity); err != nil {
			return err
		}
		if repo, ok := s.repo.(peerReportBatchRepository); ok {
			return repo.FoldPeerReportBatch(admittedCtx, identity, composed, batch)
		}
		if s.PendingQueueDispatchPersistenceAvailable() {
			return ErrQueueDispatchClaimChanged
		}
		return nil
	})
}

// ReservePeerReportBatchWithAutoRunForSession uses the ordinary reserve path;
// durable, retained, leased, or incompatible entries remain FIFO boundaries.
// Partial failure returns the reservations that the caller still owns.
func (s *Service) ReservePeerReportBatchWithAutoRunForSession(
	ctx context.Context, identity QueueSessionIdentity,
) ([]*QueuedMessage, bool, error) {
	var batch []*QueuedMessage
	autoRun := true
	err := s.WithSessionAdmission(ctx, identity.SessionID, func(admittedCtx context.Context) error {
		var err error
		batch, autoRun, err = s.reservePeerReportBatch(admittedCtx, identity)
		return err
	})
	return batch, autoRun, err
}

func (s *Service) reservePeerReportBatch(
	ctx context.Context, identity QueueSessionIdentity,
) ([]*QueuedMessage, bool, error) {
	batch := make([]*QueuedMessage, 0, MaxPeerReportBatchEntries)
	batchBytes := 0
	for len(batch) < MaxPeerReportBatchEntries {
		message, autoRun, err := s.repo.ReserveHeadIfAutoRunForSession(ctx, identity)
		if err != nil {
			return batch, true, err
		}
		if !autoRun || message == nil {
			return batch, autoRun, nil
		}
		if len(batch) > 0 && message.ID == batch[len(batch)-1].ID {
			return batch, true, nil // A retained durable head remains reserved.
		}
		if len(batch) > 0 && !peerReportFitsBatch(batch[0], message, batchBytes) {
			if err := s.RequeueAtHeadForSession(ctx, identity, message); err != nil {
				return append(batch, message), true, err
			}
			return batch, true, nil
		}
		batch = append(batch, message)
		if !IsOrdinaryPeerReport(message) || !s.peerReportBatchSupported() {
			return batch, true, nil
		}
		batchBytes += len(message.Content)
	}
	return batch, true, nil
}

func peerReportFitsBatch(leader, message *QueuedMessage, batchBytes int) bool {
	return IsOrdinaryPeerReport(message) && message.Model == leader.Model && message.PlanMode == leader.PlanMode &&
		batchBytes+len(message.Content) <= MaxPeerReportBatchBytes
}
