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
	return false
}
