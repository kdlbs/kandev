package handlers

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/controller"
	"github.com/kandev/kandev/internal/task/dto"
	v1 "github.com/kandev/kandev/pkg/api/v1"
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
	api.GET("/boards/:id", h.httpGetBoard)
	api.GET("/boards/:id/columns", h.httpListColumns)
	api.GET("/boards/:id/snapshot", h.httpGetBoardSnapshot)
	api.POST("/boards", h.httpCreateBoard)
	api.PATCH("/boards/:id", h.httpUpdateBoard)
	api.DELETE("/boards/:id", h.httpDeleteBoard)
	api.POST("/boards/:id/columns", h.httpCreateColumn)
	api.PUT("/columns/:id", h.httpUpdateColumn)
	api.DELETE("/columns/:id", h.httpDeleteColumn)
}

func (h *BoardHandlers) registerWS(dispatcher *ws.Dispatcher) {
	dispatcher.RegisterFunc(ws.ActionBoardList, h.wsListBoards)
	dispatcher.RegisterFunc(ws.ActionBoardCreate, h.wsCreateBoard)
	dispatcher.RegisterFunc(ws.ActionBoardGet, h.wsGetBoard)
	dispatcher.RegisterFunc(ws.ActionBoardUpdate, h.wsUpdateBoard)
	dispatcher.RegisterFunc(ws.ActionBoardDelete, h.wsDeleteBoard)
	dispatcher.RegisterFunc(ws.ActionColumnList, h.wsListColumns)
	dispatcher.RegisterFunc(ws.ActionColumnCreate, h.wsCreateColumn)
	dispatcher.RegisterFunc(ws.ActionColumnGet, h.wsGetColumn)
	dispatcher.RegisterFunc(ws.ActionColumnUpdate, h.wsUpdateColumn)
	dispatcher.RegisterFunc(ws.ActionColumnDelete, h.wsDeleteColumn)
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
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
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
		WorkspaceID: body.WorkspaceID,
		Name:        body.Name,
		Description: body.Description,
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

func (h *BoardHandlers) httpListColumns(c *gin.Context) {
	resp, err := h.controller.ListColumns(c.Request.Context(), dto.ListColumnsRequest{BoardID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "columns not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

type httpCreateColumnRequest struct {
	Name     string       `json:"name"`
	Position int          `json:"position"`
	State    v1.TaskState `json:"state"`
	Color    string       `json:"color"`
}

func (h *BoardHandlers) httpCreateColumn(c *gin.Context) {
	var body httpCreateColumnRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		h.logger.Error("failed to bind create column request", zap.Error(err), zap.String("board_id", c.Param("id")))
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	if body.Name == "" {
		h.logger.Error("create column missing name", zap.String("board_id", c.Param("id")))
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}
	state := body.State
	if state == "" {
		state = v1.TaskStateTODO
	}
	resp, err := h.controller.CreateColumn(c.Request.Context(), dto.CreateColumnRequest{
		BoardID:  c.Param("id"),
		Name:     body.Name,
		Position: body.Position,
		State:    state,
		Color:    body.Color,
	})
	if err != nil {
		h.logger.Error("failed to create column", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create column"})
		return
	}
	c.JSON(http.StatusCreated, resp)
}

type httpUpdateColumnRequest struct {
	Name     *string       `json:"name"`
	Position *int          `json:"position"`
	State    *v1.TaskState `json:"state"`
	Color    *string       `json:"color"`
}

func (h *BoardHandlers) httpUpdateColumn(c *gin.Context) {
	var body httpUpdateColumnRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	resp, err := h.controller.UpdateColumn(c.Request.Context(), dto.UpdateColumnRequest{
		ID:       c.Param("id"),
		Name:     body.Name,
		Position: body.Position,
		State:    body.State,
		Color:    body.Color,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "column not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *BoardHandlers) httpDeleteColumn(c *gin.Context) {
	if err := h.controller.DeleteColumn(c.Request.Context(), dto.GetColumnRequest{ID: c.Param("id")}); err != nil {
		h.logger.Error("failed to delete column", zap.Error(err), zap.String("column_id", c.Param("id")))
		handleNotFound(c, h.logger, err, "column not found")
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *BoardHandlers) httpGetBoardSnapshot(c *gin.Context) {
	resp, err := h.controller.GetSnapshot(c.Request.Context(), dto.GetBoardSnapshotRequest{BoardID: c.Param("id")})
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
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
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
		WorkspaceID: req.WorkspaceID,
		Name:        req.Name,
		Description: req.Description,
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

type wsListColumnsRequest struct {
	BoardID string `json:"board_id"`
}

func (h *BoardHandlers) wsListColumns(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsListColumnsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}

	resp, err := h.controller.ListColumns(ctx, dto.ListColumnsRequest{BoardID: req.BoardID})
	if err != nil {
		h.logger.Error("failed to list columns", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list columns", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateColumnRequest struct {
	BoardID  string `json:"board_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	State    string `json:"state,omitempty"`
	Color    string `json:"color,omitempty"`
}

func (h *BoardHandlers) wsCreateColumn(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsCreateColumnRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	state := v1.TaskState(req.State)
	if state == "" {
		state = v1.TaskStateTODO
	}

	resp, err := h.controller.CreateColumn(ctx, dto.CreateColumnRequest{
		BoardID:  req.BoardID,
		Name:     req.Name,
		Position: req.Position,
		State:    state,
		Color:    req.Color,
	})
	if err != nil {
		h.logger.Error("failed to create column", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create column", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsGetColumnRequest struct {
	ID string `json:"id"`
}

func (h *BoardHandlers) wsGetColumn(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsGetColumnRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.GetColumn(ctx, dto.GetColumnRequest{ID: req.ID})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Column not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsUpdateColumnRequest struct {
	ID       string       `json:"id"`
	Name     *string      `json:"name,omitempty"`
	Position *int         `json:"position,omitempty"`
	State    *v1.TaskState `json:"state,omitempty"`
	Color    *string      `json:"color,omitempty"`
}

func (h *BoardHandlers) wsUpdateColumn(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsUpdateColumnRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	resp, err := h.controller.UpdateColumn(ctx, dto.UpdateColumnRequest{
		ID:       req.ID,
		Name:     req.Name,
		Position: req.Position,
		State:    req.State,
		Color:    req.Color,
	})
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Column not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsDeleteColumnRequest struct {
	ID string `json:"id"`
}

func (h *BoardHandlers) wsDeleteColumn(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req wsDeleteColumnRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}
	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}
	if err := h.controller.DeleteColumn(ctx, dto.GetColumnRequest{ID: req.ID}); err != nil {
		h.logger.Error("failed to delete column (ws)", zap.Error(err), zap.String("column_id", req.ID))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Column not found", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, gin.H{"deleted": true})
}
