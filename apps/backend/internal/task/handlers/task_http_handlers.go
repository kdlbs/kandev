package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/orchestrator"
	storageworkspaces "github.com/kandev/kandev/internal/system/storage/workspaces"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	taskrepository "github.com/kandev/kandev/internal/task/repository"
	taskrepo "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/kandev/kandev/internal/task/statussummary"
	usermodels "github.com/kandev/kandev/internal/user/models"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/worktree"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

type httpWorkspaceSourcesRequest struct {
	Sources []json.RawMessage `json:"sources"`
}

type workspaceSourceJSON struct {
	Kind           string `json:"kind"`
	RepositoryID   string `json:"repository_id"`
	LocalPath      string `json:"local_path"`
	GitHubURL      string `json:"github_url"`
	RemoteURL      string `json:"remote_url"`
	Provider       string `json:"provider"`
	ProviderRepoID string `json:"provider_repo_id"`
	ProviderOwner  string `json:"provider_owner"`
	ProviderName   string `json:"provider_name"`
	BaseBranch     string `json:"base_branch"`
	CheckoutBranch string `json:"checkout_branch"`
	DisplayName    string `json:"display_name"`
}

func (h *TaskHandlers) httpAttachWorkspaceSources(c *gin.Context) {
	var body httpWorkspaceSourcesRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || len(body.Sources) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sources is required"})
		return
	}
	sources, err := parseHTTPWorkspaceSources(body.Sources)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	result, err := h.service.AttachWorkspaceSources(c.Request.Context(), service.AttachWorkspaceSourcesRequest{TaskID: c.Param("id"), Sources: sources})
	if err != nil {
		h.writeWorkspaceSourceError(c, err)
		return
	}
	response := gin.H{"task_id": result.Task.ID, "repositories": result.Task.Repositories, "workspace_folders": result.Task.WorkspaceFolders, "workspace_path": result.WorkspacePath, "session_ids": result.SessionIDs}
	c.JSON(http.StatusOK, response)
}

func parseHTTPWorkspaceSources(raw []json.RawMessage) ([]service.WorkspaceSourceInput, error) {
	sources := make([]service.WorkspaceSourceInput, 0, len(raw))
	for _, item := range raw {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil {
			return nil, fmt.Errorf("source must be an object")
		}
		var kind string
		if err := json.Unmarshal(fields["kind"], &kind); err != nil {
			return nil, fmt.Errorf("source kind is required")
		}
		allowed := map[string]bool{"kind": true, "local_path": true}
		switch kind {
		case string(service.WorkspaceSourceRepository):
			for _, key := range []string{"repository_id", "remote_url", "github_url", "provider", "provider_repo_id", "provider_owner", "provider_name", "base_branch", "checkout_branch"} {
				allowed[key] = true
			}
		case string(service.WorkspaceSourceFolder):
			allowed["display_name"] = true
		default:
			return nil, fmt.Errorf("unsupported workspace source kind %q", kind)
		}
		for key := range fields {
			if !allowed[key] {
				return nil, fmt.Errorf("field %q is not allowed for %s source", key, kind)
			}
		}
		var source workspaceSourceJSON
		if err := json.Unmarshal(item, &source); err != nil {
			return nil, err
		}
		sources = append(sources, service.WorkspaceSourceInput{Kind: service.WorkspaceSourceKind(source.Kind), RepositoryID: source.RepositoryID, LocalPath: source.LocalPath, GitHubURL: source.GitHubURL, RemoteURL: source.RemoteURL, Provider: source.Provider, ProviderRepoID: source.ProviderRepoID, ProviderOwner: source.ProviderOwner, ProviderName: source.ProviderName, BaseBranch: source.BaseBranch, CheckoutBranch: source.CheckoutBranch, DisplayName: source.DisplayName})
	}
	return sources, nil
}

