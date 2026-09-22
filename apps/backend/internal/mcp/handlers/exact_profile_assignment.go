package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strings"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	mcpscope "github.com/kandev/kandev/internal/mcp/scope"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"

	ws "github.com/kandev/kandev/pkg/websocket"
)

type assignExactTaskProfileRequest struct {
	TaskID                       string       `json:"task_id"`
	AgentProfileID               string       `json:"agent_profile_id"`
	ExpectedModel                string       `json:"expected_model"`
	ExpectedTaskState            v1.TaskState `json:"expected_task_state"`
	ExpectedWorkflowStepID       string       `json:"expected_workflow_step_id"`
	TargetWorkflowStepID         string       `json:"target_workflow_step_id"`
	ExpectedAssignmentGeneration int64        `json:"expected_assignment_generation"`
	SenderTaskID                 string       `json:"sender_task_id"`
	SenderSessionID              string       `json:"sender_session_id"`
}

func (h *Handlers) handleAssignExactTaskProfile(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var req assignExactTaskProfileRequest
	if err := json.Unmarshal(msg.Payload, &req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid payload: "+err.Error(), nil)
	}
	normalizeExactProfileRequest(&req)
	if err := validateExactProfileRequest(req); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	principal, authorized := h.exactProfileCoordinatorPrincipal(ctx, req)
	if !authorized {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"exact task profile assignment requires the authenticated Coordinator surface", nil)
	}
	if h.exactTaskProfileAssigner == nil || h.taskSvc == nil || h.workflowCtrl == nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeInternalError, "exact task profile assignment is unavailable", nil)
	}
	task, response := h.validateExactProfileTarget(ctx, msg, principal, req)
	if response != nil {
		return response, nil
	}
	result, err := h.exactTaskProfileAssigner.AssignExactTaskProfile(ctx, orchestrator.ExactTaskProfileAssignmentRequest{
		TaskID: req.TaskID, AgentProfileID: req.AgentProfileID, ExpectedModel: req.ExpectedModel,
		Generation:         req.ExpectedAssignmentGeneration + 1,
		ExpectedWorkflowID: task.WorkflowID, ExpectedWorkflowStepID: req.ExpectedWorkflowStepID,
		ExpectedTaskState: req.ExpectedTaskState,
	})
	if err != nil {
		var modelMismatch *orchestrator.ExactProfileModelMismatchError
		if errors.As(err, &modelMismatch) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
				"agent profile model changed; refresh the profile and retry with its exact model", map[string]interface{}{
					"agent_profile_id": req.AgentProfileID,
					"expected_model":   modelMismatch.Expected,
					"actual_model":     modelMismatch.Actual,
				})
		}
		if errors.Is(err, models.ErrExactProfileAssignmentGeneration) ||
			errors.Is(err, orchestrator.ErrExactProfileAssignmentTargetChanged) {
			return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
				"exact task profile assignment rejected because the task or assignment generation changed", nil)
		}
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeValidation, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, map[string]interface{}{
		"operation_id":                     req.TaskID + ":exact-profile:" + jsonInt(result.Generation),
		"task_id":                          req.TaskID,
		"workspace_id":                     task.WorkspaceID,
		"current_workflow_step_id":         task.WorkflowStepID,
		"target_workflow_step_id":          req.TargetWorkflowStepID,
		"task_state":                       task.State,
		"changed":                          result.Changed,
		"assignment_generation":            result.Generation,
		"after_effective_agent_profile_id": result.AgentProfileID,
		"after_effective_model":            result.Model,
		"profile_revision":                 result.Revision,
		"launch_behavior":                  "future starts use this exact profile/model; this call did not move the task or start a session",
	})
}

