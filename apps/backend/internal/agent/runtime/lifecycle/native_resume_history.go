package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
)

type contextContinuedSubmissionReader interface {
	ListContextContinuedSubmissionIDs(context.Context, string, string, string, int64) ([]string, error)
}

// A consumed context continuation changes the active native scope, while the
// old SQL outcome remains unknown. Retained peer records still require their
// ordinary exact validation; ancestry never hides contradictory peer evidence.
func contextContinuedNativeResumeHistory(ctx context.Context, execution *AgentExecution, status *agentctl.DeliveryStatus, repository AgentDeliveryRepository, peer []journal.Submission, rows []*models.AgentDeliverySubmission) ([]*models.AgentDeliverySubmission, error) {
	reader, supported := repository.(contextContinuedSubmissionReader)
	if !supported {
		return rows, nil
	}
	retained := make(map[string]bool, len(peer))
	for _, submission := range peer {
		retained[submission.ID] = true
	}
	if !hasMissingHistoricalNativeSubmission(status, retained, rows) {
		return rows, nil
	}
	if _, err := nativeResumeProofCursor(ctx, repository, status); err != nil {
		return nil, err
	}
	ids, err := reader.ListContextContinuedSubmissionIDs(ctx, execution.SessionID, status.IncarnationID, execution.ACPSessionID, int64(status.HarnessGeneration))
	if err != nil {
		return nil, err
	}
	continued := make(map[string]bool, len(ids))
	for _, id := range ids {
		continued[id] = true
	}
	active := make([]*models.AgentDeliverySubmission, 0, len(rows))
	for _, row := range rows {
		if missingHistoricalNativeSubmission(status, retained, row) && continued[row.ID] {
			continue
		}
		active = append(active, row)
	}
	return active, nil
}

func hasMissingHistoricalNativeSubmission(status *agentctl.DeliveryStatus, retained map[string]bool, rows []*models.AgentDeliverySubmission) bool {
	for _, row := range rows {
		if missingHistoricalNativeSubmission(status, retained, row) {
			return true
		}
	}
	return false
}

func missingHistoricalNativeSubmission(status *agentctl.DeliveryStatus, retained map[string]bool, row *models.AgentDeliverySubmission) bool {
	return row.State == models.DeliverySubmissionInterruptedUnknown && row.IncarnationID == status.IncarnationID &&
		row.HarnessGeneration > 0 && row.HarnessGeneration < int64(status.HarnessGeneration) && !retained[row.ID]
}
