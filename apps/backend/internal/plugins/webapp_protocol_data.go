package plugins

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/kandev/kandev/internal/plugins/instances"
	"github.com/kandev/kandev/internal/plugins/state"
	"github.com/kandev/kandev/internal/plugins/webapp"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/pkg/pluginsdk"
)

func (s *Service) listWebAppTasks(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding) {
	page, err := webAppRequestPage(r)
	if err != nil {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	includeArchived, err := webAppBoolQuery(r, "include_archived")
	if err != nil {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	workspaceIDs, err := s.webAppTaskWorkspaceIDs(r, binding)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	filter := pluginsdk.TaskFilter{
		WorkspaceIDs:     workspaceIDs,
		WorkflowIDs:      webAppQueryList(r.URL.Query()["workflow_id"]),
		States:           webAppQueryList(r.URL.Query()["state"]),
		ParentID:         webAppParentQuery(r),
		IncludeArchived:  includeArchived,
		IncludeEphemeral: true,
	}
	if instances.EffectiveDataScopeKind(binding.ScopeKind, binding.DataScopeKind) == instances.ScopeTask {
		// Resolved via the non-attaching fetchTask, and dependencies attached
		// only below, after the discard checks: matching the attachment rule
		// the non-task-scope branch below already documents for itself.
		dto, model, err := host.fetchTask(ctx, binding.TaskID)
		if err != nil || dto == nil {
			writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
			return
		}
		if !webAppTaskMatches(ctx, host, binding, *dto) || !webAppTaskMatchesFilter(*dto, filter) || (!includeArchived && dto.ArchivedAt != nil) {
			writeWebAppJSON(w, r, http.StatusOK, webAppPage[webAppTask]{Items: []webAppTask{}, PageInfo: webAppPageInfo{}})
			return
		}
		fetched := []pluginsdk.Task{*dto}
		host.attachPullRequests(ctx, fetched)
		var models []*taskmodels.Task
		if model != nil {
			models = []*taskmodels.Task{model}
		}
		if err := host.attachDependencies(ctx, fetched, models, false, "ListWebAppTasks"); err != nil {
			writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
			return
		}
		scope := dependencyRedactionScopeFromBinding(binding, nil)
		items, info := paginate([]webAppTask{webAppTaskFromSDK(fetched[0], scope)}, page)
		writeWebAppJSON(w, r, http.StatusOK, webAppPageFromSDK(items, info))
		return
	}

	// Fetches without deriving dependencies: the binding scope (repository or
	// session) narrows the fetched page further below, and deriving before
	// that narrowing would both waste work on tasks the caller never sees and
	// risk tripping the fan-out cap on a page the actual response never
	// approaches.
	if !host.capabilities.CanRead(resourceTasks) {
		writeWebAppError(w, http.StatusForbidden, "plugin_permission_denied")
		return
	}
	if host.taskData == nil {
		_, _, err := host.UnimplementedHostData.Tasks().List(ctx, filter, page)
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	models, info, err := host.fetchTaskPage(ctx, filter, page)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	dtos := tasksToDTOs(models)
	host.attachPullRequests(ctx, dtos)
	survivingModels := make([]*taskmodels.Task, 0, len(dtos))
	survivingDTOs := make([]pluginsdk.Task, 0, len(dtos))
	for i, dto := range dtos {
		if webAppTaskMatches(ctx, host, binding, dto) {
			survivingDTOs = append(survivingDTOs, dto)
			survivingModels = append(survivingModels, models[i])
		}
	}
	if err := host.attachDependencies(ctx, survivingDTOs, survivingModels, true, "ListWebAppTasks"); err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	directlyReadableIDs := make([]string, len(survivingDTOs))
	for i, dto := range survivingDTOs {
		directlyReadableIDs[i] = dto.ID
	}
	scope := dependencyRedactionScopeFromBinding(binding, directlyReadableIDs)
	filtered := make([]webAppTask, len(survivingDTOs))
	for i, dto := range survivingDTOs {
		filtered[i] = webAppTaskFromSDK(dto, scope)
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppPageFromSDK(filtered, info))
}

func (s *Service) getWebAppTask(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, taskID string) {
	if !validWebAppKey(taskID) {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	// Resolved via the non-attaching fetchTask, and dependencies attached only
	// below, after the scope check that can discard the result and 404.
	dto, model, err := host.fetchTask(ctx, taskID)
	if err != nil || dto == nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	if !webAppTaskMatches(ctx, host, binding, *dto) {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	fetched := []pluginsdk.Task{*dto}
	host.attachPullRequests(ctx, fetched)
	var models []*taskmodels.Task
	if model != nil {
		models = []*taskmodels.Task{model}
	}
	if err := host.attachDependencies(ctx, fetched, models, false, "GetWebAppTask"); err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	scope := dependencyRedactionScopeFromBinding(binding, []string{fetched[0].ID})
	writeWebAppJSON(w, r, http.StatusOK, webAppTaskFromSDK(fetched[0], scope))
}

type webAppTaskPatch struct {
	Title          *string `json:"title"`
	Description    *string `json:"description"`
	State          *string `json:"state"`
	WorkflowStepID *string `json:"workflow_step_id"`
}

func (p webAppTaskPatch) hasChanges() bool {
	return p.Title != nil || p.Description != nil || p.State != nil || p.WorkflowStepID != nil
}

func (p webAppTaskPatch) hasTaskFields() bool {
	return p.Title != nil || p.Description != nil || p.State != nil
}

func (s *Service) updateWebAppTask(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, taskID string) {
	if !validWebAppKey(taskID) {
		writeWebAppError(w, http.StatusNotFound, "not_found")
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
	var patch webAppTaskPatch
	if err := decodeWebAppJSON(r, &patch); err != nil || !patch.hasChanges() {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	var updated *pluginsdk.Task
	if patch.hasTaskFields() {
		updateInput := pluginsdk.UpdateTaskInput{
			ID: taskID, Title: patch.Title, Description: patch.Description, State: patch.State,
		}
		if patch.WorkflowStepID != nil {
			// A body naming both a task field and workflow_step_id takes both
			// this branch and the Move branch below, but only Move's result is
			// serialized -- so this write must not attach dependencies on a
			// result nobody sees. The one derivation the response makes runs
			// on Move's result, below.
			if _, err = host.writeTaskUpdate(ctx, updateInput); err != nil {
				writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
				return
			}
		} else {
			updated, err = host.Tasks().Update(ctx, updateInput)
			if err != nil {
				writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
				return
			}
		}
	}
	if patch.WorkflowStepID != nil {
		// Keep the browser PATCH contract, but route transitions through Move so
		// workflow hooks, active-session guards, and step history stay intact.
		outcome, moveErr := host.Tasks().Move(ctx, pluginsdk.MoveTaskInput{
			TaskID:         taskID,
			WorkflowStepID: *patch.WorkflowStepID,
		})
		if moveErr != nil {
			writeWebAppError(w, webAppProtocolStatus(moveErr), webAppErrorCode(moveErr))
			return
		}
		if outcome == nil || outcome.Task == nil {
			writeWebAppError(w, http.StatusInternalServerError, webAppRuntimeUnavailable)
			return
		}
		updated = outcome.Task
	}
	if updated == nil {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	scope := dependencyRedactionScopeFromBinding(binding, []string{updated.ID})
	writeWebAppJSON(w, r, http.StatusOK, webAppTaskFromSDK(*updated, scope))
}

type webAppMessageRequest struct {
	Text      string `json:"text"`
	SessionID string `json:"session_id,omitempty"`
}

func (s *Service) sendWebAppMessage(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, taskID string) {
	if !validWebAppKey(taskID) {
		writeWebAppError(w, http.StatusNotFound, "not_found")
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
	var request webAppMessageRequest
	if err := decodeWebAppJSON(r, &request); err != nil || strings.TrimSpace(request.Text) == "" {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	dispatch, err := host.Messages().Send(ctx, taskID, request.SessionID, request.Text)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	writeWebAppJSON(w, r, http.StatusAccepted, dispatch)
}

func (s *Service) listWebAppWorkflows(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding) {
	page, err := webAppRequestPage(r)
	if err != nil {
		writeWebAppError(w, http.StatusBadRequest, "invalid_request")
		return
	}
	workspaceIDs, err := s.webAppWorkflowWorkspaceIDs(ctx, r, host, binding)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	var workflows []pluginsdk.Workflow
	for _, workspaceID := range workspaceIDs {
		items, _, err := host.Workflows().List(ctx, workspaceID, pluginsdk.Page{Limit: maxPageLimit})
		if err != nil {
			writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
			return
		}
		workflows = append(workflows, items...)
	}
	if instances.EffectiveDataScopeKind(binding.ScopeKind, binding.DataScopeKind) == instances.ScopeTask {
		task, err := host.fetchTaskForScopeCheck(ctx, binding.TaskID)
		if err != nil || task == nil {
			writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
			return
		}
		workflows = filterWebAppWorkflows(workflows, task.WorkflowID)
	}
	items, info := paginate(workflows, page)
	result := make([]webAppWorkflow, len(items))
	for i, workflow := range items {
		result[i] = webAppWorkflowFromSDK(workflow)
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppPageFromSDK(result, info))
}

func (s *Service) listWebAppWorkflowSteps(ctx context.Context, w http.ResponseWriter, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding, workflowID string) {
	if !validWebAppKey(workflowID) {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	workspaceIDs, err := s.webAppWorkflowWorkspaceIDs(ctx, r, host, binding)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	allowed := false
	for _, workspaceID := range workspaceIDs {
		workflows, _, err := host.Workflows().List(ctx, workspaceID, pluginsdk.Page{Limit: maxPageLimit})
		if err != nil {
			writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
			return
		}
		for _, workflow := range workflows {
			if workflow.ID == workflowID {
				allowed = true
				break
			}
		}
	}
	if instances.EffectiveDataScopeKind(binding.ScopeKind, binding.DataScopeKind) == instances.ScopeTask {
		task, err := host.fetchTaskForScopeCheck(ctx, binding.TaskID)
		if err != nil || task == nil || task.WorkflowID != workflowID {
			writeWebAppError(w, http.StatusNotFound, "not_found")
			return
		}
	}
	if !allowed {
		writeWebAppError(w, http.StatusNotFound, "not_found")
		return
	}
	steps, err := host.Workflows().ListSteps(ctx, workflowID)
	if err != nil {
		writeWebAppError(w, webAppProtocolStatus(err), webAppErrorCode(err))
		return
	}
	result := make([]webAppWorkflowStep, len(steps))
	for i, step := range steps {
		result[i] = webAppWorkflowStepFromSDK(step)
	}
	writeWebAppJSON(w, r, http.StatusOK, webAppPage[webAppWorkflowStep]{Items: result, PageInfo: webAppPageInfo{}})
}

func (s *Service) webAppTaskWorkspaceIDs(r *http.Request, binding webapp.CapabilityBinding) ([]string, error) {
	requested := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if binding.WorkspaceID != "" {
		if requested != "" && requested != binding.WorkspaceID {
			return nil, status.Error(codes.PermissionDenied, "workspace is outside the canvas scope")
		}
		return []string{binding.WorkspaceID}, nil
	}
	if requested != "" {
		return []string{requested}, nil
	}
	// Task reads can use the Host task reader's global workspace resolution.
	// This avoids requiring a separate workspaces grant for an instance canvas.
	return nil, nil
}

func (s *Service) webAppWorkflowWorkspaceIDs(ctx context.Context, r *http.Request, host *pluginHost, binding webapp.CapabilityBinding) ([]string, error) {
	requested := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if binding.WorkspaceID != "" {
		if requested != "" && requested != binding.WorkspaceID {
			return nil, status.Error(codes.PermissionDenied, "workspace is outside the canvas scope")
		}
		return []string{binding.WorkspaceID}, nil
	}
	if requested != "" {
		return []string{requested}, nil
	}
	workspaces, _, err := host.Workspaces().List(ctx, pluginsdk.Page{Limit: maxPageLimit})
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(workspaces))
	for i, workspace := range workspaces {
		ids[i] = workspace.ID
	}
	return ids, nil
}

func webAppTaskMatches(ctx context.Context, host *pluginHost, binding webapp.CapabilityBinding, task pluginsdk.Task) bool {
	dataScope := instances.EffectiveDataScopeKind(binding.ScopeKind, binding.DataScopeKind)
	if dataScope == instances.ScopeInstance {
		return true
	}
	if task.WorkspaceID != binding.WorkspaceID {
		return false
	}
	switch dataScope {
	case instances.ScopeWorkspace:
		return true
	case instances.ScopeTask:
		return task.ID == binding.TaskID
	case instances.ScopeRepository:
		return webAppTaskHasRepository(task, binding.RepositoryID)
	case instances.ScopeSession:
		return webAppTaskHasSession(ctx, host, task, binding.SessionID)
	default:
		return false
	}
}

func webAppTaskHasRepository(task pluginsdk.Task, repositoryID string) bool {
	for _, repository := range task.Repositories {
		if repository.RepositoryID == repositoryID || repository.ID == repositoryID {
			return true
		}
	}
	return false
}

func webAppTaskHasSession(ctx context.Context, host *pluginHost, task pluginsdk.Task, sessionID string) bool {
	if host.taskData == nil {
		return false
	}
	sessions, err := host.taskData.ListTaskSessions(ctx, task.ID)
	if err != nil {
		return false
	}
	for _, session := range sessions {
		if session != nil && session.ID == sessionID {
			return true
		}
	}
	return false
}

func webAppTaskMatchesFilter(task pluginsdk.Task, filter pluginsdk.TaskFilter) bool {
	if len(filter.WorkflowIDs) > 0 && !stringInSlice(filter.WorkflowIDs, task.WorkflowID) {
		return false
	}
	if len(filter.States) > 0 && !stringInSlice(filter.States, task.State) {
		return false
	}
	return filter.ParentID == nil || (task.ParentID != nil && *task.ParentID == *filter.ParentID)
}

func stringInSlice(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func filterWebAppWorkflows(workflows []pluginsdk.Workflow, workflowID string) []pluginsdk.Workflow {
	filtered := make([]pluginsdk.Workflow, 0, 1)
	for _, workflow := range workflows {
		if workflow.ID == workflowID {
			filtered = append(filtered, workflow)
		}
	}
	return filtered
}

func webAppErrorCode(err error) string {
	if err == nil {
		return webAppRuntimeUnavailable
	}
	if errors.Is(err, webapp.ErrRuntimeTokenStale) {
		return "runtime_token_stale"
	}
	if errors.Is(err, state.ErrConflict) {
		return "plugin_state_conflict"
	}
	if errors.Is(err, instances.ErrNotFound) || status.Code(err) == codes.NotFound {
		return "not_found"
	}
	switch status.Code(err) {
	case codes.PermissionDenied:
		return "plugin_permission_denied"
	case codes.InvalidArgument:
		return "invalid_request"
	case codes.FailedPrecondition:
		return "plugin_state_conflict"
	case codes.Unimplemented:
		return webAppRuntimeUnavailable
	case codes.ResourceExhausted:
		return "response_too_large"
	default:
		return webAppRuntimeUnavailable
	}
}
