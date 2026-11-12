package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

// SQL retains retired unknown submissions. A restored snapshot becomes
// historical only when the authenticated journal has distinct unsettled work.
func (s *Service) hasNewRestartJournalSubmission(ctx context.Context, session *models.TaskSession, generation *models.HarnessSessionGeneration, previousSubmissionID string) bool {
	inspector, supported := s.agentManager.(deliveryRecordEvidenceClient)
	if !supported {
		return false
	}
	evidence, err := inspector.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{TaskID: session.TaskID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID, HarnessGeneration: uint64(generation.Generation)})
	if err != nil || evidence == nil {
		return false
	}
	descriptor := evidence.Descriptor
	if !descriptor.Durable || descriptor.SubmissionsTruncated || descriptor.SessionID != session.ID || descriptor.IncarnationID != session.QueueIncarnationID || descriptor.HarnessGeneration != uint64(generation.Generation) {
		return false
	}
	for _, submission := range descriptor.Submissions {
		if submission.ID != previousSubmissionID &&
			(submission.State == journal.SubmissionDispatching || submission.State == journal.SubmissionInterruptedUnknown) {
			return true
		}
	}
	reader, ok := s.repo.(nativeResumeDeliverySubmissionReader)
	if !ok {
		return false
	}
	submissions, err := reader.ListAgentDeliverySubmissions(ctx, session.ID)
	if err != nil {
		return false
	}
	for _, submission := range submissions {
		if submission.ID == previousSubmissionID || submission.IncarnationID != session.QueueIncarnationID ||
			submission.HarnessGeneration != generation.Generation || terminalDeliverySubmission(submission.State) {
			continue
		}
		stored, err := inspector.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
			TaskID: session.TaskID, SessionID: session.ID, IncarnationID: session.QueueIncarnationID,
			HarnessGeneration: uint64(generation.Generation), StoredSubmissionID: submission.ID,
		})
		if err != nil || stored == nil || !restartStoredSubmissionMatches(stored, submission, descriptor) {
			continue
		}
		return true
	}
	return false
}

func restartStoredSubmissionMatches(
	evidence *agentruntime.DeliveryRecordEvidence,
	submission *models.AgentDeliverySubmission,
	owner journal.RecoveryDescriptor,
) bool {
	if !restartStoredSubmissionIsRetiredUnknown(evidence, submission) {
		return false
	}
	stored := evidence.StoredSubmission
	return restartStoredSubmissionOwnerMatches(owner, submission) &&
		restartStoredSubmissionMatchesCanonical(stored, submission)
}

func restartStoredSubmissionIsRetiredUnknown(
	evidence *agentruntime.DeliveryRecordEvidence,
	submission *models.AgentDeliverySubmission,
) bool {
	return evidence != nil && submission != nil && evidence.StoredSubmission != nil &&
		evidence.StoredSubmission.Retired && journal.HasUnprovenOutcome(*evidence.StoredSubmission)
}

func restartStoredSubmissionOwnerMatches(owner journal.RecoveryDescriptor, submission *models.AgentDeliverySubmission) bool {
	return owner.Durable && !owner.SubmissionsTruncated && owner.SessionID == submission.SessionID &&
		owner.IncarnationID == submission.IncarnationID &&
		owner.HarnessGeneration == uint64(submission.HarnessGeneration)
}

func restartStoredSubmissionMatchesCanonical(stored *journal.Submission, submission *models.AgentDeliverySubmission) bool {
	return stored.ID == submission.ID && stored.SessionID == submission.SessionID &&
		stored.IncarnationID == submission.IncarnationID &&
		stored.HarnessGeneration == uint64(submission.HarnessGeneration) && stored.Hash != "" &&
		stored.Hash == submission.PayloadHash && stored.StreamID != "" &&
		restartSubmissionStatesMatch(submission.State, stored.State)
}