func workspaceSourceHTTPStatus(err error) int {
	switch {
	case errors.Is(err, service.ErrInvalidWorkspaceSource):
		return http.StatusBadRequest
	case errors.Is(err, taskrepo.ErrTaskNotFound), errors.Is(err, taskrepository.ErrRepositoryNotFound), errors.Is(err, service.ErrTaskRepositoryNotFound):
		return http.StatusNotFound
	case errors.Is(err, service.ErrWorkspaceSourceConflict), errors.Is(err, service.ErrWorkspaceSourceActive):
		return http.StatusConflict
	case errors.Is(err, service.ErrUnsupportedWorkspaceSource), errors.Is(err, service.ErrWorkspaceSourceMaterialize):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

func (h *TaskHandlers) writeWorkspaceSourceError(c *gin.Context, err error) {
	status := workspaceSourceHTTPStatus(err)
	if status == http.StatusInternalServerError {
		h.logger.Error("attach workspace sources failed", zap.Error(err))
		c.JSON(status, gin.H{"error": "request failed"})
		return
	}
	c.JSON(status, gin.H{"error": err.Error()})
}

func (h *TaskHandlers) httpListTasks(c *gin.Context) {
	tasks, err := h.service.ListTasks(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}
	taskDTOs, err := h.toTaskDTOsWithSessionInfo(c.Request.Context(), tasks)
	if err != nil {
		h.logger.Error("failed to enrich tasks with status summaries", zap.Error(err))
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}
	c.JSON(http.StatusOK, dto.ListTasksResponse{
		Tasks: taskDTOs,
		Total: len(tasks),
	})
}

func (h *TaskHandlers) httpListTasksByWorkspace(c *gin.Context) {
	page := 1
	pageSize := 50

	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	query := c.Query("query")
	sort := usermodels.NormalizeTasksListSort(c.Query("sort"))
	workflowID := c.Query("workflow_id")
	repositoryID := c.Query("repository_id")
	includeArchived := c.Query("include_archived") == queryValueTrue
	onlyArchived := c.Query("only_archived") == queryValueTrue
	includeEphemeral := c.Query("include_ephemeral") == queryValueTrue
	onlyEphemeral := c.Query("only_ephemeral") == queryValueTrue
	excludeConfig := c.Query("exclude_config") == queryValueTrue

	tasks, total, err := h.service.ListTasksByWorkspaceWithArchiveMode(
		c.Request.Context(), c.Param("id"), workflowID, repositoryID, query, page, pageSize, sort, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig, onlyArchived,
	)
	if err != nil {
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}

	taskDTOs, err := h.toTaskDTOsWithSessionInfo(c.Request.Context(), tasks)
	if err != nil {
		h.logger.Error("failed to enrich tasks with session info", zap.Error(err))
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}

	c.JSON(http.StatusOK, dto.ListTasksResponse{
		Tasks: taskDTOs,
		Total: total,
	})
}

// buildTaskDTOsWithSessionInfo converts tasks to DTOs enriched with primary
// session IDs, session counts, review status, and the bounded task status
// summary. Session and executor lookups remain batched; missing summary rows
// are repaired lazily from the same batch-loaded authoritative inputs.
func buildTaskDTOsWithSessionInfo(
	ctx context.Context,
	svc *service.Service,
	log *logger.Logger,
	activityProvider dto.ForegroundActivityProvider,
	tasks []*models.Task,
) ([]dto.TaskDTO, error) {
	if len(tasks) == 0 {
		return []dto.TaskDTO{}, nil
	}
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	sessionsByTask, err := svc.BatchGetSessionsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	primarySessionInfoMap, err := svc.GetPrimarySessionInfoForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	pendingActionsBySession, pendingErr := pendingActionsForInputCapableSessions(ctx, svc, sessionsByTask)
	if pendingErr != nil {
		log.Warn("failed to load pending actions for task list, using empty map", zap.Error(pendingErr))
		pendingActionsBySession = map[string]models.TaskPendingAction{}
	}
	statusSummaries, summaryErr := svc.GetTaskStatusSummaries(ctx, taskIDs)
	if summaryErr != nil {
		log.Warn("failed to load task status summaries, using coarse task fields", zap.Error(summaryErr))
		statusSummaries = map[string]*statussummary.TaskStatusSummary{}
	}
	if summaryErr == nil && pendingErr == nil {
		reconciledSummaries, reconcileErr := svc.ReconcileTaskStatusSummaries(
			ctx, tasks, sessionsByTask, pendingActionsBySession, statusSummaries,
		)
		statusSummaries = reconciledSummaries
		if reconcileErr != nil {
			log.Warn("failed to reconcile task status summaries", zap.Error(reconcileErr))
		}
	}
	// Stamp the authoritative per-task queued prompt count onto every summary.
	// The projector keeps the summary field fresh between list loads; this
	// fresh batch read is the initial-load backstop (e.g. after a restart the
	// projector may not have observed every queue mutation yet). Never
	// fabricate a summary here — a synthetic summary would make the frontend
	// treat summary fields as authoritative and hide the coarse fallbacks.
	queuedByTask, queuedErr := svc.CountPendingQueuedByTaskIDs(ctx, taskIDs)
	if queuedErr != nil {
		log.Warn("failed to load queued prompt counts for task list, omitting badges", zap.Error(queuedErr))
	}
	// Dependency state is derived, never stored, so it is computed per read. One
	// batched call for the whole list: a per-task query would add a round trip
	// per card to every board load.
	dependencyViews := svc.BuildDependencyViews(ctx, tasks)
	result := make([]dto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		sessions := sessionsByTask[task.ID]
		var primarySessionID *string
		for _, s := range sessions {
			if s.IsPrimary {
				id := s.ID
				primarySessionID = &id
				break
			}
		}
		var sessionCount *int
		if n := len(sessions); n > 0 {
			sessionCount = &n
		}
		si := extractSessionInfo(primarySessionInfoMap[task.ID])
		taskDTO := dto.FromTaskWithSessionInfo(
			task,
			primarySessionID,
			sessionCount,
			si.reviewStatus,
			si.executorID,
			si.executorType,
			si.executorName,
			si.agentName,
			si.workingDirectory,
			si.sessionState,
			dto.PendingActionPtr(si.sessionID, pendingActionsBySession),
		)
		taskDTO.TaskPendingAction = dto.TaskPendingActionPtr(sessions, pendingActionsBySession)
		dto.EnrichTaskForegroundActivity(&taskDTO, sessions, activityProvider)
		dto.EnrichTaskDependencies(&taskDTO, dependencyProjection(dependencyViews[task.ID]), task)
		dto.EnrichTaskStatusSummary(&taskDTO, task.ID, statusSummaries)
		if taskDTO.StatusSummary != nil {
			switch {
			case queuedErr != nil:
				// Counter failed: honor the documented no-badge fallback in the
				// response without persisting the cleared value.
				taskDTO.StatusSummary.QueuedPromptCount = 0
			case queuedByTask != nil:
				// Fresh per-task count from the queue store; the projector keeps
				// this field live between list loads, this batch read is the
				// authoritative initial-load backstop. See the comment above the
				// batch load.
				taskDTO.StatusSummary.QueuedPromptCount = queuedByTask[task.ID]
			}
		}
		result = append(result, taskDTO)
	}
	return result, nil
}

type sessionInfoFields struct {
	sessionID        *string
	reviewStatus     models.ReviewStatus
	sessionState     *string
	executorID       *string
	executorType     *string
	executorName     *string
	agentName        *string
	workingDirectory *string
}

func extractSessionInfo(info *models.TaskSession) sessionInfoFields {
	var si sessionInfoFields
	if info == nil {
		return si
	}
	if info.ID != "" {
		val := info.ID
		si.sessionID = &val
	}
	si.reviewStatus = info.ReviewStatus
	if info.State != "" {
		val := string(info.State)
		si.sessionState = &val
	}
	if info.ExecutorID != "" {
		val := info.ExecutorID
		si.executorID = &val
	}
	if info.ExecutorSnapshot != nil {
		if t, ok := info.ExecutorSnapshot["executor_type"].(string); ok && t != "" {
			si.executorType = &t
		}
		if n, ok := info.ExecutorSnapshot["executor_name"].(string); ok && n != "" {
			si.executorName = &n
		}
	}
	if info.AgentProfileSnapshot != nil {
		if name, ok := info.AgentProfileSnapshot["name"].(string); ok && name != "" {
			si.agentName = &name
		}
	}
	if info.RepositorySnapshot != nil {
		if path, ok := info.RepositorySnapshot["path"].(string); ok && path != "" {
			si.workingDirectory = &path
		}
	}
	return si
}

func pendingActionsForInputCapableSessions(
	ctx context.Context,
	svc *service.Service,
	sessionsByTask map[string][]*models.TaskSession,
) (map[string]models.TaskPendingAction, error) {
	sessionIDs := make([]string, 0)
	for _, sessions := range sessionsByTask {
		for _, session := range sessions {
			if dto.IsInputCapableSession(session) {
				sessionIDs = append(sessionIDs, session.ID)
			}
		}
	}
	if len(sessionIDs) == 0 {
		return map[string]models.TaskPendingAction{}, nil
	}
	return svc.GetPendingActionsForSessions(ctx, sessionIDs)
}

func isInputCapableSession(session *models.TaskSession) bool {
	return session != nil && (session.State == models.TaskSessionStateRunning || session.State == models.TaskSessionStateWaitingForInput)
}

func pendingActionPtr(
	sessionID *string,
	pendingActionsBySession map[string]models.TaskPendingAction,
) *string {
	if sessionID == nil {
		return nil
	}
	action, ok := pendingActionsBySession[*sessionID]
	if !ok {
		return nil
	}
	value := string(action)
	return &value
}

func pendingActionRevisionPtr(
	sessionID string,
	revisionsBySession map[string]models.PendingActionRevision,
) *models.PendingActionRevision {
	revision, ok := revisionsBySession[sessionID]
	if !ok {
		return nil
	}
	return &revision
}

func (h *TaskHandlers) taskSessionDTO(ctx context.Context, session *models.TaskSession) dto.TaskSessionDTO {
	result := dto.FromTaskSession(session)
	dto.EnrichCancellationPending(&result, h.cancellationPending)
	actions, revisions, err := h.service.GetPendingActionProjectionsForSessions(
		ctx,
		[]string{session.ID},
	)
	if err != nil {
		h.logger.Warn("get task session pending action failed",
			zap.String("session_id", session.ID), zap.Error(err))
		return result
	}
	if isInputCapableSession(session) {
		result.PendingAction = pendingActionPtr(&session.ID, actions)
	}
	result.PendingActionRevision = pendingActionRevisionPtr(session.ID, revisions)
	return result
}

func (h *TaskHandlers) toTaskDTOsWithSessionInfo(ctx context.Context, tasks []*models.Task) ([]dto.TaskDTO, error) {
	return buildTaskDTOsWithSessionInfo(ctx, h.service, h.logger, h.foregroundActivity, tasks)
}

func (h *TaskHandlers) httpGetTask(c *gin.Context) {
	task, err := h.service.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	dtos, err := buildTaskDTOsWithSessionInfo(c.Request.Context(), h.service, h.logger, h.foregroundActivity, []*models.Task{task})
	if err != nil {
		h.logger.Error("failed to build task DTO with session info", zap.Error(err))
		c.JSON(http.StatusOK, dto.FromTask(task))
		return
	}
	c.JSON(http.StatusOK, dtos[0])
}

func (h *TaskHandlers) httpListTaskSessions(c *gin.Context) {
	ctx := c.Request.Context()
	sessions, err := h.service.ListTaskSessions(ctx, c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task sessions not found")
		return
	}
	sessionDTOs, projectionErr := h.taskSessionSummariesWithPendingActions(ctx, sessions)
	if projectionErr != nil {
		h.logger.Error("get task session pending actions failed", zap.Error(projectionErr))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load task session pending actions"})
		return
	}
	ids := make([]string, 0, len(sessionDTOs))
	for _, summary := range sessionDTOs {
		ids = append(ids, summary.ID)
	}
	// Resolve the per-session tool_call counts so the frontend can render
	// the "ran N commands" segment without fetching every session's full
	// message list. Best-effort: a count failure leaves CommandCount at 0.
	if counts, cErr := h.repo.CountToolCallMessagesBySession(ctx, ids); cErr == nil {
		for i := range sessionDTOs {
			sessionDTOs[i].CommandCount = counts[sessionDTOs[i].ID]
		}
	} else {
		h.logger.Warn("count tool calls failed", zap.Error(cErr))
	}
	c.JSON(http.StatusOK, dto.ListTaskSessionSummariesResponse{
		Sessions: sessionDTOs,
		Total:    len(sessionDTOs),
	})
}

// httpEnsureTaskSession returns the task's existing primary/newest session if any,
// otherwise resolves the agent profile server-side and creates one (prepare or
// start, depending on the workflow step). Idempotent under concurrent calls.
func (h *TaskHandlers) httpEnsureTaskSession(c *gin.Context) {
	taskID := c.Param("id")
	if taskID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task id is required"})
		return
	}
	resp, err := h.orchestrator.EnsureSession(c.Request.Context(), taskID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpGetTaskSession(c *gin.Context) {
	session, err := h.service.GetTaskSession(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	sessionDTO := h.taskSessionDTO(c.Request.Context(), session)
	dto.EnrichForegroundActivity(&sessionDTO, h.foregroundActivity)
	c.JSON(http.StatusOK, dto.GetTaskSessionResponse{
		Session: sessionDTO,
	})
}

type dismissLastAgentErrorRequest struct {
	Stamp string `json:"stamp"`
}

func (h *TaskHandlers) httpDismissLastAgentError(c *gin.Context) {
	var req dismissLastAgentErrorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	session, err := h.service.DismissLastAgentError(c.Request.Context(), c.Param("id"), req.Stamp)
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	sessionDTO := h.taskSessionDTO(c.Request.Context(), session)
	dto.EnrichForegroundActivity(&sessionDTO, h.foregroundActivity)
	c.JSON(http.StatusOK, dto.GetTaskSessionResponse{
		Session: sessionDTO,
	})
}

type markSessionReadRequest struct {
	MessageID string `json:"message_id"`
}

// httpMarkSessionRead advances the session's Slack-style unread-divider read
// cursor to req.MessageID. A missing session is a 404; bad caller input
// (empty ids, an unknown message, or a message belonging to a different
// session — see service.ErrInvalidMarkSessionRead) is a 400; any other,
// unexpected failure (a repository/DB error) is logged and returned as a
// sanitized 500 rather than leaking the raw internal error to the caller.
func (h *TaskHandlers) httpMarkSessionRead(c *gin.Context) {
	var req markSessionReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: " + err.Error()})
		return
	}
	sessionID := c.Param("id")
	session, err := h.service.MarkSessionRead(c.Request.Context(), sessionID, req.MessageID)
	if err != nil {
		if errors.Is(err, models.ErrTaskSessionNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "task session not found"})
			return
		}
		if errors.Is(err, service.ErrInvalidMarkSessionRead) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		h.logger.Error("failed to mark session read",
			zap.String("session_id", sessionID),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to mark session read"})
		return
	}
	c.JSON(http.StatusOK, dto.MarkSessionReadResponse{
		SessionID:         session.ID,
		LastReadMessageID: session.LastReadMessageID,
	})
}

func (h *TaskHandlers) httpListSessionTurns(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
		return
	}
	// Per-user scoping: the caller must own the session's workspace.
	if err := h.service.AuthorizeSessionAccess(c.Request.Context(), sessionID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	turns, err := h.repo.ListTurnsBySession(c.Request.Context(), sessionID)
	if err != nil {
		h.logger.Error("failed to list turns", zap.String("session_id", sessionID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list turns"})
		return
	}

	// Convert to DTO
	turnDTOs := make([]dto.TurnDTO, 0, len(turns))
	for _, turn := range turns {
		turnDTOs = append(turnDTOs, dto.FromTurn(turn))
	}

	c.JSON(http.StatusOK, dto.ListTurnsResponse{Turns: turnDTOs, Total: len(turnDTOs)})
}

func (h *TaskHandlers) httpApproveSession(c *gin.Context) {
	result, err := h.service.ApproveSession(c.Request.Context(), c.Param("id"))
	if err != nil {
		// Route not-found through the shared mapper so a per-user scoping
		// denial answers 404 like every other session route, rather than
		// surfacing as a 500 that distinguishes "yours" from "someone else's".
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}

	resp := dto.ApproveSessionResponse{
		Success: true,
		Session: h.taskSessionDTO(c.Request.Context(), result.Session),
	}
	if result.WorkflowStep != nil {
		resp.WorkflowStep = dto.FromWorkflowStep(result.WorkflowStep)
	}
	c.JSON(http.StatusOK, resp)
}

func (h *TaskHandlers) httpGetWorkflowTaskCount(c *gin.Context) {
	count, err := h.service.CountTasksByWorkflow(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to count tasks by workflow", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count tasks"})
		return
	}
	c.JSON(http.StatusOK, dto.TaskCountResponse{TaskCount: count})
}

func (h *TaskHandlers) httpGetStepTaskCount(c *gin.Context) {
	count, err := h.service.CountTasksByWorkflowStep(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.logger.Error("failed to count tasks by step", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count tasks"})
		return
	}
	c.JSON(http.StatusOK, dto.TaskCountResponse{TaskCount: count})
}

type httpBulkMoveTasksRequest struct {
	SourceWorkflowID string   `json:"source_workflow_id"`
	SourceStepID     string   `json:"source_step_id,omitempty"`
	TargetWorkflowID string   `json:"target_workflow_id"`
	TargetStepID     string   `json:"target_step_id"`
	TaskIDs          []string `json:"task_ids,omitempty"`
}

func (h *TaskHandlers) httpBulkMoveTasks(c *gin.Context) {
	var body httpBulkMoveTasksRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.TargetWorkflowID == "" || body.TargetStepID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_workflow_id and target_step_id are required"})
		return
	}
	if len(body.TaskIDs) > 0 {
		h.httpBulkMoveSelectedTasks(c, body)
		return
	}
	if body.SourceWorkflowID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_workflow_id, target_workflow_id, and target_step_id are required"})
		return
	}
	result, err := h.service.BulkMoveTasks(
		c.Request.Context(),
		body.SourceWorkflowID, body.SourceStepID,
		body.TargetWorkflowID, body.TargetStepID,
	)
	if err != nil {
		h.logger.Error("failed to bulk move tasks", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to bulk move tasks"})
		return
	}
	c.JSON(http.StatusOK, dto.BulkMoveTasksResponse{MovedCount: result.MovedCount})
}

func (h *TaskHandlers) httpBulkMoveSelectedTasks(c *gin.Context, body httpBulkMoveTasksRequest) {
	result, err := h.service.BulkMoveSelectedTasks(
		c.Request.Context(),
		body.TaskIDs,
		body.TargetWorkflowID,
		body.TargetStepID,
	)
	if err != nil {
		handleSelectedMoveError(c, h.logger, err)
		return
	}
	c.JSON(http.StatusOK, dto.BulkMoveTasksResponse{MovedCount: result.MovedCount})
}

type httpTaskRepositoryInput struct {
	RepositoryID   string `json:"repository_id"`
	BaseBranch     string `json:"base_branch"`
	CheckoutBranch string `json:"checkout_branch"`
	PRNumber       int    `json:"pr_number,omitempty"`
	LocalPath      string `json:"local_path"`
	Name           string `json:"name"`
	DefaultBranch  string `json:"default_branch"`
	GitHubURL      string `json:"github_url"`
	RemoteURL      string `json:"remote_url"`
	Provider       string `json:"provider"`
	ProviderRepoID string `json:"provider_repo_id"`
	ProviderOwner  string `json:"provider_owner"`
	ProviderName   string `json:"provider_name"`

	// Fresh-branch flow (local executor only): when FreshBranch is true the
	// handler discards uncommitted changes in the local clone and creates
	// NewBranchName from BaseBranch after the task repository is persisted.
	// ConfirmDiscard must be true if the working tree is dirty; otherwise
	// the request is rejected with 409 + the dirty file list.
	// ConsentedDirtyFiles is the dirty-file list the UI showed the user;
	// the backend rejects with 409 if the live dirty set has any path
	// that wasn't on this list, protecting against silent loss of files
	// that became dirty between preflight and execution.
	FreshBranch         bool     `json:"fresh_branch,omitempty"`
	NewBranchName       string   `json:"new_branch_name,omitempty"`
	ConfirmDiscard      bool     `json:"confirm_discard,omitempty"`
	ConsentedDirtyFiles []string `json:"consented_dirty_files,omitempty"`
}

type httpCreateTaskRequest struct {
	WorkspaceID       string                    `json:"workspace_id"`
	WorkflowID        string                    `json:"workflow_id"`
	WorkflowStepID    string                    `json:"workflow_step_id"`
	Title             string                    `json:"title"`
	Description       string                    `json:"description,omitempty"`
	AutoTitle         bool                      `json:"auto_title,omitempty"`
	Autopilot         bool                      `json:"autopilot,omitempty"`
	Priority          string                    `json:"priority,omitempty"`
	State             *v1.TaskState             `json:"state,omitempty"`
	Repositories      []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position          int                       `json:"position,omitempty"`
	Metadata          map[string]interface{}    `json:"metadata,omitempty"`
	StartAgent        bool                      `json:"start_agent,omitempty"`
	PrepareSession    bool                      `json:"prepare_session,omitempty"`
	AgentProfileID    string                    `json:"agent_profile_id,omitempty"`
	ExecutorID        string                    `json:"executor_id,omitempty"`
	ExecutorProfileID string                    `json:"executor_profile_id,omitempty"`
	PlanMode          bool                      `json:"plan_mode,omitempty"`
	Attachments       []v1.MessageAttachment    `json:"attachments,omitempty"`
	ParentID          string                    `json:"parent_id,omitempty"`
	WorkspacePath     string                    `json:"workspace_path,omitempty"`
	BlockedBy         []string                  `json:"blocked_by,omitempty"`
	// StartWhenUnblocked records the agent start as an intent consumed by
	// dependency resolution. nil derives it from StartAgent when BlockedBy is set.
	StartWhenUnblocked *bool  `json:"start_when_unblocked,omitempty"`
	ProjectID          string `json:"project_id,omitempty"`
	// ExternalID is a caller-supplied identity used for create-idempotency
	// (docs/specs/tasks/requirements/external-id-idempotency.md).
	ExternalID string   `json:"external_id,omitempty"`
	Labels     []string `json:"labels,omitempty"`
	// Office task-handoffs phase 5 — workspace policy. Optional; same
	// shape as the MCP create_task_kandev fields.
	WorkspaceMode         string `json:"workspace_mode,omitempty"`
	WorkspaceGroupID      string `json:"workspace_group_id,omitempty"`
	DefaultChildWorkspace string `json:"default_child_workspace,omitempty"`
	DefaultChildOrdering  string `json:"default_child_ordering,omitempty"`
}

type createTaskResponse struct {
	dto.TaskDTO
	TaskSessionID    string `json:"session_id,omitempty"`
	AgentExecutionID string `json:"agent_execution_id,omitempty"`
	// Deduplicated and CreationComplete are required booleans (not
	// presence-only markers) on every create-idempotency outcome, per
	// docs/specs/tasks/requirements/external-id-idempotency.md. Deduplicated is true
	// for both Found outcomes. CreationComplete is false only for
	// Found-unsettled — every other outcome (including CreatedIdentityLost)
	// carries true, because the field means only "this task's required
	// synchronous setup finished", not "an agent is running".
	Deduplicated     bool `json:"deduplicated"`
	CreationComplete bool `json:"creation_complete"`
}

const (
	maxCreateTaskAttachments = 10
	maxAttachmentDataBytes   = 10 * 1024 * 1024 // 10 MB base64 string length cap
)

var allowedAttachmentTypes = map[string]struct{}{
	"image":    {},
	"audio":    {},
	"resource": {},
}

func encodeTaskLabels(labels []string) (string, error) {
	normalized := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, label := range labels {
		label = strings.TrimSpace(label)
		if label == "" {
			continue
		}
		if _, ok := seen[label]; ok {
			continue
		}
		seen[label] = struct{}{}
		normalized = append(normalized, label)
	}
	if len(normalized) == 0 {
		return "", nil
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return "", err
	}
	return string(encoded), nil
}

func validateAttachments(items []v1.MessageAttachment) error {
	if len(items) > maxCreateTaskAttachments {
		return fmt.Errorf("too many attachments (max %d)", maxCreateTaskAttachments)
	}
	var totalSize int64
	var legacyTotalSize int
	for i, a := range items {
		typ := strings.TrimSpace(a.Type)
		if _, ok := allowedAttachmentTypes[typ]; !ok {
			return fmt.Errorf("attachment[%d] has unsupported type %q", i, typ)
		}
		if strings.TrimSpace(a.MimeType) == "" {
			return fmt.Errorf("attachment[%d] mime_type is required", i)
		}
		if !a.HasValidDeliveryMode() {
			return fmt.Errorf("attachment[%d] delivery_mode must be prompt or path", i)
		}
		if a.AttachmentID != "" {
			if a.Data != "" {
				return fmt.Errorf("attachment[%d] descriptors cannot include inline data", i)
			}
			if strings.TrimSpace(a.Name) == "" || strings.TrimSpace(a.MimeType) == "" {
				return fmt.Errorf("attachment[%d] descriptor name and mime_type are required", i)
			}
			if err := service.ValidateAttachmentSize(a.SizeBytes); err != nil {
				return fmt.Errorf("attachment[%d]: %w", i, err)
			}
			totalSize += a.SizeBytes
			continue
		}
		if len(a.Data) == 0 {
			return fmt.Errorf("attachment[%d] data is required", i)
		}
		if len(a.Data) > maxAttachmentDataBytes {
			return fmt.Errorf("attachment[%d] data exceeds size limit", i)
		}
		totalSize += int64(len(a.Data))
		legacyTotalSize += len(a.Data)
	}
	if legacyTotalSize > maxAttachmentDataBytes {
		return fmt.Errorf("total legacy attachment size exceeds limit")
	}
	if totalSize > service.MaxAttachmentBytes {
		return fmt.Errorf("total attachment size exceeds limit")
	}
	return nil
}

func (h *TaskHandlers) httpCreateTask(c *gin.Context) {
	var body httpCreateTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	labels, err := encodeTaskLabels(body.Labels)
	if err != nil {
		h.logger.Error("failed to encode task labels", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encode task labels"})
		return
	}
	if err := validateAttachments(body.Attachments); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if body.WorkspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}
	if (body.StartAgent || body.PrepareSession) && body.AgentProfileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_profile_id is required to start agent"})
		return
	}

	repos, ok := convertCreateTaskRepositories(c, body.Repositories)
	if !ok {
		return
	}

	// Always persist profile IDs in task metadata so they can be used as the
	// task's "default" agent profile. This is needed for deferred agent start
	// (handleTaskMovedNoSession) and workflow steps that explicitly use the
	// workflow/task default profile.
	if body.AgentProfileID != "" {
		if body.Metadata == nil {
			body.Metadata = make(map[string]interface{})
		}
		body.Metadata[models.MetaKeyAgentProfileID] = body.AgentProfileID
		if body.ExecutorProfileID != "" {
			body.Metadata[models.MetaKeyExecutorProfileID] = body.ExecutorProfileID
		}
	}

	title := strings.TrimSpace(body.Title)
	description := strings.TrimSpace(body.Description)

	// Office task-handoffs phase 5: resolve workspace policy from the
	// request + parent task, merge into Metadata, and remember it so the
	// post-create attach can record group membership / blocker chain.
	wsPolicy, policyErr := h.resolveWorkspacePolicy(c.Request.Context(), body)
	if policyErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": policyErr.Error()})
		return
	}
	metadata := wsPolicy.MergeMetadataBlock(body.Metadata)
	var deferredLaunch map[string]interface{}
	if body.StartAgent || body.PrepareSession {
		intent := "prepare"
		if body.StartAgent {
			intent = "start"
		}
		deferredLaunch = map[string]interface{}{
			"intent": intent, "agent_profile_id": body.AgentProfileID, "executor_id": body.ExecutorID,
			"executor_profile_id": body.ExecutorProfileID, "prompt": description,
			"plan_mode":   body.PlanMode,
			"attachments": body.Attachments,
		}
	}

	result, err := h.service.CreateTask(c.Request.Context(), &service.CreateTaskRequest{
		WorkspaceID:        body.WorkspaceID,
		WorkflowID:         body.WorkflowID,
		WorkflowStepID:     body.WorkflowStepID,
		Title:              title,
		Description:        description,
		AutoTitle:          body.AutoTitle,
		Autopilot:          body.Autopilot,
		Priority:           body.Priority,
		State:              body.State,
		Repositories:       convertToServiceRepos(repos),
		Position:           body.Position,
		Metadata:           metadata,
		DeferredLaunch:     deferredLaunch,
		PlanMode:           body.PlanMode,
		ParentID:           body.ParentID,
		WorkspacePath:      body.WorkspacePath,
		BlockedBy:          body.BlockedBy,
		StartWhenUnblocked: body.StartWhenUnblocked,
		ProjectID:          body.ProjectID,
		Labels:             labels,
		ExternalID:         body.ExternalID,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not created")
		return
	}
	// Both Found outcomes have no side effects: skip every post-create step
	// below (attachment claim, workspace-policy attach, fresh-branch commit,
	// session prepare/start, last-used recording, PR association) and return
	// the existing task as-is. This is the data-loss guard's REST twin — the
	// steps below assume a task this request just created, and running them
	// against someone else's task would misapply attachments/policy meant for
	// the retry payload to a task that never asked for them.
	if result.Outcome != service.CreateTaskOutcomeCreated {
		c.JSON(http.StatusOK, foundCreateTaskResponse(result))
		return
	}
	task := result.Task
	if err := h.service.ClaimMessageAttachments(c.Request.Context(), task.ID, "", body.Attachments); err != nil {
		rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 10*time.Second)
		defer cancel()
		if deleteErr := h.service.DeleteTask(rollbackCtx, task.ID); deleteErr != nil {
			h.logger.Warn("failed to roll back task after attachment claim", zap.String("task_id", task.ID), zap.Error(deleteErr))
		}
		switch {
		case errors.Is(err, service.ErrAttachmentTooLarge):
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": err.Error()})
		case errors.Is(err, service.ErrAttachmentForbidden), errors.Is(err, service.ErrAttachmentNotFound), errors.Is(err, service.ErrAttachmentClaimConflict):
			c.JSON(http.StatusNotFound, gin.H{"error": "attachment not found"})
		case errors.Is(err, service.ErrAttachmentInvalid), errors.Is(err, service.ErrAttachmentTotalTooLarge), errors.Is(err, service.ErrTooManyAttachments):
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		default:
			h.logger.Error("failed to claim task attachments", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to claim task attachments"})
		}
		return
	}

	if h.handoffSvc != nil && wsPolicy.NeedsAttachment() {
		if attachErr := h.handoffSvc.AttachWorkspacePolicy(c.Request.Context(), task.ID, body.ParentID, wsPolicy); attachErr != nil {
			h.logger.Error("attach workspace policy; rolling back task creation",
				zap.String("task_id", task.ID), zap.Error(attachErr))
			if delErr := h.service.DeleteTask(c.Request.Context(), task.ID); delErr != nil {
				h.logger.Error("rollback delete failed; task left in inconsistent state",
					zap.String("task_id", task.ID), zap.Error(delErr))
			}
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to attach workspace policy: " + attachErr.Error(),
			})
			return
		}
	}

	if !h.commitFreshBranch(c, task.ID, task.Title, body.WorkspaceID, body.Repositories, repos) {
		return
	}

	taskDTO := dto.FromTask(task)
	response := createTaskResponse{TaskDTO: taskDTO, Deduplicated: false, CreationComplete: true}
	// Use the backend-resolved workflow step ID (from the created task) instead of the request's
	resolvedStepID := taskDTO.WorkflowStepID

	// Synchronous session preparation (create-sequence step 6, "required
	// synchronous post-create work") runs before settlement, not after — see
	// prepareTaskSession's doc comment for why. dispatch is nil unless a
	// start_agent create genuinely prepared a session; it is only ever acted
	// on below once settlement has succeeded.
	// A create that declared dependencies records its start as a
	// start-when-unblocked intent instead of launching now; dependency
	// resolution consumes it later.
	startWhenUnblocked := service.ResolveStartWhenUnblocked(&service.CreateTaskRequest{
		BlockedBy: body.BlockedBy, StartWhenUnblocked: body.StartWhenUnblocked,
	})
	response.StartWhenUnblocked = startWhenUnblocked

	// Blockers suppress the immediate launch regardless of start_when_unblocked:
	// that flag governs the deferred launch only, so `false` must not mean
	// "start the blocked task now".
	dispatch := h.prepareTaskSession(c, &response, taskDTO.ID, body, resolvedStepID,
		task.QueuedForStepID == "" && len(body.BlockedBy) == 0)

	// Settlement (create-sequence step 7): after all required synchronous
	// work above — including session prepare — and before any asynchronous
	// dispatch below.
	settled, survivor, settleErr := h.service.SettleExternalID(c.Request.Context(), task.ID, task.ExternalID)
	if settleErr != nil {
		if !isNotFound(settleErr) {
			h.logger.Error("failed to settle external_id", zap.String("task_id", task.ID), zap.Error(settleErr))
		}
		handleNotFound(c, h.logger, settleErr, "task not created")
		return
	}
	if !settled {
		// CreatedIdentityLost: another actor released the identity while this
		// create was running. The task survives holding no external_id; per
		// the spec, no asynchronous work (session start, PR association) is
		// dispatched for it. Any session prepared above is not launched.
		c.JSON(http.StatusOK, createTaskResponse{
			TaskDTO:          dto.FromTask(survivor),
			Deduplicated:     false,
			CreationComplete: true,
		})
		return
	}

	h.dispatchTaskSession(taskDTO.ID, taskDTO.Description, body, dispatch)
	h.recordTaskCreateLastUsed(c.Request.Context(), body, repos)

	// Associate PR with task if any repository input contains a PR URL
	h.associatePRFromRepoInputs(taskDTO.ID, response.TaskSessionID, body.Repositories)

	c.JSON(http.StatusOK, response)
}

