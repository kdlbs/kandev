package wshandlers

import (
	"context"

	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	ws "github.com/kandev/kandev/pkg/websocket"
	"go.uber.org/zap"
)

// CreateColumnRequest is the payload for column.create
type CreateColumnRequest struct {
	BoardID  string `json:"board_id"`
	Name     string `json:"name"`
	Position int    `json:"position"`
	State    string `json:"state,omitempty"`
}

// ColumnResponse is the response for column operations
type ColumnResponse struct {
	ID        string `json:"id"`
	BoardID   string `json:"board_id"`
	Name      string `json:"name"`
	Position  int    `json:"position"`
	State     string `json:"state"`
	CreatedAt string `json:"created_at"`
}

// ListColumnsResponse is the response for column.list
type ListColumnsResponse struct {
	Columns []ColumnResponse `json:"columns"`
	Total   int              `json:"total"`
}

// ListColumnsRequest is the payload for column.list
type ListColumnsRequest struct {
	BoardID string `json:"board_id"`
}

// ListColumns handles column.list action
func (h *Handlers) ListColumns(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req ListColumnsRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}

	columns, err := h.service.ListColumns(ctx, req.BoardID)
	if err != nil {
		h.logger.Error("failed to list columns", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to list columns", nil)
	}

	resp := ListColumnsResponse{
		Columns: make([]ColumnResponse, len(columns)),
		Total:   len(columns),
	}
	for i, c := range columns {
		resp.Columns[i] = ColumnResponse{
			ID:        c.ID,
			BoardID:   c.BoardID,
			Name:      c.Name,
			Position:  c.Position,
			State:     string(c.State),
			CreatedAt: c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// CreateColumn handles column.create action
func (h *Handlers) CreateColumn(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req CreateColumnRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.BoardID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "board_id is required", nil)
	}
	if req.Name == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "name is required", nil)
	}

	// Default state to TODO if not specified
	state := v1.TaskState(req.State)
	if state == "" {
		state = v1.TaskStateTODO
	}

	svcReq := &service.CreateColumnRequest{
		BoardID:  req.BoardID,
		Name:     req.Name,
		Position: req.Position,
		State:    state,
	}

	column, err := h.service.CreateColumn(ctx, svcReq)
	if err != nil {
		h.logger.Error("failed to create column", zap.Error(err))
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "Failed to create column", nil)
	}

	resp := ColumnResponse{
		ID:        column.ID,
		BoardID:   column.BoardID,
		Name:      column.Name,
		Position:  column.Position,
		State:     string(column.State),
		CreatedAt: column.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

// GetColumnRequest is the payload for column.get
type GetColumnRequest struct {
	ID string `json:"id"`
}

// GetColumn handles column.get action
func (h *Handlers) GetColumn(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req GetColumnRequest
	if err := msg.ParsePayload(&req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "Invalid payload: "+err.Error(), nil)
	}

	if req.ID == "" {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, "id is required", nil)
	}

	column, err := h.service.GetColumn(ctx, req.ID)
	if err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "Column not found", nil)
	}

	resp := ColumnResponse{
		ID:        column.ID,
		BoardID:   column.BoardID,
		Name:      column.Name,
		Position:  column.Position,
		State:     string(column.State),
		CreatedAt: column.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}

	return ws.NewResponse(msg.ID, msg.Action, resp)
}

