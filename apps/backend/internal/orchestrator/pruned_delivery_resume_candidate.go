package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func (s *Service) hasPrunedDeliveryResumeCandidate(ctx context.Context, session *models.TaskSession) bool {
	if recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata); exists && recovery.HistoryGap != nil {
		return silentRestoreRecoveryPhase(recovery.Phase)
	}
	client, supported := s.agentManager.(deliveryRecordEvidenceClient)
	continuity, hasContinuity := s.repo.(sessionContinuityStore)
	delivery, hasDelivery := s.repo.(repository.AgentDeliveryRepository)
	if !supported || !hasContinuity || !hasDelivery {
		return false
	}
	incarnation := session.QueueIncarnationID
	if incarnation == "" {
		incarnation = session.ID
	}
	generation, err := continuity.GetCurrentHarnessSessionGeneration(ctx, session.ID, incarnation)
	if err != nil || generation == nil || generation.NativeSessionID == "" {
		return false
	}
	evidence, err := client.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{TaskID: session.TaskID, SessionID: session.ID, ExecutionID: session.AgentExecutionID, IncarnationID: incarnation, HarnessGeneration: uint64(generation.Generation)})
	if err != nil || evidence == nil {
		return false
	}
	return prunedDeliveryCandidateCursorMatches(ctx, delivery, session, incarnation, generation.Generation, evidence.Descriptor.Stream)
}

func prunedDeliveryCandidateCursorMatches(ctx context.Context, delivery repository.AgentDeliveryRepository, session *models.TaskSession, incarnation string, generation int64, stream *journal.Stream) bool {
	if stream == nil || stream.HighWater > 1<<63-1 || stream.FirstRetained > 1<<63-1 {
		return false
	}
	cursor, err := delivery.GetAgentDeliveryCursor(ctx, stream.StreamID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false
	}
	var received, projected int64
	if cursor != nil {
		if cursor.SessionID != session.ID || cursor.IncarnationID != incarnation || cursor.HarnessGeneration != generation {
			return false
		}
		received, projected = cursor.ReceivedSequence, cursor.ProjectedSequence
	}
	gap := models.AgentDeliveryHistoryGap{FirstSequence: received + 1, LastSequence: int64(stream.HighWater), ReceivedSequence: received, ProjectedSequence: projected}
	return gap.MatchesPrunedStream(received, projected, int64(stream.HighWater), int64(stream.Acknowledged), int64(stream.FirstRetained))
}