// foundCreateTaskResponse builds the response for either Found outcome: the
// existing task, deduplicated true, and creation_complete reflecting whether
// that task's own create had finished settling.
func foundCreateTaskResponse(result service.CreateTaskResult) createTaskResponse {
	return createTaskResponse{
		TaskDTO:          dto.FromTask(result.Task),
		Deduplicated:     true,
		CreationComplete: result.Outcome == service.CreateTaskOutcomeFoundSettled,
	}
}

// lookupTaskResponse is the by-external-id GET route's body: the task DTO
// plus creation_complete. Unlike createTaskResponse, it carries no
// deduplicated field — that flag is meaningful only relative to a create
// this request performed, and a lookup never creates anything.
type lookupTaskResponse struct {
	dto.TaskDTO
	CreationComplete bool `json:"creation_complete"`
}

// httpGetTaskByExternalID is the REST lookup route
// (docs/specs/tasks/system-design/external-id-idempotency.md, "REST — lookup"): a
// side-effect-free way to ask what holds an identity without risking a
// create. Returns the task whether settled or not, including archived tasks.
func (h *TaskHandlers) httpGetTaskByExternalID(c *gin.Context) {
	task, err := h.service.GetTaskByExternalID(c.Request.Context(), c.Param("id"), c.Query("external_id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	c.JSON(http.StatusOK, lookupTaskResponse{
		TaskDTO:          dto.FromTask(task),
		CreationComplete: task.ExternalIDSettledAt != nil,
	})
}

