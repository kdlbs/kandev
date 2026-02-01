package controller

import (
	"context"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

type TaskController struct {
	service *service.Service
}

func NewTaskController(svc *service.Service) *TaskController {
	return &TaskController{service: svc}
}

func (c *TaskController) ListTasks(ctx context.Context, req dto.ListTasksRequest) (dto.ListTasksResponse, error) {
	tasks, err := c.service.ListTasks(ctx, req.BoardID)
	if err != nil {
		return dto.ListTasksResponse{}, err
	}
	resp := dto.ListTasksResponse{
		Tasks: make([]dto.TaskDTO, 0, len(tasks)),
		Total: len(tasks),
	}
	for _, task := range tasks {
		taskDTO := dto.FromTask(task)
		resp.Tasks = append(resp.Tasks, taskDTO)
	}
	return resp, nil
}

func (c *TaskController) ListTasksByWorkspace(ctx context.Context, req dto.ListTasksByWorkspaceRequest) (dto.ListTasksResponse, error) {
	tasks, total, err := c.service.ListTasksByWorkspace(ctx, req.WorkspaceID, req.Query, req.Page, req.PageSize)
	if err != nil {
		return dto.ListTasksResponse{}, err
	}

	// Get primary session IDs for all tasks
	taskIDs := make([]string, 0, len(tasks))
	for _, task := range tasks {
		taskIDs = append(taskIDs, task.ID)
	}

	primarySessionMap, err := c.service.GetPrimarySessionIDsForTasks(ctx, taskIDs)
	if err != nil {
		// Log error but continue without primary session IDs
		primarySessionMap = make(map[string]string)
	}

	resp := dto.ListTasksResponse{
		Tasks: make([]dto.TaskDTO, 0, len(tasks)),
		Total: total,
	}
	for _, task := range tasks {
		var primarySessionID *string
		if id, ok := primarySessionMap[task.ID]; ok {
			primarySessionID = &id
		}
		taskDTO := dto.FromTaskWithPrimarySession(task, primarySessionID)
		resp.Tasks = append(resp.Tasks, taskDTO)
	}
	return resp, nil
}

func (c *TaskController) ListTaskSessions(ctx context.Context, req dto.ListTaskSessionsRequest) (dto.ListTaskSessionsResponse, error) {
	sessions, err := c.service.ListTaskSessions(ctx, req.TaskID)
	if err != nil {
		return dto.ListTaskSessionsResponse{}, err
	}
	resp := dto.ListTaskSessionsResponse{
		Sessions: make([]dto.TaskSessionDTO, 0, len(sessions)),
		Total:    len(sessions),
	}
	for _, session := range sessions {
		resp.Sessions = append(resp.Sessions, dto.FromTaskSession(session))
	}
	return resp, nil
}

func (c *TaskController) GetTaskSession(ctx context.Context, req dto.GetTaskSessionRequest) (dto.GetTaskSessionResponse, error) {
	session, err := c.service.GetTaskSession(ctx, req.TaskSessionID)
	if err != nil {
		return dto.GetTaskSessionResponse{}, err
	}
	return dto.GetTaskSessionResponse{
		Session: dto.FromTaskSession(session),
	}, nil
}

func (c *TaskController) GetTask(ctx context.Context, req dto.GetTaskRequest) (dto.TaskDTO, error) {
	task, err := c.service.GetTask(ctx, req.ID)
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}

func (c *TaskController) CreateTask(ctx context.Context, req dto.CreateTaskRequest) (dto.TaskDTO, error) {
	// Convert DTO repositories to service layer input
	var repos []service.TaskRepositoryInput
	for _, r := range req.Repositories {
		repos = append(repos, service.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		})
	}

	task, err := c.service.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:    req.WorkspaceID,
		BoardID:        req.BoardID,
		WorkflowStepID: req.WorkflowStepID,
		Title:          req.Title,
		Description:    req.Description,
		Priority:       req.Priority,
		State:          req.State,
		Repositories:   repos,
		Position:       req.Position,
		Metadata:       req.Metadata,
	})
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}

