package controller

import (
	"context"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/service"
)

type BoardController struct {
	service *service.Service
}

func NewBoardController(svc *service.Service) *BoardController {
	return &BoardController{service: svc}
}

func (c *BoardController) ListBoards(ctx context.Context, req dto.ListBoardsRequest) (dto.ListBoardsResponse, error) {
	boards, err := c.service.ListBoards(ctx, req.WorkspaceID)
	if err != nil {
		return dto.ListBoardsResponse{}, err
	}
	resp := dto.ListBoardsResponse{
		Boards: make([]dto.BoardDTO, 0, len(boards)),
		Total:  len(boards),
	}
	for _, board := range boards {
		resp.Boards = append(resp.Boards, dto.FromBoard(board))
	}
	return resp, nil
}

func (c *BoardController) GetBoard(ctx context.Context, req dto.GetBoardRequest) (dto.BoardDTO, error) {
	board, err := c.service.GetBoard(ctx, req.ID)
	if err != nil {
		return dto.BoardDTO{}, err
	}
	return dto.FromBoard(board), nil
}

func (c *BoardController) CreateBoard(ctx context.Context, req dto.CreateBoardRequest) (dto.BoardDTO, error) {
	board, err := c.service.CreateBoard(ctx, &service.CreateBoardRequest{
		WorkspaceID: req.WorkspaceID,
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		return dto.BoardDTO{}, err
	}
	return dto.FromBoard(board), nil
}

func (c *BoardController) UpdateBoard(ctx context.Context, req dto.UpdateBoardRequest) (dto.BoardDTO, error) {
	board, err := c.service.UpdateBoard(ctx, req.ID, &service.UpdateBoardRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		return dto.BoardDTO{}, err
	}
	return dto.FromBoard(board), nil
}

func (c *BoardController) DeleteBoard(ctx context.Context, req dto.DeleteBoardRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteBoard(ctx, req.ID); err != nil {
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}

func (c *BoardController) ListColumns(ctx context.Context, req dto.ListColumnsRequest) (dto.ListColumnsResponse, error) {
	columns, err := c.service.ListColumns(ctx, req.BoardID)
	if err != nil {
		return dto.ListColumnsResponse{}, err
	}
	resp := dto.ListColumnsResponse{
		Columns: make([]dto.ColumnDTO, 0, len(columns)),
		Total:   len(columns),
	}
	for _, column := range columns {
		resp.Columns = append(resp.Columns, dto.FromColumn(column))
	}
	return resp, nil
}

func (c *BoardController) GetColumn(ctx context.Context, req dto.GetColumnRequest) (dto.ColumnDTO, error) {
	column, err := c.service.GetColumn(ctx, req.ID)
	if err != nil {
		return dto.ColumnDTO{}, err
	}
	return dto.FromColumn(column), nil
}

func (c *BoardController) CreateColumn(ctx context.Context, req dto.CreateColumnRequest) (dto.ColumnDTO, error) {
	column, err := c.service.CreateColumn(ctx, &service.CreateColumnRequest{
		BoardID:  req.BoardID,
		Name:     req.Name,
		Position: req.Position,
		State:    req.State,
		Color:    req.Color,
	})
	if err != nil {
		return dto.ColumnDTO{}, err
	}
	return dto.FromColumn(column), nil
}

func (c *BoardController) UpdateColumn(ctx context.Context, req dto.UpdateColumnRequest) (dto.ColumnDTO, error) {
	column, err := c.service.UpdateColumn(ctx, req.ID, &service.UpdateColumnRequest{
		Name:     req.Name,
		Position: req.Position,
		State:    req.State,
		Color:    req.Color,
	})
	if err != nil {
		return dto.ColumnDTO{}, err
	}
	return dto.FromColumn(column), nil
}

func (c *BoardController) DeleteColumn(ctx context.Context, req dto.GetColumnRequest) error {
	return c.service.DeleteColumn(ctx, req.ID)
}

func (c *BoardController) GetSnapshot(ctx context.Context, req dto.GetBoardSnapshotRequest) (dto.BoardSnapshotDTO, error) {
	board, err := c.service.GetBoard(ctx, req.BoardID)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}
	columns, err := c.service.ListColumns(ctx, req.BoardID)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}
	tasks, err := c.service.ListTasks(ctx, req.BoardID)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}

	resp := dto.BoardSnapshotDTO{
		Board:   dto.FromBoard(board),
		Columns: make([]dto.ColumnDTO, 0, len(columns)),
		Tasks:   make([]dto.TaskDTO, 0, len(tasks)),
	}
	for _, column := range columns {
		resp.Columns = append(resp.Columns, dto.FromColumn(column))
	}
	for _, task := range tasks {
		resp.Tasks = append(resp.Tasks, dto.FromTask(task))
	}
	return resp, nil
}
