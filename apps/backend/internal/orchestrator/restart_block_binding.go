package orchestrator

import (
	"context"
	"errors"
	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/task/models"
)

type restartRecoveryBlockRepairer interface {
	RepairRestartRecoveryBlock(context.Context, string, models.AgentDeliveryRecovery, models.SessionRecoveryBlock) (bool, error)
}

func (s *Service) repairKnownRestartBlock(ctx context.Context, session *models.TaskSession, recovery models.AgentDeliveryRecovery, block *models.SessionRecoveryBlock) error {
	inspector, ok := s.agentManager.(deliveryRecordEvidenceClient)
	if !ok {
		return errors.New("restart block journal evidence is unavailable")
	}
	evidence, err := inspector.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: session.TaskID, SessionID: session.ID, ExecutionID: recovery.AgentExecutionID, IncarnationID: recovery.IncarnationID,
		HarnessGeneration: uint64(recovery.HarnessGeneration), OriginalRuntime: &recovery.OriginalRuntime})
	if err != nil {
		return err
	}
	lister, ok := s.repo.(nativeResumeDeliverySubmissionReader)
	if !ok {
		return errors.New("restart submission inventory is unavailable")
	}
	rows, err := lister.ListAgentDeliverySubmissions(ctx, session.ID)
	if err != nil {
		return err
	}
	inputs := &silentRestoreInputs{session: session, recovery: recovery}
	if err := s.validateRestartSubmissionEvidence(ctx, inputs, rows, evidence); err != nil {
		return err
	}
	repairer, ok := s.repo.(restartRecoveryBlockRepairer)
	if !ok {
		return errors.New("restart block repair store is unavailable")
	}
	changed, err := repairer.RepairRestartRecoveryBlock(ctx, session.TaskID, recovery, *block)
	if err != nil {
		return err
	}
	if !changed {
		return errors.New("restart recovery block changed before repair")
	}
	return nil
}
