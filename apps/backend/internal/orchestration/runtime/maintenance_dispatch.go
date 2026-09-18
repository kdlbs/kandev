package runtime

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kandev/kandev/internal/orchestration/maintenance"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (s *Service) startMaintenance(ctx context.Context, b *models.AssistantBinding, grant models.MaintenanceGrant, req maintenanceRequest, guard maintenance.Guard) (any, error) {
	candidate, err := s.Repo.ImprovementCandidate(ctx, b.ID, grant.CandidateID)
	if err != nil {
		return nil, err
	}
	if candidate.RepairTaskID != "" {
		return candidate, nil
	}
	if err = s.Repo.ReserveMaintenanceRepair(ctx, b, grant, req.CandidateRevision, *req.ExpectedIntentRevision); err != nil {
		return nil, rejectOperation(409, "maintenance_proposal_superseded")
	}
	completed := false
	defer func() {
		if !completed {
			receipt, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
			defer cancel()
			_ = s.Repo.UnknownMaintenanceRepair(receipt, b.ID, grant.CandidateID)
		}
	}()
	_, source, err := s.currentMaintenance(ctx, b, grant.CandidateID, req)
	if err != nil {
		return nil, err
	}
	if err = s.Maintenance.Prepare(ctx, source, grant, guard); err != nil {
		return nil, err
	}
	packet, err := s.buildContext(ctx, b, candidate.ObjectiveID, models.ContextScope{ProfileID: grant.Scope.ProfileID, ProjectID: grant.Scope.RepositoryID})
	if err != nil {
		return nil, err
	}
	raw, err := json.Marshal(packet)
	if err != nil {
		return nil, err
	}
	if err = s.Repo.SaveContextPacket(ctx, packet, string(raw)); err != nil {
		return nil, err
	}
	objective, err := s.Repo.Objective(ctx, b.ID, candidate.ObjectiveID)
	if err != nil {
		return nil, err
	}
	ref := delegationReference(objective, packet.ID, req.OperationID)
	ref.Packet = packet
	spec := models.WorkspaceTaskSpec{MaintenanceCandidateID: candidate.ID, DirectProfile: true, WorkspaceID: b.WorkspaceID, ChiefID: b.OrchestratorID,
		WorkflowID: grant.Scope.WorkflowID, WorkflowStepID: grant.Scope.WorkflowStepID, RepositoryID: grant.Scope.RepositoryID, AssigneeID: grant.Scope.ProfileID,
		ExecutionMode: executionModeExecute, Title: "Prepare a scoped workflow repair", Description: "Prepare and review the approved local repair through the Assistant maintenance controls. Ordinary agent launch and publication are outside this task's grant.",
		ExternalID: "assistant-maintenance:" + candidate.ID, DelegationReference: ref}
	if err = guard(ctx); err != nil {
		return nil, err
	}
	task, err := s.Manager.CreateWorkspaceTask(ctx, spec)
	if err != nil {
		return nil, err
	}
	receipt, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err = s.Repo.RecordMaintenanceTask(receipt, b.ID, candidate.ID, task, packet.ID, req.OperationID); err != nil {
		return nil, err
	}
	completed = true
	return s.Repo.ImprovementCandidate(receipt, b.ID, candidate.ID)
}
