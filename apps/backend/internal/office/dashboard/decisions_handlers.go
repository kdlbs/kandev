package dashboard

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// DecisionDTO is the public-facing representation of a task approval
// decision. The decider's display name is resolved via the AgentReader
// when available so the timeline reads "CEO approved this task" rather
// than dumping the raw agent ID.
type DecisionDTO struct {
	ID           string `json:"id"`
	TaskID       string `json:"task_id"`
	DeciderType  string `json:"decider_type"`
	DeciderID    string `json:"decider_id"`
	DeciderName  string `json:"decider_name,omitempty"`
	Role         string `json:"role"`
	Decision     string `json:"decision"`
	Comment      string `json:"comment,omitempty"`
	CreatedAt    string `json:"created_at"`
	SupersededAt string `json:"superseded_at,omitempty"`
}

// ApproveTaskRequest is the body for POST /tasks/:id/approve.
type ApproveTaskRequest struct {
	Comment string `json:"comment"`
}

// RequestChangesRequest is the body for POST /tasks/:id/request-changes.
type RequestChangesRequest struct {
	Comment string `json:"comment"`
}

// DecisionResponse wraps a single created decision row.
type DecisionResponse struct {
	Decision *DecisionDTO `json:"decision"`
}

// DecisionListResponse wraps the active decisions for a task.
type DecisionListResponse struct {
	Decisions []*DecisionDTO `json:"decisions"`
}

// callerHeader is the HTTP header an unauthenticated frontend uses to
// signal "this request is from the singleton human user" so the
// service treats them as an implicit approver. Agents go through the
// existing agent_caller middleware path.
const callerHeader = "X-Office-User-Caller"

// approveTask handles POST /tasks/:id/approve.
func (h *Handler) approveTask(c *gin.Context) {
	var req ApproveTaskRequest
	_ = c.ShouldBindJSON(&req)
	callerType, callerID := resolveDeciderCaller(c)
	d, err := h.svc.ApproveTask(c.Request.Context(), callerType, callerID, c.Param("id"), req.Comment)
	if err != nil {
		h.respondDecisionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, DecisionResponse{
		Decision: h.decisionToDTO(c, d),
	})
}

// requestTaskChanges handles POST /tasks/:id/request-changes.
func (h *Handler) requestTaskChanges(c *gin.Context) {
	var req RequestChangesRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Comment == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": approvalCommentRequiredOnRequest})
		return
	}
	callerType, callerID := resolveDeciderCaller(c)
	d, err := h.svc.RequestTaskChanges(c.Request.Context(), callerType, callerID, c.Param("id"), req.Comment)
	if err != nil {
		h.respondDecisionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, DecisionResponse{
		Decision: h.decisionToDTO(c, d),
	})
}

// listTaskDecisions handles GET /tasks/:id/decisions.
func (h *Handler) listTaskDecisions(c *gin.Context) {
	rows, err := h.svc.ListTaskDecisions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	out := make([]*DecisionDTO, len(rows))
	for i := range rows {
		row := rows[i]
		out[i] = h.decisionToDTO(c, &row)
	}
	c.JSON(http.StatusOK, DecisionListResponse{Decisions: out})
}

// resolveDeciderCaller returns (callerType, callerID) for a decision
// HTTP request. Agents go through the existing agent_caller middleware
// (set by the gateway). Unauthenticated requests are treated as the
// singleton human user when the X-Office-User-Caller header is set;
// otherwise callerType is empty (the service will return ErrForbidden).
func resolveDeciderCaller(c *gin.Context) (string, string) {
	if id := agentIDFromCtx(c); id != "" {
		return models.DeciderTypeAgent, id
	}
	if c.GetHeader(callerHeader) != "" {
		return models.DeciderTypeUser, c.GetHeader(callerHeader)
	}
	// v1 simplification: treat unauthenticated callers as the
	// singleton user. The frontend always runs against the local
	// backend and there is exactly one human user in the office.
	return models.DeciderTypeUser, userSentinel
}

// decisionToDTO maps a DecisionRecord into the JSON DTO,
// resolving the decider's display name via the AgentReader. The DTO
// JSON shape is preserved across the ADR 0005 Wave E migration so the
// frontend doesn't need to change.
func (h *Handler) decisionToDTO(c *gin.Context, d *DecisionRecord) *DecisionDTO {
	dto := &DecisionDTO{
		ID:          d.ID,
		TaskID:      d.TaskID,
		DeciderType: d.DeciderType,
		DeciderID:   d.DeciderID,
		Role:        d.Role,
		Decision:    d.Decision,
		Comment:     d.Comment,
		CreatedAt:   d.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if d.SupersededAt != nil {
		dto.SupersededAt = d.SupersededAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	dto.DeciderName = h.svc.resolveDeciderName(c.Request.Context(), d)
	return dto
}

// respondDecisionError maps service errors to HTTP responses.
// ErrForbidden → 403; ApprovalsPendingError shouldn't appear here, but
// is mapped to 409 defensively. Everything else → 400.
func (h *Handler) respondDecisionError(c *gin.Context, err error) {
	if errors.Is(err, shared.ErrForbidden) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	var pending *ApprovalsPendingError
	if errors.As(err, &pending) {
		c.JSON(http.StatusConflict, gin.H{
			"error":             err.Error(),
			"pending_approvers": h.svc.resolvePendingApprovers(c.Request.Context(), pending.Pending),
		})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}
