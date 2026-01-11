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
	api.GET("/boards/:id", h.httpGetBoard)
	api.GET("/boards/:id/columns", h.httpListColumns)
	api.GET("/boards/:id/snapshot", h.httpGetBoardSnapshot)
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
}

// HTTP handlers

func (h *BoardHandlers) httpListBoards(c *gin.Context) {
	resp, err := h.controller.ListBoards(c.Request.Context(), dto.ListBoardsRequest{})
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

func (h *BoardHandlers) httpListColumns(c *gin.Context) {
	resp, err := h.controller.ListColumns(c.Request.Context(), dto.ListColumnsRequest{BoardID: c.Param("id")})
	if err != nil {
		handleNotFound(c, h.logger, err, "columns not found")
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

// WS handlers

func (h *BoardHandlers) wsListBoards(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	resp, err := h.controller.ListBoards(ctx, dto.ListBoardsRequest{})
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list boards", nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, resp)
}

type wsCreateBoardRequest struct {
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

	resp, err := h.controller.CreateBoard(ctx, dto.CreateBoardRequest{
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
