package controller

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// WorkflowStepLister provides access to workflow steps for workflows with workflow templates.
// This allows the workflow controller to return workflow steps in snapshots.
type WorkflowStepLister interface {
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*workflowmodels.WorkflowStep, error)
}

type WorkflowController struct {
	service            *service.Service
	workflowStepLister WorkflowStepLister
}

func NewWorkflowController(svc *service.Service) *WorkflowController {
	return &WorkflowController{service: svc}
}

// SetWorkflowStepLister sets the workflow step lister for returning workflow steps.
func (c *WorkflowController) SetWorkflowStepLister(lister WorkflowStepLister) {
	c.workflowStepLister = lister
}

// toWorkflowStepDTO converts a workflow step to a WorkflowStepDTO.
func toWorkflowStepDTO(step *workflowmodels.WorkflowStep) dto.WorkflowStepDTO {
	result := WorkflowStepToDTO(step)
	result.CreatedAt = step.CreatedAt
	result.UpdatedAt = step.UpdatedAt
	return result
}

// getStepsForWorkflow returns workflow steps for a workflow.
func (c *WorkflowController) getStepsForWorkflow(ctx context.Context, workflow *models.Workflow) ([]dto.WorkflowStepDTO, error) {
	if c.workflowStepLister == nil {
		return nil, fmt.Errorf("workflow step lister not configured")
	}
	steps, err := c.workflowStepLister.ListStepsByWorkflow(ctx, workflow.ID)
	if err != nil {
		return nil, err
	}
	result := make([]dto.WorkflowStepDTO, 0, len(steps))
	for _, step := range steps {
		result = append(result, toWorkflowStepDTO(step))
	}
	return result, nil
}

func (c *WorkflowController) ListWorkflows(ctx context.Context, req dto.ListWorkflowsRequest) (dto.ListWorkflowsResponse, error) {
	workflows, err := c.service.ListWorkflows(ctx, req.WorkspaceID)
	if err != nil {
		return dto.ListWorkflowsResponse{}, err
	}
	resp := dto.ListWorkflowsResponse{
		Workflows: make([]dto.WorkflowDTO, 0, len(workflows)),
		Total:     len(workflows),
	}
	for _, workflow := range workflows {
		resp.Workflows = append(resp.Workflows, dto.FromWorkflow(workflow))
	}
	return resp, nil
}

func (c *WorkflowController) GetWorkflow(ctx context.Context, req dto.GetWorkflowRequest) (dto.WorkflowDTO, error) {
	workflow, err := c.service.GetWorkflow(ctx, req.ID)
	if err != nil {
		return dto.WorkflowDTO{}, err
	}
	return dto.FromWorkflow(workflow), nil
}

func (c *WorkflowController) CreateWorkflow(ctx context.Context, req dto.CreateWorkflowRequest) (dto.WorkflowDTO, error) {
	workflow, err := c.service.CreateWorkflow(ctx, &service.CreateWorkflowRequest{
		WorkspaceID:        req.WorkspaceID,
		Name:               req.Name,
		Description:        req.Description,
		WorkflowTemplateID: req.WorkflowTemplateID,
	})
	if err != nil {
		return dto.WorkflowDTO{}, err
	}
	return dto.FromWorkflow(workflow), nil
}

func (c *WorkflowController) UpdateWorkflow(ctx context.Context, req dto.UpdateWorkflowRequest) (dto.WorkflowDTO, error) {
	workflow, err := c.service.UpdateWorkflow(ctx, req.ID, &service.UpdateWorkflowRequest{
		Name:        req.Name,
		Description: req.Description,
	})
	if err != nil {
		return dto.WorkflowDTO{}, err
	}
	return dto.FromWorkflow(workflow), nil
}

func (c *WorkflowController) DeleteWorkflow(ctx context.Context, req dto.DeleteWorkflowRequest) (dto.SuccessResponse, error) {
	if err := c.service.DeleteWorkflow(ctx, req.ID); err != nil {
		return dto.SuccessResponse{}, err
	}
	return dto.SuccessResponse{Success: true}, nil
}

