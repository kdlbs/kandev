package approvals

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// ServiceIface is the subset of ApprovalService used by the HTTP handlers.
type ServiceIface interface {
	GetApproval(ctx context.Context, id string) (*Approval, error)
	ListApprovals(ctx context.Context, wsID string) ([]*Approval, error)
	DecideApproval(ctx context.Context, approvalID, status, decidedBy, note string) (*Approval, error)
}

// Handler provides HTTP handlers for approval routes.
type Handler struct {
	svc ServiceIface
}

// NewHandler constructs an approval Handler.
func NewHandler(svc ServiceIface) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts approval routes onto an existing router group.
func RegisterRoutes(api *gin.RouterGroup, svc ServiceIface) {
	h := NewHandler(svc)
	api.GET("/workspaces/:wsId/approvals", h.listApprovals)
	api.POST("/approvals/:id/decide", h.decideApproval)
}

func (h *Handler) listApprovals(c *gin.Context) {
	approvals, err := h.svc.ListApprovals(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, ApprovalListResponse{Approvals: approvals})
}

func (h *Handler) decideApproval(c *gin.Context) {
	var req DecideApprovalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	approval, err := h.svc.GetApproval(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "approval not found"})
		return
	}

	// Resolve the authoritative "decided by" identity from the auth
	// context — never trust req.DecidedBy for agent callers. For agent
	// callers, also gate on can_approve and reject cross-workspace
	// decisions (workers must not be able to approve their own hire
	// requests, and a caller in workspace A must not be able to decide
	// workspace B's approvals).
	caller := agents.CallerFromContext(c)
	decidedBy, err := resolveDecider(c, caller, approval, req.DecidedBy)
	if err != nil {
		return
	}

	decided, err := h.svc.DecideApproval(
		c.Request.Context(), approval.ID,
		req.Status, decidedBy, req.DecisionNote,
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"approval": decided})
}

// resolveDecider returns the trusted "decided_by" identifier and performs
// the agent-caller authorization checks. Writes the HTTP error response
// directly on rejection and returns a non-nil error so the handler stops.
//
// For UI callers (no agent JWT) the request body's DecidedBy is still
// accepted for now — the dashboard does not yet ship a real user-session
// auth layer; once it does, this branch should derive the identity from
// the session instead.
func resolveDecider(
	c *gin.Context, caller *models.AgentInstance, approval *Approval, requestedDecidedBy string,
) (string, error) {
	if caller == nil {
		if requestedDecidedBy != "" {
			return requestedDecidedBy, nil
		}
		return "ui", nil
	}
	if caller.WorkspaceID != approval.WorkspaceID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot decide approvals from another workspace"})
		return "", shared.ErrForbidden
	}
	perms := shared.ResolvePermissions(shared.AgentRole(caller.Role), caller.Permissions)
	if !shared.HasPermission(perms, shared.PermCanApprove) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: missing can_approve"})
		return "", shared.ErrForbidden
	}
	// Prevent self-approval of one's own hire/request approvals.
	if approval.RequestedByAgentProfileID != "" &&
		approval.RequestedByAgentProfileID == caller.ID {
		c.JSON(http.StatusForbidden, gin.H{"error": "cannot decide an approval you requested"})
		return "", shared.ErrForbidden
	}
	return caller.ID, nil
}
