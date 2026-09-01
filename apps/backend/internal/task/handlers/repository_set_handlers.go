package handlers

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// RepositorySetHandlers exposes named groups of workspace repositories over HTTP
// and the WebSocket dispatcher. Collection routes are workspace-scoped and item
// routes are flat, matching the sibling repository routes; the workspace param
// must stay `:id` because gin rejects a second wildcard name at that position
// within one group.
type RepositorySetHandlers struct {
	service *service.Service
	logger  *logger.Logger
}

func NewRepositorySetHandlers(svc *service.Service, log *logger.Logger) *RepositorySetHandlers {
	return &RepositorySetHandlers{
		service: svc,
		logger:  log.WithFields(zap.String("component", "task-repository-set-handlers")),
	}
}

func RegisterRepositorySetRoutes(
	router *gin.Engine,
	dispatcher *ws.Dispatcher,
	svc *service.Service,
	log *logger.Logger,
) *RepositorySetHandlers {
	handlers := NewRepositorySetHandlers(svc, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
	return handlers
}

func (h *RepositorySetHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/workspaces/:id/repository-sets", h.httpListRepositorySets)
	api.POST("/workspaces/:id/repository-sets", h.httpCreateRepositorySet)
	api.GET("/repository-sets/:id", h.httpGetRepositorySet)
	api.PATCH("/repository-sets/:id", h.httpUpdateRepositorySet)
	api.DELETE("/repository-sets/:id", h.httpDeleteRepositorySet)
}

func (h *RepositorySetHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionRepositorySetList, h.wsListRepositorySets)
	dispatcher.RegisterFunc(ws.ActionRepositorySetCreate, h.wsCreateRepositorySet)
	dispatcher.RegisterFunc(ws.ActionRepositorySetGet, h.wsGetRepositorySet)
	dispatcher.RegisterFunc(ws.ActionRepositorySetUpdate, h.wsUpdateRepositorySet)
	dispatcher.RegisterFunc(ws.ActionRepositorySetDelete, h.wsDeleteRepositorySet)
}

// repositorySetsToListResponse keeps the list shape identical on both transports.
func repositorySetsToListResponse(sets []*models.RepositorySet) dto.ListRepositorySetsResponse {
	items := make([]dto.RepositorySetDTO, 0, len(sets))
	for _, set := range sets {
		if set != nil {
			items = append(items, dto.FromRepositorySet(set))
		}
	}
	return dto.ListRepositorySetsResponse{RepositorySets: items, Total: len(items)}
}

// repositorySetCreateBody is the create payload. workspace_id is read from the
// path on HTTP and from the payload over WebSocket.
type repositorySetCreateBody struct {
	WorkspaceID   string                              `json:"workspace_id"`
	Name          string                              `json:"name"`
	Description   string                              `json:"description"`
	RepositoryIDs *[]string                           `json:"repository_ids"`
	Repositories  *[]service.RepositorySetMemberInput `json:"repositories"`
}

func (b repositorySetCreateBody) serviceRequest(workspaceID string) (*service.CreateRepositorySetRequest, error) {
	if b.RepositoryIDs != nil && b.Repositories != nil {
		return nil, fmt.Errorf("%w: repositories and repository_ids cannot both be provided", service.ErrInvalidRepositorySet)
	}
	req := &service.CreateRepositorySetRequest{
		WorkspaceID: workspaceID,
		Name:        b.Name,
		Description: b.Description,
	}
	if b.RepositoryIDs != nil {
		req.RepositoryIDs = *b.RepositoryIDs
	}
	if b.Repositories != nil {
		req.Repositories = *b.Repositories
	}
	return req, nil
}

// repositorySetUpdateBody patches a set. Pointers distinguish "absent" from
// "set to empty": an absent repository_ids leaves membership alone, while an
// explicitly empty one is a rejected request rather than a silent wipe.
type repositorySetUpdateBody struct {
	Name          *string                             `json:"name"`
	Description   *string                             `json:"description"`
	RepositoryIDs *[]string                           `json:"repository_ids"`
	Repositories  *[]service.RepositorySetMemberInput `json:"repositories"`
}

