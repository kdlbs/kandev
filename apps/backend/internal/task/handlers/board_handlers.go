package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

type BoardHandlers struct {
	controller *controller.BoardController
	logger     *logger.Logger
}

func NewBoardHandlers(svc *controller.BoardController, log *logger.Logger) *BoardHandlers {
	return &BoardHandlers{
		controller: svc,
		logger:     log.WithFields(zap.String("component", "task-board-handlers")),
	}
}

func RegisterBoardRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, ctrl *controller.BoardController, log *logger.Logger) {
	handlers := NewBoardHandlers(ctrl, log)
	handlers.registerHTTP(router)
	handlers.registerWS(dispatcher)
}

func (h *BoardHandlers) registerHTTP(router *gin.Engine) {
	api := router.Group("/api/v1")
	api.GET("/boards", h.httpListBoards)
	api.GET("/workspaces/:id/boards", h.httpListBoardsByWorkspace)
	api.GET("/workspaces/:id/board-snapshot", h.httpGetWorkspaceBoardSnapshot)
	api.GET("/boards/:id", h.httpGetBoard)
	api.GET("/boards/:id/snapshot", h.httpGetBoardSnapshot)
	api.POST("/boards", h.httpCreateBoard)
	api.PATCH("/boards/:id", h.httpUpdateBoard)
	api.DELETE("/boards/:id", h.httpDeleteBoard)
	// Column endpoints removed - use workflow steps instead
}

func (h *BoardHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionBoardList, h.wsListBoards)
	dispatcher.RegisterFunc(ws.ActionBoardCreate, h.wsCreateBoard)
	dispatcher.RegisterFunc(ws.ActionBoardGet, h.wsGetBoard)
	dispatcher.RegisterFunc(ws.ActionBoardUpdate, h.wsUpdateBoard)
	dispatcher.RegisterFunc(ws.ActionBoardDelete, h.wsDeleteBoard)
	// Column WS actions removed - use workflow steps instead
}

// HTTP handlers

func (h *BoardHandlers) httpListBoards(c *gin.Context) {
	resp, err := h.controller.ListBoards(c.Request.Context(), dto.ListBoardsRequest{
		WorkspaceID: c.Query("workspace_id"),
	})
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list boards"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BoardHandlers) httpListBoardsByWorkspace(c *gin.Context) {
	resp, err := h.controller.ListBoards(c.Request.Context(), dto.ListBoardsRequest{
		WorkspaceID: c.Param("id"),
	})
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list boards"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BoardHandlers) httpGetBoard(c *gin.Context) {
	resp, err := h.controller.GetBoard(c.Request.Context(), dto.GetBoardRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateBoardRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

func (h *BoardHandlers) httpCreateBoard(c *gin.Context) {
	var body httpCreateBoardRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.Name == "" || body.WorkspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id and name are required"})
		return
	}
	resp, err := h.controller.CreateBoard(c.Request.Context(), dto.CreateBoardRequest{
		WorkspaceID:        body.WorkspaceID,
		Name:               body.Name,
		Description:        body.Description,
		WorkflowTemplateID: body.WorkflowTemplateID,
	})
	if err != nil {
		h.logger.Error("failed to create board", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create board"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

type httpUpdateBoardRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
}

func (h *BoardHandlers) httpUpdateBoard(c *gin.Context) {
	var body httpUpdateBoardRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	resp, err := h.controller.UpdateBoard(c.Request.Context(), dto.UpdateBoardRequest{
		ID:          c.Param("id"),
		Name:        body.Name,
		Description: body.Description,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BoardHandlers) httpDeleteBoard(c *gin.Context) {
	resp, err := h.controller.DeleteBoard(c.Request.Context(), dto.DeleteBoardRequest{ID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BoardHandlers) httpGetBoardSnapshot(c *gin.Context) {
	resp, err := h.controller.GetSnapshot(c.Request.Context(), dto.GetBoardSnapshotRequest{BoardID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BoardHandlers) httpGetWorkspaceBoardSnapshot(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace id is required"})
		return
	}
	boardID := c.Query("board_id")
	resp, err := h.controller.GetWorkspaceSnapshot(c.Request.Context(), dto.GetWorkspaceBoardSnapshotRequest{
		WorkspaceID: workspaceID,
		BoardID:     boardID,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "board not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

// WS handlers

func (h *BoardHandlers) wsListBoards(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req struct {
		WorkspaceID string `json:"workspace_id,omitempty"`
	}
	if msg.Payload != nil {
		if err := msg.ParsePayload(&req); err != nil {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
		}
	}
	resp, err := h.controller.ListBoards(ctx, dto.ListBoardsRequest{WorkspaceID: req.WorkspaceID})
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list boards", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateBoardRequest struct {
	WorkspaceID        string  `json:"workspace_id"`
	Name               string  `json:"name"`
	Description        string  `json:"description,omitempty"`
	WorkflowTemplateID *string `json:"workflow_template_id,omitempty"`
}

func (h *BoardHandlers) wsCreateBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}
	if req.WorkspaceID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "workspace_id is required", nil)
	}

	resp, err := h.controller.CreateBoard(ctx, dto.CreateBoardRequest{
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
	})
	if err != nil {
		h.logger.Error("failed to create board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create board", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetBoardRequest struct {
	ID string `json:"id"`
}

func (h *BoardHandlers) wsGetBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.GetBoard(ctx, dto.GetBoardRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Board not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateBoardRequest struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

func (h *BoardHandlers) wsUpdateBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.UpdateBoard(ctx, dto.UpdateBoardRequest{
		ID:          req.ID,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		h.logger.Error("failed to update board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update board", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsDeleteBoardRequest struct {
	ID string `json:"id"`
}

func (h *BoardHandlers) wsDeleteBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.DeleteBoard(ctx, dto.DeleteBoardRequest{ID: req.ID})
	if err != nil {
		h.logger.Error("failed to delete board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete board", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}