func (c *WorkflowController) GetSnapshot(ctx context.Context, req dto.GetWorkflowSnapshotRequest) (dto.WorkflowSnapshotDTO, error) {
	workflow, err := c.service.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return dto.WorkflowSnapshotDTO{}, err
	}
	steps, err := c.getStepsForWorkflow(ctx, workflow)
	if err != nil {
		return dto.WorkflowSnapshotDTO{}, err
	}
	tasks, err := c.service.ListTasks(ctx, req.WorkflowID)
	if err != nil {
		return dto.WorkflowSnapshotDTO{}, err
	}

	// Fetch primary session IDs for all tasks in one query
	taskDTOs, err := c.convertTasksWithPrimarySessions(ctx, tasks)
	if err != nil {
		return dto.WorkflowSnapshotDTO{}, err
	}

	return dto.WorkflowSnapshotDTO{
		Workflow: dto.FromWorkflow(workflow),
		Steps:   steps,
		Tasks:   taskDTOs,
	}, nil
}

func (c *WorkflowController) GetWorkspaceSnapshot(ctx context.Context, req dto.GetWorkspaceSnapshotRequest) (dto.WorkflowSnapshotDTO, error) {
	workflowID := req.WorkflowID
	if workflowID == "" {
		workflows, err := c.service.ListWorkflows(ctx, req.WorkspaceID)
		if err != nil {
			return dto.WorkflowSnapshotDTO{}, err
		}
		if len(workflows) == 0 {
			return dto.WorkflowSnapshotDTO{}, fmt.Errorf("no workflows found for workspace: %s", req.WorkspaceID)
		}
		workflowID = workflows[0].ID
	}

	workflow, err := c.service.GetWorkflow(ctx, workflowID)
	if err != nil {
		return dto.WorkflowSnapshotDTO{}, err
	}
	if workflow.WorkspaceID != req.WorkspaceID {
		return dto.WorkflowSnapshotDTO{}, fmt.Errorf("workflow does not belong to workspace: %s", req.WorkspaceID)
	}

	steps, err := c.getStepsForWorkflow(ctx, workflow)
	if err != nil {
		return dto.WorkflowSnapshotDTO{}, err
	}
	tasks, err := c.service.ListTasks(ctx, workflowID)
	if err != nil {
		return dto.WorkflowSnapshotDTO{}, err
	}

	// Fetch primary session IDs for all tasks in one query
	taskDTOs, err := c.convertTasksWithPrimarySessions(ctx, tasks)
	if err != nil {
		return dto.WorkflowSnapshotDTO{}, err
	}

	return dto.WorkflowSnapshotDTO{
		Workflow: dto.FromWorkflow(workflow),
		Steps:   steps,
		Tasks:   taskDTOs,
	}, nil
}

// convertTasksWithPrimarySessions converts task models to DTOs with primary session IDs.
func (c *WorkflowController) convertTasksWithPrimarySessions(ctx context.Context, tasks []*models.Task) ([]dto.TaskDTO, error) {
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

	// Fetch session counts in bulk
	sessionCountMap, err := c.service.GetSessionCountsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}

	// Fetch primary session info (review status) in bulk
	primarySessionInfoMap, err := c.service.GetPrimarySessionInfoForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}

	// Convert tasks to DTOs with session information
	result := make([]dto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		var primarySessionID *string
		if sid, ok := primarySessionMap[task.ID]; ok {
			primarySessionID = &sid
		}

		var sessionCount *int
		if count, ok := sessionCountMap[task.ID]; ok {
			sessionCount = &count
		}

		var reviewStatus *string
		if sessionInfo, ok := primarySessionInfoMap[task.ID]; ok && sessionInfo.ReviewStatus != nil {
			reviewStatus = sessionInfo.ReviewStatus
		}

		result = append(result, dto.FromTaskWithSessionInfo(task, primarySessionID, sessionCount, reviewStatus))
	}
	return result, nil
}
