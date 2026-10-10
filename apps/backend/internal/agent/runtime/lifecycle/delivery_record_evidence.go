package lifecycle

import (
	"context"
	"errors"
	"strings"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/common/processidentity"
)

var (
	// ErrDeliveryRecordEvidenceTruncated means the bounded descriptor omitted candidates.
	ErrDeliveryRecordEvidenceTruncated = errors.New("delivery record evidence is truncated")
	// ErrDeliveryRecordEvidenceAmbiguous means more than one unresolved candidate exists.
	ErrDeliveryRecordEvidenceAmbiguous = errors.New("delivery record evidence is ambiguous")
	// ErrDeliveryRecordEvidenceStale means the selected owner snapshot changed during inspection.
	ErrDeliveryRecordEvidenceStale = errors.New("delivery record evidence changed")
	// ErrDeliveryRecordEvidenceConflict means retained bytes do not match their immutable identity.
	ErrDeliveryRecordEvidenceConflict = errors.New("delivery record evidence conflicts")
	// ErrDeliveryRecordEvidenceUnsupported means the peer lacks the inspection capability.
	ErrDeliveryRecordEvidenceUnsupported = errors.New("delivery record evidence inspection is unsupported")
)

// DeliveryRecordEvidenceRequest names the SQL-owned session generation and the
// historical execution whose process proof may be available. It contains no
// storage path and is not a browser-facing request.
type DeliveryRecordEvidenceRequest struct {
	TaskID            string
	SessionID         string
	ExecutionID       string
	IncarnationID     string
	HarnessGeneration uint64
	OriginalRuntime   *processidentity.Identity
}

// DeliveryRecordEvidence separates the live session instance from proof about
// the historical process that owned a candidate prompt.
type DeliveryRecordEvidence struct {
	Descriptor        journal.RecoveryDescriptor
	ProcessTerminated *bool
	LiveExecutionID   string
	ControlIdentity   *AgentDeliveryRecoveryIdentity
}

// DeliveryRecordSubmissionRequest selects the exact sole unresolved candidate
// from a previously inspected descriptor.
type DeliveryRecordSubmissionRequest struct {
	TaskID            string
	SessionID         string
	IncarnationID     string
	HarnessGeneration uint64
	Candidate         journal.SubmissionSummary
}

// DeliveryRecordSubmission is a selected immutable record fetched through the
// backend-only runtime operation.
type DeliveryRecordSubmission = journal.Submission

// InspectDeliveryRecordEvidence reads one owner snapshot without starting an
// instance, acknowledging output, or changing the journal.
func (m *Manager) InspectDeliveryRecordEvidence(
	ctx context.Context,
	request DeliveryRecordEvidenceRequest,
) (*DeliveryRecordEvidence, error) {
	if !validDeliveryRecordEvidenceRequest(request) {
		return nil, ErrDeliveryOwnerMismatch
	}
	lease, control, err := m.acquireDeliveryRecordEvidenceLease(ctx)
	if err != nil {
		return nil, err
	}
	defer lease.Close()
	instances, err := control.ListInstances(ctx)
	if err != nil {
		return nil, deliveryEvidenceReadError(lease, err)
	}
	instance, err := matchDeliveryRecordInstance(instances, request)
	if err != nil {
		return nil, err
	}
	result, err := m.inspectDeliveryRecordTransport(ctx, lease, control, instance, request)
	if err != nil {
		return nil, err
	}
	if err = validateDeliveryRecordDescriptor(result.Descriptor, request); err != nil {
		return nil, err
	}
	if result.Descriptor.SubmissionsTruncated {
		return result, errors.Join(ErrDeliveryRecordEvidenceTruncated, ErrDeliveryRecoveryBlocked)
	}
	result.ControlIdentity = m.deliveryRecordControlIdentity(request, result.Descriptor)
	if err = lease.CheckCurrent(); err != nil {
		return nil, err
	}
	return result, nil
}

// ReadDeliveryRecordSubmission fetches a uniquely selected payload, verifies
// its immutable identity, and rejects any descriptor change around the read.
func (m *Manager) ReadDeliveryRecordSubmission(
	ctx context.Context,
	request DeliveryRecordSubmissionRequest,
) (*DeliveryRecordSubmission, error) {
	if !validDeliveryRecordSubmissionRequest(request) {
		return nil, ErrDeliveryOwnerMismatch
	}

	lease, control, err := m.acquireDeliveryRecordEvidenceLease(ctx)
	if err != nil {
		return nil, err
	}
	defer lease.Close()
	instances, err := control.ListInstances(ctx)
	if err != nil {
		return nil, deliveryEvidenceReadError(lease, err)
	}
	instance, err := matchDeliveryRecordInstance(instances, DeliveryRecordEvidenceRequest{
		TaskID: request.TaskID, SessionID: request.SessionID,
		IncarnationID: request.IncarnationID, HarnessGeneration: request.HarnessGeneration,
	})
	if err != nil {
		return nil, err
	}

	read, err := m.readDeliveryRecordTransport(ctx, lease, control, instance, request)
	if err != nil {
		return nil, err
	}
	before, after, submission := read.before, read.after, read.submission
	if err = journal.VerifyReconstructionSubmission(*submission, request.Candidate); err != nil {
		if errors.Is(err, journal.ErrSubmissionConflict) || errors.Is(err, journal.ErrStreamFull) {
			return nil, errors.Join(ErrDeliveryRecordEvidenceConflict, ErrDeliveryRecoveryBlocked, err)
		}
		return nil, errors.Join(ErrDeliveryRecoveryBlocked, err)
	}
	if err = validateSelectedDeliveryRecord(after, request); err != nil {
		return nil, err
	}
	if !sameDeliveryRecordDescriptor(before, after) {
		return nil, errors.Join(ErrDeliveryRecordEvidenceStale, ErrDeliveryRecoveryBlocked)
	}
	if err = lease.CheckCurrent(); err != nil {
		return nil, err
	}
	return submission, nil
}