// httpReleaseTaskExternalID is the REST release route
// (docs/specs/tasks/system-design/external-id-idempotency.md, "REST — release"): an
// operator action for an identity a human has determined is abandoned. Frees
// the identity without deleting or otherwise modifying the task. MUST NOT be
// called automatically in response to creation_complete:false — see "The one
// unsafe thing a caller can do".
func (h *TaskHandlers) httpReleaseTaskExternalID(c *gin.Context) {
	released, err := h.service.ReleaseTaskExternalID(c.Request.Context(), c.Param("id"), c.Query("external_id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	if !released {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *TaskHandlers) recordTaskCreateLastUsed(ctx context.Context, body httpCreateTaskRequest, repos []dto.TaskRepositoryInput) {
	if h.taskCreateLastUsedRecorder == nil {
		return
	}
	patch := buildTaskCreateLastUsedPatch(body, repos)
	if err := h.taskCreateLastUsedRecorder.RecordTaskCreateLastUsed(ctx, patch); err != nil {
		h.logger.Warn("failed to record task-create last-used settings", zap.Error(err))
	}
}

func buildTaskCreateLastUsedPatch(body httpCreateTaskRequest, repos []dto.TaskRepositoryInput) usermodels.TaskCreateLastUsed {
	patch := usermodels.TaskCreateLastUsed{
		AgentProfileID:    body.AgentProfileID,
		ExecutorProfileID: body.ExecutorProfileID,
	}
	if body.WorkspaceID != "" && body.WorkflowID != "" {
		patch.WorkflowIDsByWorkspace = map[string]string{body.WorkspaceID: body.WorkflowID}
	}
	for i, repo := range repos {
		if repo.RepositoryID == "" {
			continue
		}
		branch := taskCreateLastUsedBranch(body, i, repo)
		patch.RepositoryID = repo.RepositoryID
		patch.Branch = branch
		break
	}
	return patch
}

func taskCreateLastUsedBranch(
	body httpCreateTaskRequest,
	index int,
	repo dto.TaskRepositoryInput,
) string {
	if index < len(body.Repositories) && body.Repositories[index].FreshBranch {
		raw := body.Repositories[index]
		return firstNonEmpty(raw.BaseBranch, raw.CheckoutBranch)
	}
	return firstNonEmpty(repo.CheckoutBranch, repo.BaseBranch)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

// commitFreshBranch wraps the post-CreateTask fresh-branch sequence: run the
// destructive checkout, compensate by deleting the task if it fails, then
// persist the rewritten BaseBranch onto the now-existing task. Returns false
// when an HTTP error response was already written.
func (h *TaskHandlers) commitFreshBranch(
	c *gin.Context,
	taskID, title, workspaceID string,
	inputs []httpTaskRepositoryInput,
	repos []dto.TaskRepositoryInput,
) bool {
	hasFresh := false
	for _, raw := range inputs {
		if raw.FreshBranch {
			hasFresh = true
			break
		}
	}
	if !hasFresh {
		// No fresh-branch opt-in — repos were already persisted by CreateTask,
		// so skip the destructive checkout and the DELETE+INSERT rewrite.
		return true
	}
	task, err := h.service.GetTask(c.Request.Context(), taskID)
	if err != nil {
		h.logger.Error("failed to reload task repositories for fresh branch", zap.String("task_id", taskID), zap.Error(err))
		h.rollbackFreshBranchTask(c.Request.Context(), taskID)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve task repository"})
		return false
	}
	if !h.applyFreshBranch(c, title, inputs, repos, task.Repositories) {
		h.rollbackFreshBranchTask(c.Request.Context(), taskID)
		return false
	}
	// Persist the rewritten BaseBranch (set by applyFreshBranch) onto the task.
	// applyFreshBranch already mutated the git repo, so we can't roll back —
	// but we must surface a 5xx so the caller knows the DB still references
	// the user's original fork point. Otherwise every subsequent session
	// resume would silently check out the old branch and abandon the new one.
	if err := h.service.ReplaceTaskRepositories(c.Request.Context(), taskID, workspaceID, convertToServiceRepos(repos)); err != nil {
		h.logger.Error("failed to persist fresh-branch base branch onto task",
			zap.String("task_id", taskID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "fresh branch was created but the task record could not be updated; please check the repository",
		})
		return false
	}
	return true
}

func (h *TaskHandlers) rollbackFreshBranchTask(ctx context.Context, taskID string) {
	if err := h.service.DeleteTask(ctx, taskID); err != nil {
		h.logger.Warn("failed to compensate by deleting task after fresh-branch failure",
			zap.String("task_id", taskID), zap.Error(err))
	}
}

// applyFreshBranch executes the fresh-branch flow for any local-executor
// repository inputs that opted in, resolving each repository from the ordered
// rows persisted on the new task. Mutates `repos[i].BaseBranch` to the
// newly-created branch on success so the persisted task uses it as the
// effective base branch on every session resume. Writes the appropriate
// HTTP error response and returns false on failure.
//
// When the caller doesn't supply NewBranchName, the backend generates a
// semantic name from the task title (matching the worktree executor's
// branch-naming) so the user only has to flip a switch.
func (h *TaskHandlers) applyFreshBranch(
	c *gin.Context,
	taskTitle string,
	inputs []httpTaskRepositoryInput,
	repos []dto.TaskRepositoryInput,
	persisted []*models.TaskRepository,
) bool {
	ctx := c.Request.Context()
	for i, raw := range inputs {
		if !raw.FreshBranch {
			continue
		}
		if i >= len(persisted) || persisted[i] == nil || persisted[i].RepositoryID == "" {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve task repository"})
			return false
		}
		repositoryID := persisted[i].RepositoryID
		baseBranch := raw.BaseBranch
		if baseBranch == "" {
			// User didn't pick one — fall back to the repo's checked-out branch.
			baseBranch, _ = h.service.RepositoryCurrentBranch(ctx, repositoryID)
		}
		newBranch := resolveFreshBranchName(raw.NewBranchName, taskTitle)
		err := h.service.PerformFreshBranch(ctx, service.FreshBranchRequest{
			RepositoryID:        repositoryID,
			BaseBranch:          baseBranch,
			NewBranch:           newBranch,
			ConfirmDiscard:      raw.ConfirmDiscard,
			ConsentedDirtyFiles: raw.ConsentedDirtyFiles,
		})
		if err != nil {
			h.respondFreshBranchError(c, err)
			return false
		}
		repos[i].BaseBranch = newBranch
		repos[i].CheckoutBranch = ""
	}
	return true
}

// resolveFreshBranchName returns the user-supplied branch name when present,
// otherwise generates one from the task title using the same semantic name +
// random-suffix scheme as the worktree executor. Returns an empty string only
// when both the raw name and the task title would produce nothing useful;
// PerformFreshBranch's sanitizeGitRef rejects that downstream.
func resolveFreshBranchName(rawNewBranch, taskTitle string) string {
	if name := strings.TrimSpace(rawNewBranch); name != "" {
		return name
	}
	return worktree.SemanticWorktreeName(taskTitle, worktree.SmallSuffix(3))
}

func (h *TaskHandlers) respondFreshBranchError(c *gin.Context, err error) {
	var dirty *service.ErrDirtyWorkingTree
	if errors.As(err, &dirty) {
		c.JSON(http.StatusConflict, gin.H{
			"error":       "working tree has uncommitted changes",
			"dirty_files": dirty.DirtyFiles,
		})
		return
	}
	if errors.Is(err, service.ErrInvalidGitRef) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, service.ErrFreshBranchCheckout) {
		// git checkout failure (e.g. branch already exists, base branch unknown).
		// Surface the underlying message — it tells the user what to fix.
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, service.ErrPartialDiscard) {
		// Tracked files already gone, untracked survived. The user needs to
		// know they have lost work before retrying — never a generic 500.
		h.logger.Error("partial discard during fresh-branch", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "tracked changes were discarded but git clean failed; untracked files remain — inspect the repo before retrying",
			"partial": true,
		})
		return
	}
	h.logger.Error("fresh branch checkout failed", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare fresh branch"})
}

// convertCreateTaskRepositories converts httpTaskRepositoryInput slice to dto.TaskRepositoryInput slice.
// Returns (nil, false) and writes a 400 response if any entry is missing both repository_id and local_path.
func convertCreateTaskRepositories(c *gin.Context, inputs []httpTaskRepositoryInput) ([]dto.TaskRepositoryInput, bool) {
	var repos []dto.TaskRepositoryInput
	for _, r := range inputs {
		if r.RepositoryID == "" && r.LocalPath == "" && r.RemoteURL == "" && r.GitHubURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "repository_id, local_path, or remote_url is required"})
			return nil, false
		}
		repos = append(repos, dto.TaskRepositoryInput{
			RepositoryID:   r.RepositoryID,
			BaseBranch:     r.BaseBranch,
			CheckoutBranch: r.CheckoutBranch,
			PRNumber:       r.PRNumber,
			LocalPath:      r.LocalPath,
			Name:           r.Name,
			DefaultBranch:  r.DefaultBranch,
			GitHubURL:      r.GitHubURL,
			RemoteURL:      r.RemoteURL,
			Provider:       r.Provider,
			ProviderRepoID: r.ProviderRepoID,
			ProviderOwner:  r.ProviderOwner,
			ProviderName:   r.ProviderName,
		})
	}
	return repos, true
}

