package coordinator

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// Handlers provides the HTTP handlers for the coordinator CRUD, proposals
// read and stalls read routes (docs/plans/workspace-coordinator/
// task-01-shared-interface.md). The conversation, approve/reject and
// subscriber routes are registered by later work packages on the same
// Service.
type Handlers struct {
	service *Service
	logger  *logger.Logger
}

// NewHandlers creates coordinator HTTP handlers over svc.
func NewHandlers(svc *Service, log *logger.Logger) *Handlers {
	return &Handlers{service: svc, logger: log.WithFields(zap.String("component", "coordinator-handlers"))}
}

// RegisterRoutes registers the coordinator CRUD, proposals-read and
// stalls-read HTTP routes. A later work package (backendapp/coordinator.go)
// calls this only when features.coordinator is on
// (coordinators.md#flag-and-wiring); with the flag off these routes are
// simply never registered, so they 404.
func RegisterRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	h := NewHandlers(svc, log)
	workspace := router.Group("/api/v1/workspaces/:id")
	workspace.GET("/coordinators", h.httpListCoordinators)
	workspace.POST("/coordinators", h.httpCreateCoordinator)
	workspace.GET("/coordinators/:cid", h.httpGetCoordinator)
	workspace.PATCH("/coordinators/:cid", h.httpPatchCoordinator)
	workspace.DELETE("/coordinators/:cid", h.httpDeleteCoordinator)
	workspace.GET("/coordinators/:cid/proposals", h.httpListProposals)
	workspace.GET("/coordinators/:cid/proposals/:pid", h.httpGetProposal)
	workspace.POST("/coordinators/:cid/proposals/:pid/approve", h.httpApproveProposal)
	workspace.POST("/coordinators/:cid/proposals/:pid/reject", h.httpRejectProposal)
	workspace.GET("/coordinator-stalls", h.httpListStalls)
}

// httpListCoordinators backs GET /api/v1/workspaces/:id/coordinators.
func (h *Handlers) httpListCoordinators(c *gin.Context) {
	ctx := c.Request.Context()
	items, err := h.service.ListCoordinators(ctx, c.Param("id"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	dtos := make([]*CoordinatorDTO, len(items))
	for i, item := range items {
		dtos[i] = NewCoordinatorDTO(item.Coordinator).WithOpenProposals(item.OpenProposals)
	}
	c.JSON(http.StatusOK, NewCoordinatorListResponse(dtos))
}

// httpCreateCoordinator backs POST /api/v1/workspaces/:id/coordinators.
func (h *Handlers) httpCreateCoordinator(c *gin.Context) {
	ctx := c.Request.Context()
	var req CreateCoordinatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid request body"))
		return
	}
	created, err := h.service.CreateCoordinator(ctx, c.Param("id"), req)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, NewCoordinatorDTO(created))
}

// httpGetCoordinator backs GET /api/v1/workspaces/:id/coordinators/:cid.
func (h *Handlers) httpGetCoordinator(c *gin.Context) {
	ctx := c.Request.Context()
	found, agentStatus, executorStatus, err := h.service.GetCoordinator(ctx, c.Param("id"), c.Param("cid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCoordinatorDTO(found).WithProfileStatuses(agentStatus, executorStatus))
}

// httpPatchCoordinator backs PATCH /api/v1/workspaces/:id/coordinators/:cid.
func (h *Handlers) httpPatchCoordinator(c *gin.Context) {
	ctx := c.Request.Context()
	var req PatchCoordinatorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid request body"))
		return
	}
	updated, err := h.service.PatchCoordinator(ctx, c.Param("id"), c.Param("cid"), req)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewCoordinatorDTO(updated))
}

