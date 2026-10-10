package lifecycle

import (
	"context"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
)

func (m *Manager) inspectDeliveryRecordTransport(ctx context.Context, lease *agentctl.RuntimeLease, control *agentctl.ControlClient, instance *agentctl.InstanceInfo, request DeliveryRecordEvidenceRequest) (*DeliveryRecordEvidence, error) {
	if instance == nil {
		return m.inspectRetainedDeliveryRecord(ctx, lease, control, request)
	}
	return m.inspectLiveDeliveryRecord(ctx, lease, instance, request)
}

func (m *Manager) inspectRetainedDeliveryRecord(ctx context.Context, lease *agentctl.RuntimeLease, control *agentctl.ControlClient, request DeliveryRecordEvidenceRequest) (*DeliveryRecordEvidence, error) {
	retained, readErr := control.ReadRetainedReconstructionEvidence(ctx, journal.RetainedReconstructionEvidenceRequest{
		Root: m.dataDir, SessionID: request.SessionID, IncarnationID: request.IncarnationID,
		HarnessGeneration: request.HarnessGeneration, OriginalRuntime: request.OriginalRuntime,
	})
	if readErr != nil {
		return nil, deliveryEvidenceReadError(lease, readErr)
	}
	return &DeliveryRecordEvidence{
		Descriptor: retained.Descriptor, ProcessTerminated: retained.ProcessTerminated,
	}, nil
}

func (m *Manager) inspectLiveDeliveryRecord(ctx context.Context, lease *agentctl.RuntimeLease, instance *agentctl.InstanceInfo, request DeliveryRecordEvidenceRequest) (*DeliveryRecordEvidence, error) {

	if !instance.ListenerActive || instance.Port <= 0 {
		return nil, ErrDeliveryRecoveryBlocked
	}
	client := lease.NewBoundInstanceClient(instance.Port, m.logger)
	if client == nil {
		return nil, ErrDeliveryTransportUnavailable
	}
	defer client.Close()
	descriptor, readErr := client.GetDeliveryStatus(ctx, "")
	if readErr != nil {
		return nil, deliveryEvidenceReadError(lease, readErr)
	}
	return &DeliveryRecordEvidence{
		Descriptor: *descriptor, LiveExecutionID: instance.ID,
		ProcessTerminated: historicalProcessTermination(request, instance.ID),
	}, nil
}

func validDeliveryRecordSubmissionRequest(request DeliveryRecordSubmissionRequest) bool {
	return request.TaskID != "" && request.SessionID != "" && request.IncarnationID != "" &&
		request.HarnessGeneration != 0 && request.Candidate.ID != "" &&
		request.Candidate.SessionID == request.SessionID && request.Candidate.IncarnationID == request.IncarnationID &&
		request.Candidate.HarnessGeneration == request.HarnessGeneration
}

type deliveryRecordRead struct {
	before, after journal.RecoveryDescriptor
	submission    *journal.Submission
}

func (m *Manager) readDeliveryRecordTransport(ctx context.Context, lease *agentctl.RuntimeLease, control *agentctl.ControlClient, instance *agentctl.InstanceInfo, request DeliveryRecordSubmissionRequest) (*deliveryRecordRead, error) {
	if instance == nil {
		return m.readRetainedDeliveryRecord(ctx, lease, control, request)
	}
	return m.readLiveDeliveryRecord(ctx, lease, instance, request)
}

func (m *Manager) readRetainedDeliveryRecord(ctx context.Context, lease *agentctl.RuntimeLease, control *agentctl.ControlClient, request DeliveryRecordSubmissionRequest) (*deliveryRecordRead, error) {
	var before, after journal.RecoveryDescriptor
	var submission *journal.Submission
	var err error
	input := journal.RetainedReconstructionEvidenceRequest{
		Root: m.dataDir, SessionID: request.SessionID,
		IncarnationID: request.IncarnationID, HarnessGeneration: request.HarnessGeneration,
	}
	first, readErr := control.ReadRetainedReconstructionEvidence(ctx, input)
	if readErr != nil {
		return nil, deliveryEvidenceReadError(lease, readErr)
	}
	before = first.Descriptor
	if err = validateSelectedDeliveryRecord(before, request); err != nil {
		return nil, err
	}
	submission, err = control.ReadRetainedReconstructionSubmission(ctx, journal.RetainedReconstructionSubmissionRequest{
		Root: m.dataDir, SessionID: request.SessionID, IncarnationID: request.IncarnationID,
		HarnessGeneration: request.HarnessGeneration, Candidate: request.Candidate,
	})
	if err != nil {
		return nil, deliveryEvidenceReadError(lease, err)
	}
	second, readErr := control.ReadRetainedReconstructionEvidence(ctx, input)
	if readErr != nil {
		return nil, deliveryEvidenceReadError(lease, readErr)
	}
	after = second.Descriptor
	return &deliveryRecordRead{before: before, after: after, submission: submission}, nil
}

func (m *Manager) readLiveDeliveryRecord(ctx context.Context, lease *agentctl.RuntimeLease, instance *agentctl.InstanceInfo, request DeliveryRecordSubmissionRequest) (*deliveryRecordRead, error) {
	var before, after journal.RecoveryDescriptor
	var submission *journal.Submission
	var err error

	if !instance.ListenerActive || instance.Port <= 0 {
		return nil, ErrDeliveryRecoveryBlocked
	}
	client := lease.NewBoundInstanceClient(instance.Port, m.logger)
	if client == nil {
		return nil, ErrDeliveryTransportUnavailable
	}
	defer client.Close()
	first, readErr := client.GetDeliveryStatus(ctx, "")
	if readErr != nil {
		return nil, deliveryEvidenceReadError(lease, readErr)
	}
	before = *first
	if err = validateSelectedDeliveryRecord(before, request); err != nil {
		return nil, err
	}
	submission, err = client.GetDeliverySubmission(ctx, request.Candidate.ID)
	if err != nil {
		return nil, deliveryEvidenceReadError(lease, err)
	}
	if err = lease.CheckCurrent(); err != nil {
		return nil, err
	}
	second, readErr := client.GetDeliveryStatus(ctx, "")
	if readErr != nil {
		return nil, deliveryEvidenceReadError(lease, readErr)
	}
	after = *second
	return &deliveryRecordRead{before: before, after: after, submission: submission}, nil
}
