package orchestrator

import (
	"context"

	agentruntime "github.com/kandev/kandev/internal/agent/runtime"
	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/orchestrator/messagequeue"
	"github.com/kandev/kandev/internal/task/models"
)

type unsentSubmissionReader interface {
	GetAgentDeliverySubmissionMetadata(context.Context, string) (*models.AgentDeliverySubmission, error)
}

func unsentDeliveryState(state models.DeliverySubmissionState) bool {
	return state == models.DeliverySubmissionPrepared || state == models.DeliverySubmissionAccepted
}

func (s *Service) readUnsentQueueSubmission(ctx context.Context, claim *messagequeue.PendingQueueDispatch, session *models.TaskSession) (bool, error) {
	if claim == nil || claim.Accepted || claim.Protocol != messagequeue.DeliveryProtocolV1 || claim.SubmissionID == "" {
		return false, nil
	}
	reader, ok := s.repo.(unsentSubmissionReader)
	if !ok {
		return false, nil
	}
	row, err := reader.GetAgentDeliverySubmissionMetadata(ctx, normalizeDeliverySubmissionID(claim.SubmissionID))
	if err != nil || row == nil || !unsentDeliveryState(row.State) {
		return false, err
	}
	identity := automaticExecutionQueueIdentity(session)
	generation, err := s.currentDeliveryGeneration(ctx, session, identity.SessionIncarnationID)
	if err != nil || !unsentSubmissionMatchesSession(row, session.ID, identity.SessionIncarnationID, generation) {
		return false, err
	}
	return s.nativeUnsentQueueEvidence(ctx, claim, row)
}

func unsentSubmissionMatchesSession(row *models.AgentDeliverySubmission, sessionID, incarnation string, generation int64) bool {
	return row.SessionID == sessionID && row.IncarnationID == incarnation && row.HarnessGeneration == generation &&
		row.OwnerGeneration == generation && row.DispatchAttemptID != ""
}

func (s *Service) nativeUnsentQueueEvidence(ctx context.Context, claim *messagequeue.PendingQueueDispatch, row *models.AgentDeliverySubmission) (bool, error) {
	client, ok := s.agentManager.(deliveryRecordEvidenceClient)
	if !ok {
		return false, nil
	}
	evidence, err := client.InspectDeliveryRecordEvidence(ctx, agentruntime.DeliveryRecordEvidenceRequest{
		TaskID: claim.Message.TaskID, SessionID: row.SessionID, IncarnationID: row.IncarnationID,
		HarnessGeneration: uint64(row.HarnessGeneration), StoredSubmissionID: row.ID,
	})
	if err != nil || evidence == nil || evidence.ProcessTerminated == nil || !*evidence.ProcessTerminated {
		return false, err
	}
	peer, owner := evidence.StoredSubmission, evidence.ControlIdentity
	if peer == nil || owner == nil || (peer.State != journal.SubmissionPrepared && peer.State != journal.SubmissionAccepted) {
		return false, nil
	}
	return matchingNativeUnsentOwner(claim, row, evidence), nil
}

func matchingNativeUnsentOwner(claim *messagequeue.PendingQueueDispatch, row *models.AgentDeliverySubmission, evidence *agentruntime.DeliveryRecordEvidence) bool {
	peer, owner := evidence.StoredSubmission, evidence.ControlIdentity
	return peer.ID == row.ID && peer.SessionID == row.SessionID && peer.IncarnationID == row.IncarnationID &&
		peer.HarnessGeneration == uint64(row.HarnessGeneration) && !peer.Retired &&
		owner.TaskID == claim.Message.TaskID && owner.SessionID == row.SessionID && owner.SubmissionID == row.ID &&
		owner.IncarnationID == row.IncarnationID && owner.HarnessGeneration == uint64(row.HarnessGeneration) &&
		owner.ExecutionID != "" && evidence.LiveExecutionID == ""
}

// A proven pre-dispatch claim can remain parked by an independent task policy.
// Reporting it handled avoids relabelling a known unsent request as unknown.
func (s *Service) recoverAutomaticUnsentClaim(ctx context.Context, claim *messagequeue.PendingQueueDispatch) (bool, error) {
	if !s.config.AutoRetryUnsentRequests || claim == nil || claim.Accepted || s.silentRestoreOwnerContext == nil {
		return false, nil
	}
	session, err := s.repo.GetTaskSession(ctx, claim.Message.SessionID)
	if err != nil || session == nil || session.TaskID != claim.Message.TaskID {
		return false, err
	}
	known, err := s.readUnsentQueueSubmission(ctx, claim, session)
	if err != nil || !known {
		return false, err
	}
	task, err := s.repo.GetTask(ctx, session.TaskID)
	if err != nil || task == nil {
		return true, err
	}
	owner, current, err := s.automaticExecutionOwner(ctx, models.SilentRestoreCandidate{
		TaskID: task.ID, SessionID: session.ID, WorkspaceID: task.WorkspaceID,
	})
	if err != nil || owner == nil || isTerminalSessionState(current.State) {
		return true, err
	}
	return true, s.restoreAutomaticUnsentClaim(owner.ctx, claim, current)
}

func (s *Service) restoreAutomaticUnsentClaim(ctx context.Context, claim *messagequeue.PendingQueueDispatch, current *models.TaskSession) error {
	lock, release := s.acquireCancelInFlightGuard(current.ID)
	defer release()
	if !lock.TryLock() {
		return nil
	}
	defer lock.Unlock()
	if s.agentManager.IsAgentRunningForSession(ctx, current.ID) {
		return nil
	}
	known, err := s.readUnsentQueueSubmission(ctx, claim, current)
	if err != nil || !known {
		return err
	}
	if err := s.restoreOneQueueDispatchForRecovery(ctx, current.ID, claim); err != nil {
		return err
	}
	return s.messageQueue.DeletePendingQueueDispatch(ctx, &claim.Message)
}

func (s *Service) recoverAutomaticUnsentClaims(ctx context.Context) {
	if !s.config.AutoRetryUnsentRequests || s.messageQueue == nil {
		return
	}
	claims, err := s.messageQueue.ListPendingQueueDispatches(ctx)
	if err != nil {
		return
	}
	for i := range claims {
		if ctx.Err() != nil {
			return
		}
		_, _ = s.recoverAutomaticUnsentClaim(ctx, &claims[i])
	}
}