// associatePRFromRepoInputs checks if any repository input contains a GitHub PR URL
// (e.g., github.com/owner/repo/pull/123) and fires the onTaskCreatedWithPR callback
// in a background goroutine to associate the PR with the newly created task.
func (h *TaskHandlers) associatePRFromRepoInputs(taskID, sessionID string, repos []httpTaskRepositoryInput) {
	if h.onTaskCreatedWithPR == nil {
		return
	}
	for _, r := range repos {
		if r.GitHubURL != "" && strings.Contains(r.GitHubURL, "/pull/") {
			prURL := r.GitHubURL
			branch := r.CheckoutBranch
			go h.onTaskCreatedWithPR(context.Background(), taskID, sessionID, prURL, branch)
			return // only one PR per task
		}
	}
}

// startAgentDispatch carries what dispatchTaskSession needs to launch the
// deferred agent start (create-sequence step 8) after settlement has
// succeeded. A nil *startAgentDispatch means there is nothing to dispatch —
// prepare-only, start_agent not requested, or prepare itself failed.
type startAgentDispatch struct {
	sessionID string
}

// prepareTaskSession runs the create sequence's synchronous session-setup
// step (part of step 6, "required synchronous post-create work") — it MUST
// run, and be allowed to fail, before settlement (step 7). Only the
// resulting agent launch is asynchronous dispatch (step 8), which must run
// after settlement (docs/specs/tasks/system-design/external-id-idempotency.md,
// "Settlement call site (normative, per surface)": "the helper must expose
// preparation and dispatch separately so settlement can sit between them").
// A prior shape settled before calling this at all, which satisfied "no
// dispatch precedes settlement" too narrowly — a crash between settling and
// preparing would report creation_complete:true for a task whose session
// never got created, and a retry would never attempt session-prep again
// since Found outcomes skip all post-create work by design. Reordering fixes
// that: any failure or crash during prepare now leaves the row unsettled, so
// a retry correctly reports FoundUnsettled instead.
//
// A prepare failure (as opposed to a crash) is not itself fatal to task
// creation — matching the existing behavior for a request with no
// start_agent/prepare_session at all — so it is logged and the caller
// proceeds to settle normally; only a genuine crash before this call returns
// leaves the row unsettled.
func (h *TaskHandlers) prepareTaskSession(
	c *gin.Context,
	response *createTaskResponse,
	taskID string,
	body httpCreateTaskRequest,
	resolvedStepID string,
	canLaunch bool,
) *startAgentDispatch {
	if !canLaunch || h.orchestrator == nil || body.AgentProfileID == "" {
		return nil
	}
	if body.PrepareSession && !body.StartAgent {
		// Prepare-only: no follow-up start is coming, so DeferredStart is
		// intentionally omitted — a passthrough profile should be eagerly
		// upgraded to a full launch here so the terminal has a PTY to attach to.
		// (Contrast the start_agent branch below, which sets DeferredStart=true.)
		resp, err := h.orchestrator.LaunchSession(c.Request.Context(), &orchestrator.LaunchSessionRequest{
			TaskID:            taskID,
			Intent:            orchestrator.IntentPrepare,
			AgentProfileID:    body.AgentProfileID,
			ExecutorID:        body.ExecutorID,
			ExecutorProfileID: body.ExecutorProfileID,
			WorkflowStepID:    resolvedStepID,
			LaunchWorkspace:   true,
		})
		if err != nil {
			h.logger.Error("failed to prepare session for task", zap.Error(err), zap.String("task_id", taskID))
		} else {
			response.TaskSessionID = resp.SessionID
		}
		return nil
	}
	if !body.StartAgent {
		return nil
	}
	return h.prepareStartAgentSession(c.Request.Context(), response, taskID, body, resolvedStepID)
}

