package orchestrator

import (
	"context"
	"database/sql"
	"errors"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
)

func recoveryPayloadHash(recovery models.AgentDeliveryRecovery) string {
	if recovery.Reconstruction != nil {
		return recovery.Reconstruction.PayloadHash
	}
	return ""
}

func (s *Service) provePrunedDeliveryReplacementDead(running *models.ExecutorRunning, sessionID string) error {
	if running == nil || running.SessionID != sessionID || running.AgentExecutionID == "" {
		return errors.New("replacement execution identity is unavailable")
	}
	prober, ok := s.agentManager.(rowLivenessProber)
	if !ok || prober.RowLiveness(running) != models.ProcessLivenessDead {
		return errors.New("replacement execution liveness is not proven dead")
	}
	return nil
}

func (s *Service) knownRestartHistoryGap(ctx context.Context, session *models.TaskSession, recovery models.AgentDeliveryRecovery) bool {
	if recovery.HistoryGap != nil {
		return false
	}
	client, supported := s.agentManager.(deliveryRecordEvidenceClient)
	delivery, hasRepository := s.repo.(repository.AgentDeliveryRepository)
	if !supported || !hasRepository {
		return false
	}
	evidence, err := client.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: session.TaskID, SessionID: session.ID, ExecutionID: recovery.AgentExecutionID,
		IncarnationID: recovery.IncarnationID, HarnessGeneration: uint64(recovery.HarnessGeneration),
		OriginalRuntime: &recovery.OriginalRuntime})
	if err != nil || evidence == nil || evidence.ProcessTerminated == nil || !*evidence.ProcessTerminated {
		return false
	}
	stream := evidence.Descriptor.Stream
	if stream == nil || stream.StreamID != recovery.StreamID || stream.HighWater > 1<<63-1 || stream.FirstRetained > 1<<63-1 {
		return false
	}
	cursor, err := delivery.GetAgentDeliveryCursor(ctx, recovery.StreamID)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return false
	}
	var received, projected int64
	if cursor != nil {
		received, projected = cursor.ReceivedSequence, cursor.ProjectedSequence
	}
	gap := models.AgentDeliveryHistoryGap{FirstSequence: received + 1, LastSequence: int64(stream.HighWater),
		ReceivedSequence: received, ProjectedSequence: projected}
	return gap.MatchesPrunedStream(received, projected, int64(stream.HighWater), int64(stream.Acknowledged), int64(stream.FirstRetained))
}

func (s *Service) resumePrunedDelivery(ctx context.Context, session *models.TaskSession) (*LaunchSessionResponse, bool, error) {
	if !s.hasPrunedDeliveryResumeCandidate(ctx, session) {
		return nil, false, nil
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil {
		return nil, false, err
	}
	if models.IsAutomationTaskOrigin(task.Origin) {
		return nil, false, nil
	}
	office, err := s.lookupOfficeTask(ctx, task.ID)
	if err != nil || office {
		return nil, false, err
	}
	session, err = s.repairRestartRecoveryPresentation(ctx, session)
	if err != nil {
		return nil, false, err
	}
	recovery, exists := models.LoadAgentDeliveryRecovery(session.Metadata)
	if !exists || recovery.HistoryGap == nil || !silentRestoreRecoveryPhase(recovery.Phase) {
		return nil, false, nil
	}
	store, supported := s.repo.(repository.SilentRestoreRepository)
	if !supported {
		return nil, true, errors.New("guarded delivery gap restoration is unavailable")
	}
	err = s.restoreSilentRestartCandidate(ctx, store, models.SilentRestoreCandidate{
		TaskID: task.ID, SessionID: session.ID, WorkspaceID: task.WorkspaceID})
	if err != nil {
		return nil, true, err
	}
	current, err := s.repo.GetTaskSession(ctx, session.ID)
	if err != nil {
		return nil, true, err
	}
	restored, exists := models.LoadAgentDeliveryRecovery(current.Metadata)
	return &LaunchSessionResponse{Success: true, InProgress: !exists || restored.Phase != models.AgentDeliveryRecoveryRestored,
		TaskID: task.ID, SessionID: session.ID, AgentExecutionID: current.AgentExecutionID, State: string(current.State)}, true, nil
}
