package wshandlers

import (
	"context"

	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// CreateBoardRequest is the payload for board.create
type CreateBoardRequest struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// BoardResponse is the response for board operations
type BoardResponse struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// ListBoardsResponse is the response for board.list
type ListBoardsResponse struct {
	Boards []BoardResponse `json:"boards"`
	Total  int             `json:"total"`
}

// CreateBoard handles board.create action
func (h *Handlers) CreateBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req CreateBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	svcReq := &service.CreateBoardRequest{
		Name:        req.Name,
		Description: req.Description,
	}

	board, err := h.service.CreateBoard(ctx, svcReq)
	if err != nil {
		h.logger.Error("failed to create board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create board", nil)
	}

	resp := BoardResponse{
		ID:          board.ID,
		Name:        board.Name,
		Description: board.Description,
		CreatedAt:   board.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   board.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// ListBoards handles board.list action
func (h *Handlers) ListBoards(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	boards, err := h.service.ListBoards(ctx)
	if err != nil {
		h.logger.Error("failed to list boards", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list boards", nil)
	}

	resp := ListBoardsResponse{
		Boards: make([]BoardResponse, len(boards)),
		Total:  len(boards),
	}
	for i, b := range boards {
		resp.Boards[i] = BoardResponse{
			ID:          b.ID,
			Name:        b.Name,
			Description: b.Description,
			CreatedAt:   b.CreatedAt.Format("2006-01-02T15:04:05Z"),
			UpdatedAt:   b.UpdatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// GetBoardRequest is the payload for board.get
type GetBoardRequest struct {
	ID string `json:"id"`
}

// GetBoard handles board.get action
func (h *Handlers) GetBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GetBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	board, err := h.service.GetBoard(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Board not found", nil)
	}

	resp := BoardResponse{
		ID:          board.ID,
		Name:        board.Name,
		Description: board.Description,
		CreatedAt:   board.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   board.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// UpdateBoardRequest is the payload for board.update
type UpdateBoardRequest struct {
	ID          string  `json:"id"`
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// UpdateBoard handles board.update action
func (h *Handlers) UpdateBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req UpdateBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	svcReq := &service.UpdateBoardRequest{
		Name:        req.Name,
		Description: req.Description,
	}

	board, err := h.service.UpdateBoard(ctx, req.ID, svcReq)
	if err != nil {
		h.logger.Error("failed to update board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to update board", nil)
	}

	resp := BoardResponse{
		ID:          board.ID,
		Name:        board.Name,
		Description: board.Description,
		CreatedAt:   board.CreatedAt.Format("2006-01-02T15:04:05Z"),
		UpdatedAt:   board.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// DeleteBoardRequest is the payload for board.delete
type DeleteBoardRequest struct {
	ID string `json:"id"`
}

// DeleteBoard handles board.delete action
func (h *Handlers) DeleteBoard(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req DeleteBoardRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	if err := h.service.DeleteBoard(ctx, req.ID); err != nil {
		h.logger.Error("failed to delete board", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to delete board", nil)
	}

	return ws.NewResponse(msg.ID, msg.Action, map[string]bool{"success": true})
}