// prepareStartAgentSession runs the synchronous half of a start_agent create:
// creates the session entry so the caller can return a session ID
// immediately, without launching the workspace (the deferred async start
// below handles that, avoiding a 30-60s block on remote executors).
func (h *TaskHandlers) prepareStartAgentSession(
	ctx context.Context,
	response *createTaskResponse,
	taskID string,
	body httpCreateTaskRequest,
	resolvedStepID string,
) *startAgentDispatch {
	prepResp, err := h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:            taskID,
		Intent:            orchestrator.IntentPrepare,
		AgentProfileID:    body.AgentProfileID,
		ExecutorID:        body.ExecutorID,
		ExecutorProfileID: body.ExecutorProfileID,
		WorkflowStepID:    resolvedStepID,
		// The async IntentStartCreated dispatch below carries the prompt. Mark
		// this as a deferred start so a passthrough profile is not eagerly
		// launched here with an empty prompt (which would pre-empt that
		// prompt-bearing start).
		DeferredStart: true,
	})
	if err != nil {
		h.logger.Error("failed to prepare session for task", zap.Error(err), zap.String("task_id", taskID))
		return nil
	}
	sessionID := prepResp.SessionID
	response.TaskSessionID = sessionID
	if updatedTask, updateErr := h.service.UpdateTaskState(ctx, taskID, v1.TaskStateScheduling); updateErr != nil {
		h.logger.Warn("failed to mark task scheduling after preparing start session",
			zap.Error(updateErr),
			zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
	} else {
		response.State = updatedTask.State
	}
	return &startAgentDispatch{sessionID: sessionID}
}

// dispatchTaskSession launches the agent asynchronously (create-sequence
// step 8). Callers MUST only invoke this after settlement has succeeded —
// dispatch is nil whenever there was nothing prepared to dispatch.
func (h *TaskHandlers) dispatchTaskSession(taskID, description string, body httpCreateTaskRequest, dispatch *startAgentDispatch) {
	if dispatch == nil {
		return
	}
	sessionID := dispatch.sessionID
	// Launch agent asynchronously so the HTTP request can return immediately.
	// The frontend will receive WebSocket updates when the agent actually starts.
	go func() {
		startCtx, cancel := context.WithTimeout(context.Background(), constants.AgentLaunchTimeout)
		defer cancel()
		launchResp, err := h.orchestrator.LaunchSession(startCtx, &orchestrator.LaunchSessionRequest{
			TaskID:            taskID,
			Intent:            orchestrator.IntentStartCreated,
			SessionID:         sessionID,
			AgentProfileID:    body.AgentProfileID,
			Prompt:            description,
			SkipMessageRecord: false,
			PlanMode:          body.PlanMode,
			Attachments:       body.Attachments,
		})
		if err != nil {
			h.logger.Error("failed to start agent for task (async)", zap.Error(err), zap.String("task_id", taskID), zap.String("session_id", sessionID))
			return
		}
		h.logger.Info("agent started for task (async)",
			zap.String("task_id", taskID),
			zap.String("session_id", launchResp.SessionID),
			zap.String("execution_id", launchResp.AgentExecutionID))
	}()
}

type httpUpdateTaskRequest struct {
	Title        *string                   `json:"title,omitempty"`
	Description  *string                   `json:"description,omitempty"`
	Priority     *string                   `json:"priority,omitempty"`
	State        *v1.TaskState             `json:"state,omitempty"`
	Repositories []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position     *int                      `json:"position,omitempty"`
	Metadata     map[string]interface{}    `json:"metadata,omitempty"`
	// ParentID nests the task under another task. "" clears the parent.
	ParentID *string `json:"parent_id,omitempty"`
}

type httpUpdateTaskPortForwardingRequest struct {
	Enabled *bool `json:"enabled"`
}

func (h *TaskHandlers) httpUpdateTaskPortForwarding(c *gin.Context) {
	var body httpUpdateTaskPortForwardingRequest
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil || body.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be a boolean"})
		return
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be a boolean"})
		return
	}

	task, err := h.service.UpdateTaskMetadata(c.Request.Context(), c.Param("id"), map[string]interface{}{
		models.MetaKeyPortForwardingEnabled: *body.Enabled,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not updated")
		return
	}
	c.JSON(http.StatusOK, dto.FromTask(task))
}

