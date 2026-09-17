package runtime

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
)

func (h *Handler) runtimeAssistant(c *gin.Context) (*runtimeauth.AgentClaims, *models.AssistantBinding, bool) {
	claims, ok := h.caller(c)
	if !ok {
		return nil, nil, false
	}
	row, err := h.Service.Repo.AssistantForConversation(c.Request.Context(), claims.TaskID)
	if err != nil || row.OrchestratorID != claims.AgentProfileID || row.WorkspaceID != claims.WorkspaceID {
		c.AbortWithStatus(404)
		return nil, nil, false
	}
	return claims, row, true
}

func (h *Handler) humanAssistant(c *gin.Context) (*models.AssistantBinding, bool) {
	identity, ok := assistantHuman(c)
	if !ok {
		return nil, false
	}
	row, err := h.Service.Repo.AssistantBinding(c.Request.Context(), identity.UserID)
	if err != nil || !h.assistantWorkspaceAllowed(c, row.WorkspaceID) {
		c.AbortWithStatus(404)
		return nil, false
	}
	return row, true
}

func (h *Handler) objectives(c *gin.Context) {
	var binding *models.AssistantBinding
	var ok bool
	if _, runtime := c.Get("agent_claims"); runtime {
		_, binding, ok = h.runtimeAssistant(c)
	} else {
		binding, ok = h.humanAssistant(c)
	}
	if !ok {
		return
	}
	rows, err := h.Service.Repo.Objectives(c.Request.Context(), binding.ID, c.Query("after"), 51)
	if err != nil {
		c.AbortWithStatus(503)
		return
	}
	next := ""
	if len(rows) > 50 {
		rows = rows[:50]
		next = rows[len(rows)-1].ID
	}
	c.JSON(200, gin.H{"objectives": rows, nextCursorKey: next})
}

func (h *Handler) createObjective(c *gin.Context) {
	claims, binding, ok := h.runtimeAssistant(c)
	if !ok {
		return
	}
	var req struct {
		models.OperationRequest
		Title      string             `json:"title"`
		Mode       string             `json:"mode"`
		Source     string             `json:"source_comment_id"`
		Acceptance []models.Criterion `json:"acceptance"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	h.performOperation(c, claims, req.OperationRequest, req, 201, func() (any, error) {
		source, err := h.Service.Repo.GetCommentByID(c.Request.Context(), claims.TaskID, req.Source)
		if err != nil || source.AuthorType != authorTypeUser || source.AuthorID != binding.OwnerUserID {
			return nil, rejectOperation(422, "objective requires an owner-authored source comment")
		}
		row := &models.Objective{BindingID: binding.ID, WorkspaceID: binding.WorkspaceID, SourceCommentID: req.Source, Title: req.Title, Mode: req.Mode, Status: statusActive, Acceptance: req.Acceptance, Evidence: []models.Evidence{}, IntentRevision: *req.ExpectedIntentRevision}
		if err := row.Validate(); err != nil {
			return nil, rejectOperation(422, err.Error())
		}
		return row, h.Service.Repo.CreateObjective(c.Request.Context(), row)
	})
}

type objectiveUpdate struct {
	models.OperationRequest
	ExpectedRevision int64              `json:"expected_revision"`
	Status           string             `json:"status"`
	Mode             string             `json:"mode"`
	Acceptance       []models.Criterion `json:"acceptance"`
	Evidence         []models.Evidence  `json:"evidence"`
}

func (h *Handler) updateObjective(c *gin.Context) {
	claims, binding, ok := h.runtimeAssistant(c)
	if !ok {
		return
	}
	var req objectiveUpdate
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	h.performOperation(c, claims, req.OperationRequest, req, http.StatusOK, func() (any, error) {
		return h.applyObjectiveUpdate(c.Request.Context(), binding, c.Param("id"), req)
	})
}

func (h *Handler) applyObjectiveUpdate(ctx context.Context, b *models.AssistantBinding, id string, req objectiveUpdate) (*models.Objective, error) {
	row, err := h.Service.Repo.Objective(ctx, b.ID, id)
	if err != nil {
		return nil, rejectOperation(404, "objective unavailable")
	}
	if row.Revision != req.ExpectedRevision {
		return nil, rejectOperation(409, "objective revision conflict")
	}
	if req.Status != "" {
		row.Status = req.Status
	}
	if req.Mode != "" {
		row.Mode = req.Mode
	}
	if req.Acceptance != nil {
		row.Acceptance = req.Acceptance
		row.AcceptanceRevision++
		row.Evidence = []models.Evidence{}
	}
	if req.Evidence != nil {
		row.Evidence = req.Evidence
	}
	row.IntentRevision = *req.ExpectedIntentRevision
	if err := row.Validate(); err != nil {
		return nil, rejectOperation(422, err.Error())
	}
	if row.Status == "complete" {
		if err := h.Service.validateObjectiveCompletion(ctx, b, row); err != nil {
			return nil, rejectOperation(422, err.Error())
		}
	}
	err = h.Service.Repo.UpdateObjective(ctx, row, req.ExpectedRevision)
	if errors.Is(err, models.ErrConflict) {
		return nil, rejectOperation(409, "objective revision conflict")
	}
	return row, err
}
