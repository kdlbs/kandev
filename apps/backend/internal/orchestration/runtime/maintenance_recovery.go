package runtime

import (
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/orchestration/models"
)

// A human can record an already committed artifact after inspecting uncertainty.
// This endpoint does not replay a patch, check, task creation or commit.
func (h *Handler) reconcileMaintenance(c *gin.Context) {
	b, ok := h.humanAssistant(c)
	if !ok {
		return
	}
	var req struct {
		ExpectedBindingVersion int64 `json:"expected_binding_version"`
		ExpectedRevision       int64 `json:"expected_revision"`
	}
	if c.ShouldBindJSON(&req) != nil {
		c.AbortWithStatus(400)
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
	if row.State != statusUnknown || row.Revision != req.ExpectedRevision || row.RepairTaskID == "" {
		c.AbortWithStatus(409)
		return
	}
	g, ok := h.humanMaintenanceArtifactGrant(c, b)
	if !ok {
		return
	}
	a, err := h.Service.Maintenance.Review(c.Request.Context(), *g)
	if !validRecoveredMaintenanceArtifact(a, *g, err) {
		c.AbortWithStatus(422)
		return
	}
	v := *a.Validation
	if err = h.Service.Repo.SaveMaintenanceValidation(c.Request.Context(), b.ID, row.ID, v); err != nil {
		c.AbortWithStatus(503)
		return
	}
	if err = h.Service.Repo.RecordRecoveredMaintenancePrepared(c.Request.Context(), b, row.ID, a.MaintenanceArtifact); err != nil {
		c.AbortWithStatus(409)
		return
	}
	h.Service.notifyAssistantUpdated(c.Request.Context(), b.ID)
	c.JSON(200, a.MaintenanceArtifact)
}

func validRecoveredMaintenanceArtifact(a models.MaintenanceReviewArtifact, g models.MaintenanceGrant, err error) bool {
	if err != nil || a.BaseOID != g.BaseOID || a.CommitOID == "" || a.CommitOID == g.BaseOID || a.Validation == nil {
		return false
	}
	v := a.Validation
	return v.Passed && len(v.Checks) == 2 && v.TreeOID == a.TreeOID && v.GrantRevision >= 1 && v.GrantRevision <= g.Revision
}
