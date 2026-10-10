package lifecycle

import (
	"context"
	"fmt"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

type nativeResumePeerSubmissionLister interface {
	ListDeliverySubmissions(context.Context, string) ([]journal.Submission, error)
}

// acknowledgedNativeResumeHistory keeps uncertain SQL history out of active
// work only when the authenticated peer retains its exact retirement evidence.
func acknowledgedNativeResumeHistory(
	ctx context.Context,
	execution *AgentExecution,
	status *agentctl.DeliveryStatus,
	peerReader agentDeliveryPeerSubmissionReader,
	backend []*models.AgentDeliverySubmission,
) ([]*models.AgentDeliverySubmission, error) {
	lister, supported := peerReader.(nativeResumePeerSubmissionLister)
	if !supported || !hasUnlistedUnknownHistory(status, backend) {
		return backend, nil
	}
	submissions, err := lister.ListDeliverySubmissions(ctx, execution.SessionID)
	if err != nil {
		return nil, fmt.Errorf("%w: %w: list acknowledged history: %v", ErrDurableAdoptionBlocked, ErrDurableAdoptionEvidenceUnavailable, err)
	}
	retained := make(map[string]journal.Submission, len(submissions))
	for _, submission := range submissions {
		retained[submission.ID] = submission
	}
	active := make([]*models.AgentDeliverySubmission, 0, len(backend))
	for _, submission := range backend {
		peer, found := retained[submission.ID]
		if found && peerAcknowledgesUnknownHistory(execution, status, peer, submission) {
			continue
		}
		active = append(active, submission)
	}
	return active, nil
}

func hasUnlistedUnknownHistory(status *agentctl.DeliveryStatus, backend []*models.AgentDeliverySubmission) bool {
	listed := make(map[string]bool, len(status.Submissions))
	for _, submission := range status.Submissions {
		listed[submission.ID] = true
	}
	for _, submission := range backend {
		if submission.State == models.DeliverySubmissionInterruptedUnknown && !listed[submission.ID] {
			return true
		}
	}
	return false
}

func peerAcknowledgesUnknownHistory(execution *AgentExecution, status *agentctl.DeliveryStatus, peer journal.Submission, submission *models.AgentDeliverySubmission) bool {
	return peer.Retired && peerMatchesUnknownHistory(execution, status, peer, submission)
}

func peerMatchesUnknownHistory(execution *AgentExecution, status *agentctl.DeliveryStatus, peer journal.Submission, submission *models.AgentDeliverySubmission) bool {
	return submission.State == models.DeliverySubmissionInterruptedUnknown &&
		peer.ID == submission.ID && peer.State == journal.SubmissionInterruptedUnknown &&
		peer.SessionID == execution.SessionID && peer.SessionID == submission.SessionID &&
		peer.IncarnationID == status.IncarnationID && peer.IncarnationID == submission.IncarnationID &&
		peer.HarnessGeneration > 0 && peer.HarnessGeneration <= status.HarnessGeneration && int64(peer.HarnessGeneration) == submission.HarnessGeneration &&
		peer.Hash != "" && peer.Hash == submission.PayloadHash
}