// repositorySetStatus maps a service error to its HTTP status. Categories are
// stable so callers never parse a message.
func repositorySetStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrInvalidRepositorySet):
		return http.StatusBadRequest
	case errors.Is(err, service.ErrRepositorySetNameConflict):
		return http.StatusConflict
	case errors.Is(err, service.ErrUnknownRepositorySetMembers):
		return http.StatusUnprocessableEntity
	case errors.Is(err, repoerrors.ErrRepositorySetNotFound),
		errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		return http.StatusNotFound
	default:
		return http.StatusInternalServerError
	}
}

// abortWithRepositorySetError answers with the mapped status. A 500 is logged
// and given a generic body; every other status carries the service message,
// which names the conflicting set or the offending repository ids.
func (h *RepositorySetHandlers) abortWithRepositorySetError(c *gin.Context, action string, err error) {
	status := repositorySetStatus(err)
	if status == http.StatusInternalServerError {
		h.logger.Error("repository set request failed", zap.String("action", action), zap.Error(err))
		c.JSON(status, gin.H{"error": "failed to " + action})
		return
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

// resolveWritableWorkspace rejects an unknown workspace with 404 and the
// read-only Improve Kandev workspace with 409, before any set is touched.
// Workspace-scoped authorization happens again in the service; this exists so an
// unknown workspace does not read as an empty one.
func (h *RepositorySetHandlers) resolveWritableWorkspace(c *gin.Context, workspaceID string) bool {
	workspace, err := h.service.GetWorkspace(c.Request.Context(), workspaceID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not found")
		return false
	}
	if workspace.IsImproveKandev() {
		c.JSON(http.StatusConflict, gin.H{"error": workspaceReadOnlyMsg})
		return false
	}
	return true
}

// HTTP handlers

func (h *RepositorySetHandlers) httpListRepositorySets(c *gin.Context) {
	sets, err := h.service.ListRepositorySets(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abortWithRepositorySetError(c, "list repository sets", err)
		return
	}
	c.JSON(http.StatusOK, repositorySetsToListResponse(sets))
}

func (h *RepositorySetHandlers) httpCreateRepositorySet(c *gin.Context) {
	var body repositorySetCreateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	workspaceID := c.Param("id")
	if !h.resolveWritableWorkspace(c, workspaceID) {
		return
	}
	request, err := body.serviceRequest(workspaceID)
	if err != nil {
		h.abortWithRepositorySetError(c, "create repository set", err)
		return
	}
	set, err := h.service.CreateRepositorySet(c.Request.Context(), request)
	if err != nil {
		h.abortWithRepositorySetError(c, "create repository set", err)
		return
	}
	c.JSON(http.StatusCreated, dto.FromRepositorySet(set))
}

func (h *RepositorySetHandlers) httpGetRepositorySet(c *gin.Context) {
	set, err := h.service.GetRepositorySet(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abortWithRepositorySetError(c, "get repository set", err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRepositorySet(set))
}

func (h *RepositorySetHandlers) httpUpdateRepositorySet(c *gin.Context) {
	var body repositorySetUpdateBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	set, err := h.service.GetRepositorySet(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abortWithRepositorySetError(c, "get repository set", err)
		return
	}
	if !h.resolveWritableWorkspace(c, set.WorkspaceID) {
		return
	}
	updated, err := h.service.UpdateRepositorySet(c.Request.Context(), set.ID, &service.UpdateRepositorySetRequest{
		Name:          body.Name,
		Description:   body.Description,
		RepositoryIDs: body.RepositoryIDs,
		Repositories:  body.Repositories,
	})
	if err != nil {
		h.abortWithRepositorySetError(c, "update repository set", err)
		return
	}
	c.JSON(http.StatusOK, dto.FromRepositorySet(updated))
}

func (h *RepositorySetHandlers) httpDeleteRepositorySet(c *gin.Context) {
	set, err := h.service.GetRepositorySet(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.abortWithRepositorySetError(c, "get repository set", err)
		return
	}
	if !h.resolveWritableWorkspace(c, set.WorkspaceID) {
		return
	}
	if err := h.service.DeleteRepositorySet(c.Request.Context(), set.ID); err != nil {
		h.abortWithRepositorySetError(c, "delete repository set", err)
		return
	}
	c.Status(http.StatusNoContent)
}

// WebSocket handlers

// wsRejectReadOnlyWorkspace mirrors the HTTP guard for WebSocket mutations. The
// HTTP path resolves the workspace before writing; without the same check here a
// WebSocket client could create, update, or delete sets in the read-only Improve
// Kandev workspace.
func (h *RepositorySetHandlers) wsRejectReadOnlyWorkspace(
	ctx context.Context,
	msg *ws.Message,
	workspaceID string,
) (*ws.Message, bool) {
	workspace, err := h.service.GetWorkspace(ctx, workspaceID)
	if err != nil {
		errMsg, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "workspace not found", nil)
		return errMsg, true
	}
	if workspace.IsImproveKandev() {
		errMsg, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, workspaceReadOnlyMsg, nil)
		return errMsg, true
	}
	return nil, false
}

// wsRejectReadOnlySet resolves a set's workspace before a mutation keyed by set
// id, so update and delete are guarded exactly like create.
func (h *RepositorySetHandlers) wsRejectReadOnlySet(
	ctx context.Context,
	msg *ws.Message,
	setID string,
) (*ws.Message, bool) {
	set, err := h.service.GetRepositorySet(ctx, setID)
	if err != nil {
		errMsg, _ := h.wsRepositorySetError(msg, "get repository set", err)
		return errMsg, true
	}
	return h.wsRejectReadOnlyWorkspace(ctx, msg, set.WorkspaceID)
}

// wsRepositorySetError maps a service error onto the WS error codes, mirroring
// the HTTP status mapping so both transports categorize failures the same way.
func (h *RepositorySetHandlers) wsRepositorySetError(msg *ws.Message, action string, err error) (*ws.Message, error) {
	switch repositorySetStatus(err) {
	case http.StatusBadRequest:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	case http.StatusConflict, http.StatusUnprocessableEntity:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	case http.StatusNotFound:
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, err.Error(), nil)
	default:
		h.logger.Error("repository set request failed", zap.String("action", action), zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to "+action, nil)
	}
}

func (h *RepositorySetHandlers) wsListRepositorySets(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	sets, err := h.service.ListRepositorySets(ctx, req.WorkspaceID)
	if err != nil {
		return h.wsRepositorySetError(msg, "list repository sets", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, repositorySetsToListResponse(sets))
}

func (h *RepositorySetHandlers) wsCreateRepositorySet(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req repositorySetCreateBody
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if errMsg, blocked := h.wsRejectReadOnlyWorkspace(ctx, msg, req.WorkspaceID); blocked {
		return errMsg, nil
	}
	request, err := req.serviceRequest(req.WorkspaceID)
	if err != nil {
		return h.wsRepositorySetError(msg, "create repository set", err)
	}
	set, err := h.service.CreateRepositorySet(ctx, request)
	if err != nil {
		return h.wsRepositorySetError(msg, "create repository set", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositorySet(set))
}

func (h *RepositorySetHandlers) wsGetRepositorySet(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	set, err := h.service.GetRepositorySet(ctx, req.ID)
	if err != nil {
		return h.wsRepositorySetError(msg, "get repository set", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositorySet(set))
}

func (h *RepositorySetHandlers) wsUpdateRepositorySet(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ID string `json:"id"`
		repositorySetUpdateBody
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if errMsg, blocked := h.wsRejectReadOnlySet(ctx, msg, req.ID); blocked {
		return errMsg, nil
	}
	updated, err := h.service.UpdateRepositorySet(ctx, req.ID, &service.UpdateRepositorySetRequest{
		Name:          req.Name,
		Description:   req.Description,
		RepositoryIDs: req.RepositoryIDs,
		Repositories:  req.Repositories,
	})
	if err != nil {
		return h.wsRepositorySetError(msg, "update repository set", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositorySet(updated))
}

func (h *RepositorySetHandlers) wsDeleteRepositorySet(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		ID string `json:"id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if errMsg, blocked := h.wsRejectReadOnlySet(ctx, msg, req.ID); blocked {
		return errMsg, nil
	}
	if err := h.service.DeleteRepositorySet(ctx, req.ID); err != nil {
		return h.wsRepositorySetError(msg, "delete repository set", err)
	}
	return ws.NewResponse(msg.ID, msg.Action, gin.H{"id": req.ID, "deleted": true})
}
