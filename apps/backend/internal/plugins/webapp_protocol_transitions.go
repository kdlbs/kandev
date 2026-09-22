package plugins

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/webapp"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type transitionDataSource interface {
	ListTaskStepTransitions(context.Context, string, int, string) ([]taskmodels.StepTransition, string, error)
	ListWorkflowTransitionGroups(context.Context, string, int, string) ([]taskmodels.TransitionGroup, string, error)
}

type webAppTransition struct {
	ID                 string  `json:"id"`
	FromWorkflowID     *string `json:"from_workflow_id"`
	FromWorkflowStepID *string `json:"from_workflow_step_id"`
	ToWorkflowID       *string `json:"to_workflow_id"`
	ToWorkflowStepID   *string `json:"to_workflow_step_id"`
	Trigger            string  `json:"trigger"`
	OccurredAt         string  `json:"occurred_at"`
}

type webAppTransitionGroup struct {
	Kind       string  `json:"kind"`
	FromStepID *string `json:"from_step_id"`
	ToStepID   *string `json:"to_step_id"`
	Count      int64   `json:"count"`
}

func (s *Service) listWebAppTaskTransitions(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, taskID string) {
	if !validWebAppKey(taskID) {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	page, err := webAppRequestPage(r)
	if err != nil {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	task, err := host.fetchTaskForScopeCheck(ctx, taskID)
	if err != nil || task == nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	if !webAppTaskMatches(ctx, host, binding, *task) {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	source, ok := s.taskData.(transitionDataSource)
	if !ok {
		writeWebAppError(w, http.StatusNotImplemented, "not_implemented")
		return
	}
	items, next, err := source.ListTaskStepTransitions(ctx, taskID, int(page.Limit), page.Cursor)
	if err != nil {
		writeTransitionReadError(w, err)
		return
	}
	result := make([]webAppTransition, len(items))
	for i, item := range items {
		result[i] = webAppTransition{
			ID: strconv.FormatInt(item.ID, 10), FromWorkflowID: item.FromWorkflowID,
			FromWorkflowStepID: item.FromWorkflowStepID, ToWorkflowID: item.ToWorkflowID,
			ToWorkflowStepID: item.ToWorkflowStepID, Trigger: item.Trigger,
			OccurredAt: item.OccurredAt.UTC().Format(time.RFC3339Nano),
		}
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppPage[webAppTransition]{Items: result, PageInfo: webAppPageInfo{NextCursor: next, HasMore: next != ""}})
}

func (s *Service) listWebAppWorkflowTransitionGroups(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, workflowID string) {
	if !validWebAppKey(workflowID) {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	if binding.ScopeKind != instances.ScopeWorkspace || binding.WorkspaceID == "" || !host.capabilities.CanRead(resourceTasks) || !host.capabilities.CanRead(resourceWorkflows) {
		writeWebAppError(w, http.StatusForbidden, "plugin_permission_denied")
		return
	}
	page, err := webAppRequestPage(r)
	if err != nil {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	if host.workflows == nil {
		writeWebAppError(w, http.StatusNotImplemented, "not_implemented")
		return
	}
	allowed, err := webAppWorkflowAllowed(ctx, host, binding.WorkspaceID, workflowID)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	if !allowed {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	source, ok := s.taskData.(transitionDataSource)
	if !ok {
		writeWebAppError(w, http.StatusNotImplemented, "not_implemented")
		return
	}
	items, next, err := source.ListWorkflowTransitionGroups(ctx, workflowID, int(page.Limit), page.Cursor)
	if err != nil {
		writeTransitionReadError(w, err)
		return
	}
	result := make([]webAppTransitionGroup, len(items))
	for i, item := range items {
		result[i] = webAppTransitionGroup{Kind: item.Kind, FromStepID: item.FromStepID, ToStepID: item.ToStepID, Count: item.Count}
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppPage[webAppTransitionGroup]{Items: result, PageInfo: webAppPageInfo{NextCursor: next, HasMore: next != ""}})
}

func webAppWorkflowAllowed(ctx context.Context, host *pluginHost, workspaceID, workflowID string) (bool, error) {
	workflows, err := host.workflows.ListWorkflows(ctx, workspaceID, false)
	if err != nil {
		return false, err
	}
	for _, workflow := range workflows {
		if workflow != nil && workflow.ID == workflowID && workflow.WorkspaceID == workspaceID {
			return true, nil
		}
	}
	return false, nil
}

func writeTransitionReadError(w http.ResponseWriter, err error) {
	if errors.Is(err, taskservice.ErrInvalidTransitionCursor) {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
}