func validDeliveryRecordEvidenceRequest(request DeliveryRecordEvidenceRequest) bool {
	return request.TaskID != "" && request.SessionID != "" && request.IncarnationID != "" && request.HarnessGeneration > 0
}

func (m *Manager) acquireDeliveryRecordEvidenceLease(ctx context.Context) (*agentctl.RuntimeLease, *agentctl.ControlClient, error) {
	if m.runtimeOwner == nil || m.dataDir == "" {
		return nil, nil, ErrDeliveryTransportUnavailable
	}
	lease, err := m.runtimeOwner.Acquire(ctx)
	if err != nil {
		return nil, nil, errors.Join(ErrDeliveryTransportUnavailable, err)
	}
	control := lease.NewControlClient(m.logger)
	if control == nil {
		lease.Close()
		return nil, nil, ErrDeliveryTransportUnavailable
	}
	return lease, control, nil
}

func matchDeliveryRecordInstance(instances []*agentctl.InstanceInfo, request DeliveryRecordEvidenceRequest) (*agentctl.InstanceInfo, error) {
	var found *agentctl.InstanceInfo
	for _, instance := range instances {
		if instance == nil || instance.SessionID != request.SessionID {
			continue
		}
		if found != nil {
			return nil, ErrDeliveryOwnerMismatch
		}
		if instance.TaskID != request.TaskID {
			return nil, ErrDeliveryOwnerMismatch
		}
		found = instance
	}
	return found, nil
}

func validateDeliveryRecordDescriptor(descriptor journal.RecoveryDescriptor, request DeliveryRecordEvidenceRequest) error {
	if !descriptor.Durable || descriptor.Version != journal.CurrentVersion {
		return ErrDeliveryRecoveryBlocked
	}
	if descriptor.SessionID != request.SessionID || descriptor.IncarnationID != request.IncarnationID ||
		descriptor.HarnessGeneration != request.HarnessGeneration {
		return ErrDeliveryOwnerMismatch
	}
	return nil
}

func validateSelectedDeliveryRecord(descriptor journal.RecoveryDescriptor, request DeliveryRecordSubmissionRequest) error {
	if err := validateDeliveryRecordDescriptor(descriptor, DeliveryRecordEvidenceRequest{
		TaskID: request.TaskID, SessionID: request.SessionID,
		IncarnationID: request.IncarnationID, HarnessGeneration: request.HarnessGeneration,
	}); err != nil {
		return err
	}
	if descriptor.SubmissionsTruncated {
		return errors.Join(ErrDeliveryRecordEvidenceTruncated, ErrDeliveryRecoveryBlocked)
	}
	if descriptor.SubmissionCount != 1 || len(descriptor.Submissions) != 1 {
		return errors.Join(ErrDeliveryRecordEvidenceAmbiguous, ErrDeliveryRecoveryBlocked)
	}
	if !journal.SubmissionSummariesEqual(descriptor.Submissions[0], request.Candidate) {
		return errors.Join(ErrDeliveryRecordEvidenceStale, ErrDeliveryRecoveryBlocked)
	}
	return nil
}

func sameDeliveryRecordDescriptor(left, right journal.RecoveryDescriptor) bool {
	if left.Version != right.Version || left.Durable != right.Durable || left.SessionID != right.SessionID ||
		left.IncarnationID != right.IncarnationID || left.HarnessGeneration != right.HarnessGeneration ||
		left.SubmissionCount != right.SubmissionCount || left.SubmissionsTruncated != right.SubmissionsTruncated ||
		len(left.Submissions) != len(right.Submissions) {
		return false
	}
	for i := range left.Submissions {
		if !journal.SubmissionSummariesEqual(left.Submissions[i], right.Submissions[i]) {
			return false
		}
	}
	return true
}

func deliveryEvidenceReadError(lease *agentctl.RuntimeLease, err error) error {
	if leaseErr := lease.CheckCurrent(); leaseErr != nil {
		return leaseErr
	}
	var httpErr *agentctl.DeliveryHTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case 404:
			if strings.Contains(httpErr.Body, "SUBMISSION_NOT_FOUND") {
				return errors.Join(ErrDeliveryRecordEvidenceStale, ErrDeliveryRecoveryBlocked, err)
			}
			return errors.Join(ErrDeliveryRecordEvidenceUnsupported, ErrDeliveryRecoveryBlocked, err)
		case 409:
			return errors.Join(ErrDeliveryRecordEvidenceStale, ErrDeliveryRecoveryBlocked, err)
		case 413:
			return errors.Join(ErrDeliveryRecordEvidenceConflict, ErrDeliveryRecoveryBlocked, err)
		}
	}
	return err
}

func historicalProcessTermination(request DeliveryRecordEvidenceRequest, liveExecutionID string) *bool {
	if request.ExecutionID != "" && request.ExecutionID == liveExecutionID {
		terminated := false
		return &terminated
	}
	if request.OriginalRuntime == nil || request.OriginalRuntime.Validate() != nil {
		return nil
	}
	terminated, err := processidentity.OwnedSessionTerminated(*request.OriginalRuntime)
	if err != nil {
		return nil
	}
	return &terminated
}
