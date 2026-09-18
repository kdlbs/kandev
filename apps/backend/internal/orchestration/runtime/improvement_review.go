package runtime

import (
	"context"
	"errors"
	"slices"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func (h *Handler) reviewImprovement(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedBindingVersion int64           `json:"expected_binding_version"`
		ExpectedRevision       int64           `json:"expected_revision"`
		Action                 string          `json:"action"`
		Evidence               models.Evidence `json:"evidence"`
	}
	if c.ShouldBindJSON(&req) != nil || !slices.Contains([]string{statusResolved, "rejected"}, req.Action) {
		c.AbortWithStatus(422)
		return
	}
	if req.ExpectedBindingVersion != b.Version {
		c.AbortWithStatus(409)
		return
	}
	h.Service.maintenanceMu.Lock()
	defer h.Service.maintenanceMu.Unlock()
	row, err := h.Service.Repo.ImprovementCandidate(c.Request.Context(), b.ID, c.Param("id"))
	if err != nil || row.WorkspaceID != b.WorkspaceID {
		c.AbortWithStatus(404)
		return
	}
	if row.Revision != req.ExpectedRevision {
		c.AbortWithStatus(409)
		return
	}
	if req.Action == statusResolved {
		if err = h.Service.validateImprovementResolution(c.Request.Context(), b, row, req.Evidence); err != nil {
			c.JSON(422, gin.H{errorResponseKey: err.Error()})
			return
		}
	} else {
		req.Evidence = models.Evidence{}
	}
	err = h.Service.Repo.ReviewImprovement(c.Request.Context(), b, row.ID, req.ExpectedRevision, req.Action, req.Evidence, h.Service.attentionNow())
	if err != nil {
		if errors.Is(err, models.ErrConflict) {
			c.AbortWithStatus(409)
		} else {
			c.AbortWithStatus(503)
		}
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, gin.H{"state": req.Action, revisionResponseKey: req.ExpectedRevision + 1})
}

func (s *Service) validateImprovementResolution(ctx context.Context, b *models.AssistantBinding, row *models.ImprovementCandidate, evidence models.Evidence) error {
	if !preparedMaintenanceEvidence(row, evidence) {
		return errors.New("maintenance_resolution_requires_prepared_repair_and_native_result")
	}
	if !s.maintenanceAccountCurrent(ctx, row) {
		return errors.New("maintenance_resolution_account_configuration_changed")
	}
	affected, err := s.Repo.ImprovementAffectedTask(ctx, b.ID, row.ID, evidence.TaskID)
	if err != nil || !affected {
		return errors.New("maintenance_resolution_requires_affected_workflow")
	}
	task, err := s.Tasks.GetTask(ctx, evidence.TaskID)
	if err != nil || task.WorkspaceID != b.WorkspaceID || task.State != v1.TaskStateCompleted {
		return errors.New("maintenance_resolution_requires_completed_task")
	}
	sessions, err := s.Tasks.ListTaskSessions(ctx, task.ID)
	if err != nil || !matchingMaintenanceSuccess(row, evidence, sessions) {
		return errors.New("maintenance_resolution_requires_subsequent_success")
	}
	reader, ok := s.Manager.(MaintenanceReviewReader)
	if !ok {
		return errors.New("maintenance_native_verification_unavailable")
	}
	return reader.ValidateMaintenanceSuccess(ctx, b.WorkspaceID, evidence, *row.PreparedAt)
}

func preparedMaintenanceEvidence(row *models.ImprovementCandidate, evidence models.Evidence) bool {
	return row.State == "prepared" && row.PreparedAt != nil && row.CommitOID != "" && evidence.SourceKind == "task_message" && evidence.SourceID != "" && len(evidence.SourceID) <= 200
}

func (s *Service) maintenanceAccountCurrent(ctx context.Context, row *models.ImprovementCandidate) bool {
	revision, err := s.contextProfileRevision(ctx, row.WorkspaceID, row.ProfileID)
	return err == nil && revision == row.AccountRevision
}

func matchingMaintenanceSuccess(row *models.ImprovementCandidate, evidence models.Evidence, sessions []*taskmodels.TaskSession) bool {
	for _, session := range sessions {
		profile := session.ExecutionProfileID
		if profile == "" {
			profile = session.AgentProfileID
		}
		if session.ID == evidence.SessionID && session.TaskID == evidence.TaskID && profile == row.ProfileID && session.State == taskmodels.TaskSessionStateCompleted && session.StartedAt.After(*row.PreparedAt) {
			return true
		}
	}
	return false
}
