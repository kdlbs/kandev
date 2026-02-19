package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// HTTP handlers

func (h *RepositoryHandlers) httpListRepositoryScripts(c *gin.Context) {
	scripts, err := h.service.ListRepositoryScripts(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to list repository scripts", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list repository scripts"})
		return
	}
	resp := dto.ListRepositoryScriptsResponse{
		Scripts: make([]dto.RepositoryScriptDTO, 0, len(scripts)),
		Total:   len(scripts),
	}
	for _, script := range scripts {
		resp.Scripts = append(resp.Scripts, dto.FromRepositoryScript(script))
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateRepositoryScriptRequest struct {
	Name     string `json:"name"`
	Command  string `json:"command"`
	Position int    `json:"position"`
}

func (h *RepositoryHandlers) httpCreateRepositoryScript(c *gin.Context) {
	var body httpCreateRepositoryScriptRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.Command == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and command are required"})
		return
	}
	script, err := h.service.CreateRepositoryScript(c.Request.Context(), &service.CreateRepositoryScriptRequest{
		RepositoryID: c.Param("id"),
		Name:         body.Name,
		Command:      body.Command,
		Position:     body.Position,
	})
	if err != nil {
		h.logger.Error("failed to create repository script", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create repository script"})
		return
	}
	c.JSON(http.StatusCreated, dto.FromRepositoryScript(script))
}

func (h *RepositoryHandlers) httpGetRepositoryScript(c *gin.Context) {
	script, err := h.service.GetRepositoryScript(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "repository script not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromRepositoryScript(script))
}

type httpUpdateRepositoryScriptRequest struct {
	Name     *string `json:"name"`
	Command  *string `json:"command"`
	Position *int    `json:"position"`
}

func (h *RepositoryHandlers) httpUpdateRepositoryScript(c *gin.Context) {
	var body httpUpdateRepositoryScriptRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	script, err := h.service.UpdateRepositoryScript(c.Request.Context(), c.Param("id"), &service.UpdateRepositoryScriptRequest{
		Name:     body.Name,
		Command:  body.Command,
		Position: body.Position,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "repository script not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromRepositoryScript(script))
}

func (h *RepositoryHandlers) httpDeleteRepositoryScript(c *gin.Context) {
	if err := h.service.DeleteRepositoryScript(c.Request.Context(), c.Param("id")); err != nil {
		handleNotFound(c, h.logger, err, "repository script not found")
		return
	}
	c.Status(http.StatusNoContent)
}

// WS handlers

func scriptsToListResponse(scripts []*models.RepositoryScript) dto.ListRepositoryScriptsResponse {
	resp := dto.ListRepositoryScriptsResponse{
		Scripts: make([]dto.RepositoryScriptDTO, 0, len(scripts)),
		Total:   len(scripts),
	}
	for _, script := range scripts {
		resp.Scripts = append(resp.Scripts, dto.FromRepositoryScript(script))
	}
	return resp
}

func (h *RepositoryHandlers) wsListRepositoryScripts(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		RepositoryID string `json:"repository_id"`
	}
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	scripts, err := h.service.ListRepositoryScripts(ctx, req.RepositoryID)
	if err != nil {
		h.logger.Error("failed to list repository scripts", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list repository scripts", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, scriptsToListResponse(scripts))
}

type wsCreateRepositoryScriptRequest struct {
	RepositoryID string `json:"repository_id"`
	Name         string `json:"name"`
	Command      string `json:"command"`
	Position     int    `json:"position"`
}

func (h *RepositoryHandlers) wsCreateRepositoryScript(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateRepositoryScriptRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.RepositoryID == "" || req.Name == "" || req.Command == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "repository_id, name, and command are required", nil)
	}
	script, err := h.service.CreateRepositoryScript(ctx, &service.CreateRepositoryScriptRequest{
		RepositoryID: req.RepositoryID,
		Name:         req.Name,
		Command:      req.Command,
		Position:     req.Position,
	})
	if err != nil {
		h.logger.Error("failed to create repository script", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create repository script", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositoryScript(script))
}

type wsGetRepositoryScriptRequest struct {
	ID string `json:"id"`
}

func (h *RepositoryHandlers) wsGetRepositoryScript(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetRepositoryScriptRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	script, err := h.service.GetRepositoryScript(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository script not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositoryScript(script))
}

type wsUpdateRepositoryScriptRequest struct {
	ID       string  `json:"id"`
	Name     *string `json:"name,omitempty"`
	Command  *string `json:"command,omitempty"`
	Position *int    `json:"position,omitempty"`
}

func (h *RepositoryHandlers) wsUpdateRepositoryScript(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateRepositoryScriptRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	script, err := h.service.UpdateRepositoryScript(ctx, req.ID, &service.UpdateRepositoryScriptRequest{
		Name:     req.Name,
		Command:  req.Command,
		Position: req.Position,
	})
	if err != nil {
		h.logger.Error("failed to update repository script", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update repository script", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, dto.FromRepositoryScript(script))
}

type wsDeleteRepositoryScriptRequest struct {
	ID string `json:"id"`
}

func (h *RepositoryHandlers) wsDeleteRepositoryScript(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteRepositoryScriptRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if err := h.service.DeleteRepositoryScript(ctx, req.ID); err != nil {
		h.logger.Error("failed to delete repository script", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Repository script not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, gin.H{"deleted": true})
}