func (h *TaskHandlers) httpUpdateTask(c *gin.Context) {
	var body httpUpdateTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	// Convert repositories if provided
	var repos []dto.TaskRepositoryInput
	if body.Repositories != nil {
		for _, r := range body.Repositories {
			repos = append(repos, dto.TaskRepositoryInput{
				RepositoryID:   r.RepositoryID,
				BaseBranch:     r.BaseBranch,
				CheckoutBranch: r.CheckoutBranch,
				PRNumber:       r.PRNumber,
				LocalPath:      r.LocalPath,
				Name:           r.Name,
				DefaultBranch:  r.DefaultBranch,
				GitHubURL:      r.GitHubURL,
				RemoteURL:      r.RemoteURL,
				Provider:       r.Provider,
				ProviderRepoID: r.ProviderRepoID,
				ProviderOwner:  r.ProviderOwner,
				ProviderName:   r.ProviderName,
			})
		}
	}

	// Trim strings like the controller did
	var title *string
	if body.Title != nil {
		trimmed := strings.TrimSpace(*body.Title)
		title = &trimmed
	}
	var description *string
	if body.Description != nil {
		trimmed := strings.TrimSpace(*body.Description)
		description = &trimmed
	}

	task, err := h.service.UpdateTask(c.Request.Context(), c.Param("id"), &service.UpdateTaskRequest{
		Title:        title,
		Description:  description,
		Priority:     body.Priority,
		State:        body.State,
		Repositories: convertUpdateRepositories(body.Repositories != nil, repos),
		Position:     body.Position,
		Metadata:     body.Metadata,
		ParentID:     body.ParentID,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not updated")
		return
	}
	c.JSON(http.StatusOK, dto.FromTask(task))
}

func (h *TaskHandlers) httpDetachTask(c *gin.Context) {
	task, err := h.service.DetachTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	c.JSON(http.StatusOK, dto.FromTask(task))
}

type httpUpdateTaskRepositoryRequest struct {
	BaseBranch string `json:"base_branch"`
}

// httpUpdateTaskRepository handles PATCH /tasks/:id/repositories/:repo_id.
// Today it only mutates base_branch; future per-row fields can be added on
// httpUpdateTaskRepositoryRequest. Mirrors the WS / MCP paths through the
// same service method so all three surfaces stay in sync.
func (h *TaskHandlers) httpUpdateTaskRepository(c *gin.Context) {
	var body httpUpdateTaskRepositoryRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	taskRepo, err := h.service.UpdateRepositoryBaseBranch(c.Request.Context(), service.UpdateRepositoryBaseBranchRequest{
		TaskID:           c.Param("id"),
		TaskRepositoryID: c.Param("repo_id"),
		BaseBranch:       body.BaseBranch,
	})
	if err != nil {
		if errors.Is(err, service.ErrTaskRepositoryNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		// Distinguish caller-fixable validation failures (required field
		// missing, unsafe ref name, …) from server-side faults (DB write
		// errors propagated up from the service). Anything that matches a
		// known validation message stays at 400; everything else escalates
		// to 500 so client retries don't mask backend regressions.
		if isValidationError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		// Avoid echoing raw service errors on the 500 path — DB / IO
		// failures can carry connection strings, table names, or stack
		// traces. Log the detail server-side; return an opaque message.
		h.logger.Error("update task repository failed",
			zap.String("task_id", c.Param("id")),
			zap.String("repo_id", c.Param("repo_id")),
			zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update task repository"})
		return
	}
	c.JSON(http.StatusOK, taskRepo)
}

type httpMoveTaskRequest struct {
	WorkflowID     string `json:"workflow_id"`
	WorkflowStepID string `json:"workflow_step_id"`
	Position       int    `json:"position"`
}

func (h *TaskHandlers) httpMoveTask(c *gin.Context) {
	var body httpMoveTaskRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.WorkflowID == "" || body.WorkflowStepID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow_id and workflow_step_id are required"})
		return
	}
	result, err := h.service.MoveTaskWithOptions(
		c.Request.Context(), c.Param("id"),
		body.WorkflowID, body.WorkflowStepID, body.Position,
		service.MoveTaskOptions{AllowActivePrimarySession: true, StepHistoryActor: wfmodels.StepTransitionActorHuman},
	)
	if err != nil {
		handleSelectedMoveError(c, h.logger, err)
		return
	}

	response := dto.MoveTaskResponse{
		Task: dto.FromTask(result.Task),
	}
	if result.WorkflowStep != nil {
		response.WorkflowStep = dto.FromWorkflowStep(result.WorkflowStep)
	}
	c.JSON(http.StatusOK, response)
}

func (h *TaskHandlers) httpDeleteTask(c *gin.Context) {
	// WithoutCancel, not Background: a delete tears down worktrees, containers
	// and subtree rows, so it must survive the client navigating away — but
	// context.Background() also dropped the caller identity the auth middleware
	// attached, and an identity-free context reads as an internal caller, which
	// authorizes everything. WithoutCancel keeps the request's values and drops
	// only its cancellation.
	deleteCtx, cancel := context.WithTimeout(
		context.WithoutCancel(c.Request.Context()), constants.TaskDeleteTimeout)
	defer cancel()
	taskID := c.Param("id")
	cascade := cascadeQueryParam(c)
	// Office task-handoffs phase 6: route through HandoffService.DeleteTaskTree
	// when wired so descendant runs are cancelled, group memberships are
	// released with reason=deleted, and the cleanup state machine fires.
	if h.handoffSvc != nil {
		if _, err := h.handoffSvc.DeleteTaskTree(deleteCtx, taskID, cascade); err != nil {
			handleNotFound(c, h.logger, err, "task not deleted")
			return
		}
		c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
		return
	}
	if err := h.service.DeleteTask(deleteCtx, taskID); err != nil {
		handleNotFound(c, h.logger, err, "task not deleted")
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
}

func (h *TaskHandlers) httpArchiveTask(c *gin.Context) {
	taskID := c.Param("id")
	cascade := cascadeQueryParam(c)
	// Office task-handoffs phase 6: when a HandoffService is wired,
	// archive the whole subtree under a single cascade ID so
	// descendants get tagged for scoped unarchive AND workspace-group
	// memberships are released. When HandoffService is unconfigured
	// (legacy / tests) fall back to the single-task path.
	if h.handoffSvc != nil {
		if _, err := h.handoffSvc.ArchiveTaskTree(c.Request.Context(), taskID, cascade); err != nil {
			handleNotFound(c, h.logger, err, "task not archived")
			return
		}
		c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
		return
	}
	if err := h.service.ArchiveTask(c.Request.Context(), taskID); err != nil {
		handleNotFound(c, h.logger, err, "task not archived")
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
}

// cascadeQueryParam returns whether the archive/delete request asked to
// cascade into subtasks. Default is false — subtasks are preserved
// unless the client explicitly opts in via ?cascade=true.
func cascadeQueryParam(c *gin.Context) bool {
	return strings.EqualFold(c.Query("cascade"), "true")
}

// httpTaskSubtaskCount returns the count of direct, non-archived,
// non-ephemeral subtasks for a task. Used by the frontend's archive /
// delete confirmation dialogs to decide whether to render the
// "Also archive/delete subtasks" checkbox.
func (h *TaskHandlers) httpTaskSubtaskCount(c *gin.Context) {
	taskID := c.Param("id")
	children, err := h.repo.ListChildren(c.Request.Context(), taskID)
	if err != nil {
		// Don't surface the raw repo error to the client — it can leak
		// driver / SQL details. Log the full reason server-side, return
		// a generic 500 to the caller.
		h.logger.Error("failed to list direct subtasks",
			zap.String("task_id", taskID), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to count subtasks"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(children)})
}

// httpUnarchiveTask routes through HandoffService.UnarchiveTaskTree so
// every task this cascade archived (and only those) is restored. The
// handler returns 503 when no HandoffService is wired since unarchive
// is meaningless without the cascade infrastructure.
func (h *TaskHandlers) httpUnarchiveTask(c *gin.Context) {
	if h.handoffSvc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "unarchive requires office task-handoffs to be configured",
		})
		return
	}
	taskID := c.Param("id")
	outcome, err := h.handoffSvc.UnarchiveTaskTree(c.Request.Context(), taskID)
	if err != nil {
		handleNotFound(c, h.logger, err, "task not unarchived")
		return
	}
	// Probe branch recoverability for every restored task: archive deleted
	// the local branch + worktree, so report whether the branch still
	// exists (locally or on origin) and restore checkout_branch so the
	// next session picks the old work back up. Best-effort — an empty
	// list just means nothing was recoverable. Detached from the request
	// context: the tasks are already unarchived, so a client disconnect
	// must not skip the checkout_branch restore.
	recoveryCtx, cancelRecovery := context.WithTimeout(
		context.WithoutCancel(c.Request.Context()),
		h.detachedRecoveryTimeout(),
	)
	defer cancelRecovery()
	recovery := make([]service.BranchRecovery, 0)
	for _, id := range outcome.ArchivedTaskIDs {
		recovery = append(recovery, h.service.RecoverTaskBranches(recoveryCtx, id)...)
	}
	workspaceRecovery := make([]storageworkspaces.WorkspaceRecovery, 0, len(outcome.ArchivedTaskIDs))
	if h.workspaceRestorer != nil {
		for _, id := range outcome.ArchivedTaskIDs {
			workspaceRecovery = append(workspaceRecovery, h.workspaceRestorer.RestoreTask(recoveryCtx, id))
		}
	}
	c.JSON(http.StatusOK, gin.H{
		"success":            true,
		"cascade_id":         outcome.CascadeID,
		"unarchived_ids":     outcome.ArchivedTaskIDs,
		"skipped_ids":        outcome.SkippedTaskIDs,
		"affected_group_ids": outcome.ReleasedGroupIDs,
		"workspace_recovery": workspaceRecovery,
		"recovery":           recovery,
	})
}

// httpStartQuickChatRequest is the request body for starting a quick chat session.
type httpStartQuickChatRequest struct {
	Title             string                         `json:"title,omitempty"`
	RepositoryID      string                         `json:"repository_id,omitempty"`
	Repositories      []httpQuickChatRepositoryInput `json:"repositories,omitempty"`
	AgentProfileID    string                         `json:"agent_profile_id,omitempty"`
	ExecutorID        string                         `json:"executor_id,omitempty"`
	Prompt            string                         `json:"prompt,omitempty"`
	LocalPath         string                         `json:"local_path,omitempty"`
	RepositoryName    string                         `json:"repository_name,omitempty"`
	DefaultBranch     string                         `json:"default_branch,omitempty"`
	BaseBranch        string                         `json:"base_branch,omitempty"`
	LaunchImmediately bool                           `json:"launch_immediately,omitempty"`
}

type httpQuickChatRepositoryInput struct {
	RepositoryID string `json:"repository_id"`
	BaseBranch   string `json:"base_branch"`
}

func (body *httpStartQuickChatRequest) validateRepositories() error {
	hasLegacyRepository := body.RepositoryID != "" || body.LocalPath != "" ||
		body.RepositoryName != "" || body.DefaultBranch != "" || body.BaseBranch != ""
	if len(body.Repositories) > 0 && hasLegacyRepository {
		return errors.New("repositories cannot be combined with legacy repository fields")
	}
	seen := make(map[string]struct{}, len(body.Repositories))
	for _, repo := range body.Repositories {
		if repo.RepositoryID == "" {
			return errors.New("repository_id is required")
		}
		if repo.BaseBranch == "" {
			return fmt.Errorf("base_branch is required for repository %q", repo.RepositoryID)
		}
		if _, exists := seen[repo.RepositoryID]; exists {
			return fmt.Errorf("repository %q can only be selected once", repo.RepositoryID)
		}
		seen[repo.RepositoryID] = struct{}{}
	}
	return nil
}

// httpStartQuickChatResponse is returned when a quick chat session is created.
type httpStartQuickChatResponse struct {
	TaskID    string `json:"task_id"`
	SessionID string `json:"session_id"`
}

// quickChatParams holds resolved parameters for creating a quick chat session.
type quickChatParams struct {
	agentProfileID string
	executorID     string
	title          string
	repos          []service.TaskRepositoryInput
	metadata       map[string]interface{}
}

// buildQuickChatRepositories builds the repository input list from the request.
func (body *httpStartQuickChatRequest) buildRepositories() []service.TaskRepositoryInput {
	if len(body.Repositories) > 0 {
		repos := make([]service.TaskRepositoryInput, 0, len(body.Repositories))
		for _, repo := range body.Repositories {
			repos = append(repos, service.TaskRepositoryInput{
				RepositoryID: repo.RepositoryID,
				BaseBranch:   repo.BaseBranch,
			})
		}
		return repos
	}
	if body.RepositoryID == "" && body.LocalPath == "" {
		return nil
	}
	return []service.TaskRepositoryInput{{
		RepositoryID:  body.RepositoryID,
		LocalPath:     body.LocalPath,
		Name:          body.RepositoryName,
		DefaultBranch: body.DefaultBranch,
		BaseBranch:    body.BaseBranch,
	}}
}

// resolveParams resolves agent/executor IDs and builds metadata for quick chat.
func (body *httpStartQuickChatRequest) resolveParams(workspace *models.Workspace) quickChatParams {
	agentProfileID := body.AgentProfileID
	if agentProfileID == "" && workspace.DefaultAgentProfileID != nil {
		agentProfileID = *workspace.DefaultAgentProfileID
	}
	repos := body.buildRepositories()
	executorID := body.ExecutorID
	if len(repos) > 0 {
		executorID = models.ExecutorIDWorktree
	} else if executorID == "" && workspace.DefaultExecutorID != nil {
		executorID = *workspace.DefaultExecutorID
	}

	metadata := make(map[string]interface{})
	if agentProfileID != "" {
		metadata[models.MetaKeyAgentProfileID] = agentProfileID
	}
	if executorID != "" {
		metadata[models.MetaKeyExecutorID] = executorID
	}

	title := body.Title
	if title == "" {
		title = "Quick Chat"
	}

	return quickChatParams{
		agentProfileID: agentProfileID,
		executorID:     executorID,
		title:          title,
		repos:          repos,
		metadata:       metadata,
	}
}

// httpStartQuickChat creates an ephemeral task and prepares a session for quick chat.
func (h *TaskHandlers) httpStartQuickChat(c *gin.Context) {
	workspaceID := c.Param("id")
	var body httpStartQuickChatRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if err := body.validateRepositories(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx := c.Request.Context()

	workspace, err := h.service.GetWorkspace(ctx, workspaceID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not found")
		return
	}

	params := body.resolveParams(workspace)
	if params.agentProfileID == "" {
		h.logger.Error("no agent profile configured for quick chat", zap.String("workspace_id", workspaceID))
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace has no default agent profile configured"})
		return
	}

	result, err := h.service.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID:  workspaceID,
		Title:        params.title,
		Description:  body.Prompt,
		Repositories: params.repos,
		IsEphemeral:  true,
		Metadata:     params.metadata,
	})
	if err != nil {
		h.logger.Error("failed to create ephemeral task", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create quick chat"})
		return
	}
	task := result.Task

	// Eager-init: launch the agent process up-front so ACP `initialize` + `session/new`
	// fire and the agent emits available_commands/modes/models. This populates the
	// slash menu, mode dropdown, and model selector before the user sends any prompt.
	resp, err := h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:         task.ID,
		Intent:         orchestrator.IntentStart,
		AgentProfileID: params.agentProfileID,
		ExecutorID:     params.executorID,
	})
	if err != nil {
		// Rollback: delete the ephemeral task to prevent orphans. Use a fresh
		// background context — the request context may already be cancelled
		// (e.g. client aborted, deadline exceeded), and we still want cleanup
		// to run. TaskDeleteTimeout matches the other DeleteTask call sites
		// in this file so a future change to the constant covers this path too.
		rollbackCtx, cancel := context.WithTimeout(context.Background(), constants.TaskDeleteTimeout)
		defer cancel()
		if deleteErr := h.service.DeleteTask(rollbackCtx, task.ID); deleteErr != nil {
			h.logger.Error("failed to rollback quick chat task",
				zap.String("task_id", task.ID),
				zap.Error(deleteErr))
		}
		h.logger.Error("failed to start quick chat session", zap.Error(err), zap.String("task_id", task.ID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start session"})
		return
	}

	h.logger.Info("quick chat session created",
		zap.String("task_id", task.ID),
		zap.String("session_id", resp.SessionID),
		zap.String("workspace_id", workspaceID))

	c.JSON(http.StatusOK, httpStartQuickChatResponse{
		TaskID:    task.ID,
		SessionID: resp.SessionID,
	})
}

// httpQuickChatSession is one restorable quick-chat tab.
type httpQuickChatSession struct {
	SessionID      string `json:"session_id"`
	TaskID         string `json:"task_id"`
	WorkspaceID    string `json:"workspace_id"`
	Kind           string `json:"kind"`
	Name           string `json:"name,omitempty"`
	AgentProfileID string `json:"agent_profile_id,omitempty"`
}

// httpListQuickChatSessionsResponse mirrors the quick-chat slice of the boot
// payload so a running client can resync its tab strip without a full reload.
type httpListQuickChatSessionsResponse struct {
	Sessions     []httpQuickChatSession `json:"sessions"`
	TaskSessions []dto.TaskSessionDTO   `json:"task_sessions"`
}

// httpListQuickChatSessions returns the workspace's restorable quick-chat tabs.
// Quick chats are created and closed from any device, so clients poll this on
// (re)connect to converge on the server's list instead of drifting apart.
func (h *TaskHandlers) httpListQuickChatSessions(c *gin.Context) {
	workspaceID := c.Param("id")
	items, err := h.service.ListQuickChatSessions(c.Request.Context(), workspaceID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not found")
		return
	}
	response := httpListQuickChatSessionsResponse{
		Sessions:     make([]httpQuickChatSession, 0, len(items)),
		TaskSessions: make([]dto.TaskSessionDTO, 0, len(items)),
	}
	for _, item := range items {
		response.Sessions = append(response.Sessions, httpQuickChatSession{
			SessionID:      item.SessionID,
			TaskID:         item.TaskID,
			WorkspaceID:    item.WorkspaceID,
			Kind:           item.Kind,
			Name:           item.Name,
			AgentProfileID: item.AgentProfileID,
		})
		sessionDTO := dto.FromTaskSession(item.Session)
		dto.EnrichCancellationPending(&sessionDTO, h.cancellationPending)
		response.TaskSessions = append(response.TaskSessions, sessionDTO)
	}
	c.JSON(http.StatusOK, response)
}

// httpStartConfigChatRequest is the request body for starting a config chat session.
type httpStartConfigChatRequest struct {
	AgentProfileID string `json:"agent_profile_id,omitempty"`
	ExecutorID     string `json:"executor_id,omitempty"`
	Prompt         string `json:"prompt,omitempty"`
}

// httpStartConfigChat creates an ephemeral task with config_mode and prepares a session.
// The session will have config MCP tools (workflow, agent, MCP management) instead of
// the normal kanban/plan tools used for task-solving agents.
// resolveConfigChatDefaults resolves the agent profile ID, executor ID, and task
// metadata for a config chat session. Profile priority: request → workspace config → workspace default.
func resolveConfigChatDefaults(body httpStartConfigChatRequest, ws *models.Workspace) (agentProfileID, executorID string, metadata map[string]interface{}) {
	agentProfileID = body.AgentProfileID
	if agentProfileID == "" && ws.DefaultConfigAgentProfileID != nil {
		agentProfileID = *ws.DefaultConfigAgentProfileID
	}
	if agentProfileID == "" && ws.DefaultAgentProfileID != nil {
		agentProfileID = *ws.DefaultAgentProfileID
	}
	executorID = body.ExecutorID
	if executorID == "" && ws.DefaultExecutorID != nil {
		executorID = *ws.DefaultExecutorID
	}
	metadata = map[string]interface{}{
		"config_mode":                true,
		models.MetaKeyAgentProfileID: agentProfileID,
	}
	if executorID != "" {
		metadata[models.MetaKeyExecutorID] = executorID
	}
	return agentProfileID, executorID, metadata
}

func (h *TaskHandlers) httpStartConfigChat(c *gin.Context) {
	workspaceID := c.Param("id")
	var body httpStartConfigChatRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}

	ctx := c.Request.Context()

	workspace, err := h.service.GetWorkspace(ctx, workspaceID)
	if err != nil {
		handleNotFound(c, h.logger, err, "workspace not found")
		return
	}

	agentProfileID, executorID, metadata := resolveConfigChatDefaults(body, workspace)
	if agentProfileID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no agent profile configured — set a default agent profile in workspace settings"})
		return
	}

	result, err := h.service.CreateTask(ctx, &service.CreateTaskRequest{
		WorkspaceID: workspaceID,
		Title:       "Config Chat",
		Description: body.Prompt,
		IsEphemeral: true,
		Metadata:    metadata,
	})
	if err != nil {
		h.logger.Error("failed to create config chat task", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create config chat"})
		return
	}
	task := result.Task

	resp, err := h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:         task.ID,
		Intent:         orchestrator.IntentPrepare,
		AgentProfileID: agentProfileID,
		ExecutorID:     executorID,
		// When a prompt is present, launchConfigChatAgent below follows with a
		// prompt-bearing IntentStartCreated. Defer the start so a passthrough
		// profile isn't eagerly launched here with an empty prompt. With no
		// prompt there is no follow-up, so keep the eager upgrade that gives the
		// terminal a PTY to attach to.
		DeferredStart: body.Prompt != "",
	})
	if err != nil {
		h.deleteTaskOnError(task.ID, "config chat", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare session"})
		return
	}

	sessionID := resp.SessionID

	// If a prompt was provided, launch the agent asynchronously so it starts
	// processing immediately. The frontend receives WS updates when it starts.
	if body.Prompt != "" {
		go h.launchConfigChatAgent(task.ID, sessionID, agentProfileID, body.Prompt)
	}

	h.logger.Info("config chat session created",
		zap.String("task_id", task.ID),
		zap.String("session_id", sessionID),
		zap.String("workspace_id", workspaceID))

	c.JSON(http.StatusOK, httpStartQuickChatResponse{
		TaskID:    task.ID,
		SessionID: sessionID,
	})
}

func (h *TaskHandlers) deleteTaskOnError(taskID, label string, err error) {
	if deleteErr := h.service.DeleteTask(context.Background(), taskID); deleteErr != nil {
		h.logger.Error("failed to rollback "+label+" task",
			zap.String("task_id", taskID), zap.Error(deleteErr))
	}
	h.logger.Error("failed to prepare "+label+" session",
		zap.Error(err), zap.String("task_id", taskID))
}

func (h *TaskHandlers) launchConfigChatAgent(
	taskID, sessionID, agentProfileID, prompt string,
) {
	startCtx, cancel := context.WithTimeout(
		context.Background(), constants.AgentLaunchTimeout,
	)
	defer cancel()
	launchResp, err := h.orchestrator.LaunchSession(startCtx, &orchestrator.LaunchSessionRequest{
		TaskID:         taskID,
		Intent:         orchestrator.IntentStartCreated,
		SessionID:      sessionID,
		AgentProfileID: agentProfileID,
		Prompt:         prompt,
	})
	if err != nil {
		h.logger.Error("failed to start config chat agent",
			zap.Error(err), zap.String("task_id", taskID),
			zap.String("session_id", sessionID))
		return
	}
	h.logger.Info("config chat agent started",
		zap.String("task_id", taskID),
		zap.String("session_id", launchResp.SessionID),
		zap.String("execution_id", launchResp.AgentExecutionID))
}
