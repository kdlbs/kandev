package controller

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// WorkflowStepLister provides access to workflow steps for boards with workflow templates.
// This allows the board controller to return workflow steps in snapshots.
type WorkflowStepLister interface {
	ListStepsByBoard(ctx context.Context, boardID string) ([]*workflowmodels.WorkflowStep, error)
}

type BoardController struct {
	service            *service.Service
	workflowStepLister WorkflowStepLister
}

func NewBoardController(svc *service.Service) *BoardController {
	return &BoardController{service: svc}
}

// SetWorkflowStepLister sets the workflow step lister for returning workflow steps.
func (c *BoardController) SetWorkflowStepLister(lister WorkflowStepLister) {
	c.workflowStepLister = lister
}

// toWorkflowStepDTO converts a workflow step to a WorkflowStepDTO.
func toWorkflowStepDTO(step *workflowmodels.WorkflowStep) dto.WorkflowStepDTO {
	return dto.WorkflowStepDTO{
		ID:              step.ID,
		BoardID:         step.BoardID,
		Name:            step.Name,
		StepType:        string(step.StepType),
		Position:        step.Position,
		State:           step.TaskState,
		Color:           step.Color,
		AutoStartAgent:  step.AutoStartAgent,
		PlanMode:        step.PlanMode,
		RequireApproval: step.RequireApproval,
		PromptPrefix:    step.PromptPrefix,
		PromptSuffix:    step.PromptSuffix,
		AllowManualMove: step.AllowManualMove,
		CreatedAt:       step.CreatedAt,
		UpdatedAt:       step.UpdatedAt,
	}
}

// getStepsForBoard returns workflow steps for a board.
func (c *BoardController) getStepsForBoard(ctx context.Context, board *models.Board) ([]dto.WorkflowStepDTO, error) {
	if c.workflowStepLister == nil {
		return nil, fmt.Errorf("workflow step lister not configured")
	}
	steps, err := c.workflowStepLister.ListStepsByBoard(ctx, board.ID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.WorkflowStepDTO, 0, len(steps))
	for _, step := range steps {
		result = append(result, toWorkflowStepDTO(step))
	}
	return result, nil
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
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
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

func (c *BoardController) GetSnapshot(ctx context.Context, req dto.GetBoardSnapshotRequest) (dto.BoardSnapshotDTO, error) {
	board, err := c.service.GetBoard(ctx, req.BoardID)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}
	steps, err := c.getStepsForBoard(ctx, board)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}
	tasks, err := c.service.ListTasks(ctx, req.BoardID)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}

	// Fetch primary session IDs for all tasks in one query
	taskDTOs, err := c.convertTasksWithPrimarySessions(ctx, tasks)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}

	return dto.BoardSnapshotDTO{
		Board: dto.FromBoard(board),
		Steps: steps,
		Tasks: taskDTOs,
	}, nil
}

func (c *BoardController) GetWorkspaceSnapshot(ctx context.Context, req dto.GetWorkspaceBoardSnapshotRequest) (dto.BoardSnapshotDTO, error) {
	boardID := req.BoardID
	if boardID == "" {
		boards, err := c.service.ListBoards(ctx, req.WorkspaceID)
		if err != nil {
			return dto.BoardSnapshotDTO{}, err
		}
		if len(boards) == 0 {
			return dto.BoardSnapshotDTO{}, fmt.Errorf("no boards found for workspace: %s", req.WorkspaceID)
		}
		boardID = boards[0].ID
	}

	board, err := c.service.GetBoard(ctx, boardID)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}
	if board.WorkspaceID != req.WorkspaceID {
		return dto.BoardSnapshotDTO{}, fmt.Errorf("board does not belong to workspace: %s", req.WorkspaceID)
	}

	steps, err := c.getStepsForBoard(ctx, board)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}
	tasks, err := c.service.ListTasks(ctx, boardID)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}

	// Fetch primary session IDs for all tasks in one query
	taskDTOs, err := c.convertTasksWithPrimarySessions(ctx, tasks)
	if err != nil {
		return dto.BoardSnapshotDTO{}, err
	}

	return dto.BoardSnapshotDTO{
		Board: dto.FromBoard(board),
		Steps: steps,
		Tasks: taskDTOs,
	}, nil
}

// convertTasksWithPrimarySessions converts task models to DTOs with primary session IDs.
func (c *BoardController) convertTasksWithPrimarySessions(ctx context.Context, tasks []*models.Task) ([]dto.TaskDTO, error) {
	if len(tasks) == 0 {
		return []dto.TaskDTO{}, nil
	}

	// Collect all task IDs
	taskIDs := make([]string, len(tasks))
	for i, task := range tasks {
		taskIDs[i] = task.ID
	}

	// Fetch primary session IDs in bulk
	primarySessionMap, err := c.service.GetPrimarySessionIDsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}

	// Convert tasks to DTOs with primary session IDs
	result := make([]dto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		var primarySessionID *string
		if sid, ok := primarySessionMap[task.ID]; ok {
			primarySessionID = &sid
		}
		result = append(result, dto.FromTaskWithPrimarySession(task, primarySessionID))
	}
	return result, nil
}