func normalizeExactProfileRequest(req *assignExactTaskProfileRequest) {
	req.TaskID = strings.TrimSpace(req.TaskID)
	req.AgentProfileID = strings.TrimSpace(req.AgentProfileID)
	req.ExpectedModel = strings.TrimSpace(req.ExpectedModel)
	req.ExpectedTaskState = v1.TaskState(strings.ToUpper(strings.TrimSpace(string(req.ExpectedTaskState))))
	req.ExpectedWorkflowStepID = strings.TrimSpace(req.ExpectedWorkflowStepID)
	req.TargetWorkflowStepID = strings.TrimSpace(req.TargetWorkflowStepID)
	req.SenderTaskID = strings.TrimSpace(req.SenderTaskID)
	req.SenderSessionID = strings.TrimSpace(req.SenderSessionID)
}

func validateExactProfileRequest(req assignExactTaskProfileRequest) error {
	switch {
	case req.TaskID == "":
		return errors.New("task_id is required")
	case req.AgentProfileID == "":
		return errors.New("agent_profile_id is required")
	case req.ExpectedModel == "":
		return errors.New("expected_model is required")
	case req.ExpectedWorkflowStepID == "" || req.TargetWorkflowStepID == "":
		return errors.New("expected and target workflow step IDs are required")
	case !isValidTaskState(req.ExpectedTaskState):
		return errors.New("expected_task_state is invalid")
	case req.ExpectedAssignmentGeneration < 0 || req.ExpectedAssignmentGeneration == math.MinInt64:
		return errors.New("expected_assignment_generation must be zero or greater")
	case req.SenderTaskID == "" || req.SenderSessionID == "":
		return errors.New("trusted Coordinator provenance is required")
	default:
		return nil
	}
}

func (h *Handlers) exactProfileCoordinatorPrincipal(
	ctx context.Context,
	req assignExactTaskProfileRequest,
) (mcpscope.Principal, bool) {
	principal, ok := mcpscope.PrincipalFromContext(ctx)
	if !ok || principal.CallerTaskID != req.SenderTaskID ||
		principal.CallerSessionID != req.SenderSessionID {
		return mcpscope.Principal{}, false
	}
	return principal, principal.IsAutomation() || h.isCanonicalCoordinator(ctx, principal)
}

func (h *Handlers) isCanonicalCoordinator(ctx context.Context, principal mcpscope.Principal) bool {
	if principal.Surface != mcpprofile.SurfaceKanbanTask || h.taskSvc == nil {
		return false
	}
	task, err := h.taskSvc.GetTask(ctx, principal.CallerTaskID)
	if err != nil || task == nil || task.WorkspaceID != principal.WorkspaceID {
		return false
	}
	return mcpscope.IsCanonicalCoordinatorTask(ctx, task, h.taskSvc)
}

func (h *Handlers) validateExactProfileTarget(
	ctx context.Context,
	msg *ws.Message,
	principal mcpscope.Principal,
	req assignExactTaskProfileRequest,
) (*models.Task, *ws.Message) {
	task, err := h.taskSvc.GetTask(ctx, req.TaskID)
	if err != nil || task == nil || task.WorkspaceID != principal.WorkspaceID {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound, "target task not found", nil)
		return nil, response
	}
	if task.ArchivedAt != nil || task.WorkflowStepID != req.ExpectedWorkflowStepID || task.State != req.ExpectedTaskState {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeConflict,
			"exact task profile assignment rejected because the target lane or state changed", nil)
		return nil, response
	}
	stepResponse, err := h.workflowCtrl.GetStep(ctx, req.TargetWorkflowStepID)
	if err != nil || stepResponse == nil || stepResponse.Step == nil || stepResponse.Step.WorkflowID != task.WorkflowID {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeNotFound,
			"target workflow step not found on the task workflow", nil)
		return nil, response
	}
	name := strings.ToLower(stepResponse.Step.Name)
	name = strings.NewReplacer(" ", "", "_", "", "-", "").Replace(name)
	if name == "todeploy" {
		response, _ := ws.NewError(msg.ID, msg.Action, ws.ErrorCodeForbidden,
			"exact task profile assignment is unavailable for ToDeploy", nil)
		return nil, response
	}
	return task, nil
}

func jsonInt(value int64) string {
	payload, _ := json.Marshal(value)
	return string(payload)
}