func (c *TaskController) UpdateTask(ctx context.Context, req dto.UpdateTaskRequest) (dto.TaskDTO, error) {
	// Convert DTO repositories to service layer input
	var repos []service.TaskRepositoryInput
	for _, r := range req.Repositories {
		repos = append(repos, service.TaskRepositoryInput{
			RepositoryID:  r.RepositoryID,
			BaseBranch:    r.BaseBranch,
			LocalPath:     r.LocalPath,
			Name:          r.Name,
			DefaultBranch: r.DefaultBranch,
		})
	}

	task, err := c.service.UpdateTask(ctx, req.ID, &service.UpdateTaskRequest{
		Title:        req.Title,
		Description:  req.Description,
		Priority:     req.Priority,
		State:        req.State,
		Repositories: repos,
		Position:     req.Position,
		Metadata:     req.Metadata,
	})
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}

func (c *TaskController) DeleteTask(ctx context.Context, req dto.DeleteTaskRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteTask(ctx, req.ID); err != nil {
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}

func (c *TaskController) MoveTask(ctx context.Context, req dto.MoveTaskRequest) (dto.MoveTaskResponse, error) {
	result, err := c.service.MoveTask(ctx, req.ID, req.BoardID, req.WorkflowStepID, req.Position)
	if err != nil {
		return dto.MoveTaskResponse{}, err
	}

	response := dto.MoveTaskResponse{
		Task: dto.FromTask(result.Task),
	}

	// Include workflow step info if available
	if result.WorkflowStep != nil {
		response.WorkflowStep = dto.WorkflowStepDTO{
			ID:              result.WorkflowStep.ID,
			BoardID:         result.WorkflowStep.BoardID,
			Name:            result.WorkflowStep.Name,
			StepType:        result.WorkflowStep.StepType,
			Position:        result.WorkflowStep.Position,
			Color:           result.WorkflowStep.Color,
			AutoStartAgent:  result.WorkflowStep.AutoStartAgent,
			PlanMode:        result.WorkflowStep.PlanMode,
			RequireApproval: result.WorkflowStep.RequireApproval,
			PromptPrefix:    result.WorkflowStep.PromptPrefix,
			PromptSuffix:    result.WorkflowStep.PromptSuffix,
			AllowManualMove: result.WorkflowStep.AllowManualMove,
		}
	}

	return response, nil
}

func (c *TaskController) UpdateTaskState(ctx context.Context, req dto.UpdateTaskStateRequest) (dto.TaskDTO, error) {
	task, err := c.service.UpdateTaskState(ctx, req.ID, req.State)
	if err != nil {
		return dto.TaskDTO{}, err
	}
	return dto.FromTask(task), nil
}

// GetGitSnapshots retrieves git snapshots for a session
func (c *TaskController) GetGitSnapshots(ctx context.Context, sessionID string, limit int) ([]*models.GitSnapshot, error) {
	return c.service.GetGitSnapshots(ctx, sessionID, limit)
}

// GetSessionCommits retrieves commits for a session
func (c *TaskController) GetSessionCommits(ctx context.Context, sessionID string) ([]*models.SessionCommit, error) {
	return c.service.GetSessionCommits(ctx, sessionID)
}

// GetCumulativeDiff retrieves the cumulative diff from base commit to current HEAD
func (c *TaskController) GetCumulativeDiff(ctx context.Context, sessionID string) (*models.CumulativeDiff, error) {
	return c.service.GetCumulativeDiff(ctx, sessionID)
}

// ApproveSession approves a session's current step and moves it to the on_approval_step_id
func (c *TaskController) ApproveSession(ctx context.Context, req dto.ApproveSessionRequest) (dto.ApproveSessionResponse, error) {
	result, err := c.service.ApproveSession(ctx, req.SessionID)
	if err != nil {
		return dto.ApproveSessionResponse{}, err
	}

	resp := dto.ApproveSessionResponse{
		Success: true,
		Session: dto.FromTaskSession(result.Session),
	}

	// Include the new workflow step if present
	if result.WorkflowStep != nil {
		resp.WorkflowStep = dto.WorkflowStepDTO{
			ID:              result.WorkflowStep.ID,
			BoardID:         result.WorkflowStep.BoardID,
			Name:            result.WorkflowStep.Name,
			StepType:        result.WorkflowStep.StepType,
			Position:        result.WorkflowStep.Position,
			Color:           result.WorkflowStep.Color,
			AutoStartAgent:  result.WorkflowStep.AutoStartAgent,
			PlanMode:        result.WorkflowStep.PlanMode,
			RequireApproval: result.WorkflowStep.RequireApproval,
			PromptPrefix:    result.WorkflowStep.PromptPrefix,
			PromptSuffix:    result.WorkflowStep.PromptSuffix,
			AllowManualMove: result.WorkflowStep.AllowManualMove,
		}
	}

	return resp, nil
}