// httpDeleteCoordinator backs DELETE /api/v1/workspaces/:id/coordinators/:cid.
func (h *Handlers) httpDeleteCoordinator(c *gin.Context) {
	ctx := c.Request.Context()
	if err := h.service.DeleteCoordinator(ctx, c.Param("id"), c.Param("cid")); err != nil {
		h.respondError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// httpListProposals backs
// GET /api/v1/workspaces/:id/coordinators/:cid/proposals?status=pending|all.
func (h *Handlers) httpListProposals(c *gin.Context) {
	ctx := c.Request.Context()
	status, fieldErr := parseProposalListStatus(c)
	if fieldErr != nil {
		c.JSON(http.StatusBadRequest, NewFieldErrorResponse(fieldErr))
		return
	}
	items, err := h.service.ListProposals(ctx, c.Param("id"), c.Param("cid"), status)
	if err != nil {
		h.respondError(c, err)
		return
	}
	dtos := make([]*ProposalDTO, len(items))
	for i, item := range items {
		dtos[i] = NewProposalDTO(item)
	}
	c.JSON(http.StatusOK, NewProposalListResponse(dtos))
}

// parseProposalListStatus implements Build decision 3's status query
// parameter rule: absent means pending; exactly "pending" or "all" (case
// sensitive) select that list; any other value, including the empty string
// and case variants, is a 400 naming "status".
func parseProposalListStatus(c *gin.Context) (ListProposalsStatus, *FieldError) {
	raw, present := c.GetQuery("status")
	if !present {
		return ListProposalsPending, nil
	}
	switch raw {
	case "pending":
		return ListProposalsPending, nil
	case "all":
		return ListProposalsAll, nil
	default:
		return 0, &FieldError{Field: "status", Message: `status must be "pending" or "all"`}
	}
}

// httpGetProposal backs
// GET /api/v1/workspaces/:id/coordinators/:cid/proposals/:pid.
func (h *Handlers) httpGetProposal(c *gin.Context) {
	ctx := c.Request.Context()
	found, err := h.service.GetProposal(ctx, c.Param("id"), c.Param("cid"), c.Param("pid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewProposalDTO(found))
}

// httpApproveProposal backs
// POST /api/v1/workspaces/:id/coordinators/:cid/proposals/:pid/approve.
func (h *Handlers) httpApproveProposal(c *gin.Context) {
	ctx := c.Request.Context()
	var edits ApproveProposalRequest
	if err := decodeOptionalJSONBody(c, &edits); err != nil {
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid request body"))
		return
	}
	updated, err := h.service.ApproveProposal(ctx, c.Param("id"), c.Param("cid"), c.Param("pid"), edits)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewProposalDTO(updated))
}

// httpRejectProposal backs
// POST /api/v1/workspaces/:id/coordinators/:cid/proposals/:pid/reject.
func (h *Handlers) httpRejectProposal(c *gin.Context) {
	ctx := c.Request.Context()
	var req RejectProposalRequest
	if err := decodeOptionalJSONBody(c, &req); err != nil {
		c.JSON(http.StatusBadRequest, NewErrorResponse("invalid request body"))
		return
	}
	updated, err := h.service.RejectProposal(ctx, c.Param("id"), c.Param("cid"), c.Param("pid"), req)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, NewProposalDTO(updated))
}

// decodeOptionalJSONBody reads c.Request.Body and, unless it is empty or
// whitespace-only, JSON-decodes it into out. An empty body is left as out's
// zero value rather than an error (proposals.md#approve and #reject: "An
// empty body, or {}, approves/rejects unchanged"). out must be a pointer.
func decodeOptionalJSONBody(c *gin.Context, out any) error {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	return json.Unmarshal(body, out)
}

// httpListStalls backs GET /api/v1/workspaces/:id/coordinator-stalls.
func (h *Handlers) httpListStalls(c *gin.Context) {
	ctx := c.Request.Context()
	items, err := h.service.ListStalls(ctx, c.Param("id"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	dtos := make([]*StallDTO, len(items))
	for i, item := range items {
		dtos[i] = NewStallDTO(item)
	}
	c.JSON(http.StatusOK, NewStallListResponse(dtos))
}

// respondError maps a Service error to Build decision 4's response shapes: a
// *FieldError is 400 naming its field; a *ProposalConflictError (approve and
// reject only) is 409 with the current row; ErrNotFound or an unreadable
// workspace is 404; a forbidden workspace scope is 403; anything else is
// logged and returned as a plain 500.
func (h *Handlers) respondError(c *gin.Context, err error) {
	var fieldErr *FieldError
	var conflictErr *ProposalConflictError
	switch {
	case errors.As(err, &fieldErr):
		c.JSON(http.StatusBadRequest, NewFieldErrorResponse(fieldErr))
	case errors.As(err, &conflictErr):
		c.JSON(http.StatusConflict, NewProposalConflictResponse(conflictErr.Proposal))
	case errors.Is(err, ErrNotFound), errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, NewErrorResponse("not found"))
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, NewErrorResponse("forbidden"))
	default:
		h.logger.Error("coordinator route failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, NewErrorResponse("internal error"))
	}
}
