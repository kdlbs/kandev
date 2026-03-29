package handlers

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/common/constants"
	"github.com/kandev/kandev/internal/orchestrator"
	"github.com/kandev/kandev/internal/task/dto"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"go.uber.org/zap"
)

func (h *TaskHandlers) httpListTasks(c *gin.Context) {
	tasks, err := h.service.ListTasks(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "tasks not found")
		return
	}
	taskDTOs := make([]dto.TaskDTO, 0, len(tasks))
	for _, task := range tasks {
		taskDTOs = append(taskDTOs, dto.FromTask(task))
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
	includeArchived := c.Query("include_archived") == queryValueTrue
	includeEphemeral := c.Query("include_ephemeral") == queryValueTrue
	onlyEphemeral := c.Query("only_ephemeral") == queryValueTrue
	excludeConfig := c.Query("exclude_config") == queryValueTrue

	tasks, total, err := h.service.ListTasksByWorkspace(
		c.Request.Context(), c.Param("id"), query, page, pageSize, includeArchived, includeEphemeral, onlyEphemeral, excludeConfig,
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

// buildTaskDTOsWithSessionInfo converts tasks to DTOs enriched with primary session IDs,
// session counts, and review status using bulk queries.
func buildTaskDTOsWithSessionInfo(ctx context.Context, svc *service.Service, tasks []*models.Task) ([]dto.TaskDTO, error) {
	if len(tasks) == 0 {
		return []dto.TaskDTO{}, nil
	}
	taskIDs := make([]string, len(tasks))
	for i, t := range tasks {
		taskIDs[i] = t.ID
	}
	primarySessionMap, err := svc.GetPrimarySessionIDsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	sessionCountMap, err := svc.GetSessionCountsForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
	primarySessionInfoMap, err := svc.GetPrimarySessionInfoForTasks(ctx, taskIDs)
	if err != nil {
		return nil, err
	}
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
		si := extractSessionInfo(primarySessionInfoMap[task.ID])
		result = append(result, dto.FromTaskWithSessionInfo(
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
		))
	}
	return result, nil
}

type sessionInfoFields struct {
	reviewStatus     *string
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

func (h *TaskHandlers) toTaskDTOsWithSessionInfo(ctx context.Context, tasks []*models.Task) ([]dto.TaskDTO, error) {
	return buildTaskDTOsWithSessionInfo(ctx, h.service, tasks)
}

func (h *TaskHandlers) httpGetTask(c *gin.Context) {
	task, err := h.service.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	dtos, err := buildTaskDTOsWithSessionInfo(c.Request.Context(), h.service, []*models.Task{task})
	if err != nil {
		h.logger.Error("failed to build task DTO with session info", zap.Error(err))
		c.JSON(http.StatusOK, dto.FromTask(task))
		return
	}
	c.JSON(http.StatusOK, dtos[0])
}

func (h *TaskHandlers) httpListTaskSessions(c *gin.Context) {
	sessions, err := h.service.ListTaskSessions(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task sessions not found")
		return
	}
	sessionDTOs := make([]dto.TaskSessionSummaryDTO, 0, len(sessions))
	for _, session := range sessions {
		sessionDTOs = append(sessionDTOs, dto.FromTaskSessionSummary(session))
	}
	c.JSON(http.StatusOK, dto.ListTaskSessionSummariesResponse{
		Sessions: sessionDTOs,
		Total:    len(sessionDTOs),
	})
}

func (h *TaskHandlers) httpGetTaskSession(c *gin.Context) {
	session, err := h.service.GetTaskSession(c.Request.Context(), c.Param("id"))
	if err != nil {
		handleNotFound(c, h.logger, err, "task session not found")
		return
	}
	c.JSON(http.StatusOK, dto.GetTaskSessionResponse{
		Session: dto.FromTaskSession(session),
	})
}

func (h *TaskHandlers) httpListSessionTurns(c *gin.Context) {
	sessionID := c.Param("id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session id is required"})
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
		h.logger.Error("failed to approve session", zap.String("session_id", c.Param("id")), zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	resp := dto.ApproveSessionResponse{
		Success: true,
		Session: dto.FromTaskSession(result.Session),
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
	SourceWorkflowID string `json:"source_workflow_id"`
	SourceStepID     string `json:"source_step_id,omitempty"`
	TargetWorkflowID string `json:"target_workflow_id"`
	TargetStepID     string `json:"target_step_id"`
}

func (h *TaskHandlers) httpBulkMoveTasks(c *gin.Context) {
	var body httpBulkMoveTasksRequest
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid payload"})
		return
	}
	if body.SourceWorkflowID == "" || body.TargetWorkflowID == "" || body.TargetStepID == "" {
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

type httpTaskRepositoryInput struct {
	RepositoryID   string `json:"repository_id"`
	BaseBranch     string `json:"base_branch"`
	CheckoutBranch string `json:"checkout_branch"`
	LocalPath      string `json:"local_path"`
	Name           string `json:"name"`
	DefaultBranch  string `json:"default_branch"`
	GitHubURL      string `json:"github_url"`
}

type httpCreateTaskRequest struct {
	WorkspaceID       string                    `json:"workspace_id"`
	WorkflowID        string                    `json:"workflow_id"`
	WorkflowStepID    string                    `json:"workflow_step_id"`
	Title             string                    `json:"title"`
	Description       string                    `json:"description,omitempty"`
	Priority          int                       `json:"priority,omitempty"`
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
}

type createTaskResponse struct {
	dto.TaskDTO
	TaskSessionID    string `json:"session_id,omitempty"`
	AgentExecutionID string `json:"agent_execution_id,omitempty"`
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

func validateAttachments(items []v1.MessageAttachment) error {
	if len(items) > maxCreateTaskAttachments {
		return fmt.Errorf("too many attachments (max %d)", maxCreateTaskAttachments)
	}
	var totalSize int
	for i, a := range items {
		typ := strings.TrimSpace(a.Type)
		if _, ok := allowedAttachmentTypes[typ]; !ok {
			return fmt.Errorf("attachment[%d] has unsupported type %q", i, typ)
		}
		if strings.TrimSpace(a.MimeType) == "" {
			return fmt.Errorf("attachment[%d] mime_type is required", i)
		}
		if len(a.Data) == 0 {
			return fmt.Errorf("attachment[%d] data is required", i)
		}
		if len(a.Data) > maxAttachmentDataBytes {
			return fmt.Errorf("attachment[%d] data exceeds size limit", i)
		}
		totalSize += len(a.Data)
	}
	if totalSize > maxAttachmentDataBytes {
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

	// When not starting the agent immediately, persist profile IDs in task metadata
	// so handleTaskMovedNoSession can retrieve them when the task is later dragged
	// to a step with auto_start_agent.
	if !body.StartAgent && !body.PrepareSession && body.AgentProfileID != "" {
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

	task, err := h.service.CreateTask(c.Request.Context(), &service.CreateTaskRequest{
		WorkspaceID:    body.WorkspaceID,
		WorkflowID:     body.WorkflowID,
		WorkflowStepID: body.WorkflowStepID,
		Title:          title,
		Description:    description,
		Priority:       body.Priority,
		State:          body.State,
		Repositories:   convertToServiceRepos(repos),
		Position:       body.Position,
		Metadata:       body.Metadata,
		PlanMode:       body.PlanMode && !body.StartAgent,
		ParentID:       body.ParentID,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not created")
		return
	}

	taskDTO := dto.FromTask(task)
	response := createTaskResponse{TaskDTO: taskDTO}
	// Use the backend-resolved workflow step ID (from the created task) instead of the request's
	resolvedStepID := taskDTO.WorkflowStepID
	h.handlePostCreateTaskSession(c, &response, taskDTO.ID, taskDTO.Description, body, resolvedStepID)

	c.JSON(http.StatusOK, response)
}

// convertCreateTaskRepositories converts httpTaskRepositoryInput slice to dto.TaskRepositoryInput slice.
// Returns (nil, false) and writes a 400 response if any entry is missing both repository_id and local_path.
func convertCreateTaskRepositories(c *gin.Context, inputs []httpTaskRepositoryInput) ([]dto.TaskRepositoryInput, bool) {
	var repos []dto.TaskRepositoryInput
	for _, r := range inputs {
		if r.RepositoryID == "" && r.LocalPath == "" && r.GitHubURL == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "repository_id, local_path, or github_url is required"})
			return nil, false
		}
		repos = append(repos, dto.TaskRepositoryInput{
			RepositoryID:   r.RepositoryID,
			BaseBranch:     r.BaseBranch,
			CheckoutBranch: r.CheckoutBranch,
			LocalPath:      r.LocalPath,
			Name:           r.Name,
			DefaultBranch:  r.DefaultBranch,
			GitHubURL:      r.GitHubURL,
		})
	}
	return repos, true
}

// handlePostCreateTaskSession prepares or starts an agent session after a task is created,
// depending on the PrepareSession and StartAgent flags in the request body.
func (h *TaskHandlers) handlePostCreateTaskSession(
	c *gin.Context,
	response *createTaskResponse,
	taskID, description string,
	body httpCreateTaskRequest,
	resolvedStepID string,
) {
	if h.orchestrator == nil || body.AgentProfileID == "" {
		return
	}
	if body.PrepareSession && !body.StartAgent {
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
	} else if body.StartAgent {
		h.startAgentForNewTask(c.Request.Context(), response, taskID, description, body, resolvedStepID)
	}
}

// startAgentForNewTask prepares a session and launches the agent asynchronously for a
// newly created task when start_agent is requested. It populates response.TaskSessionID
// on success.
func (h *TaskHandlers) startAgentForNewTask(
	ctx context.Context,
	response *createTaskResponse,
	taskID, description string,
	body httpCreateTaskRequest,
	resolvedStepID string,
) {
	// Create session entry synchronously so we can return the session ID immediately.
	// Skip workspace launch — the start intent will handle it in the background goroutine.
	// This prevents blocking for 30-60s on remote executors (sprites, remote_docker).
	prepResp, err := h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:            taskID,
		Intent:            orchestrator.IntentPrepare,
		AgentProfileID:    body.AgentProfileID,
		ExecutorID:        body.ExecutorID,
		ExecutorProfileID: body.ExecutorProfileID,
		WorkflowStepID:    resolvedStepID,
	})
	if err != nil {
		h.logger.Error("failed to prepare session for task", zap.Error(err), zap.String("task_id", taskID))
		return
	}
	sessionID := prepResp.SessionID
	response.TaskSessionID = sessionID

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
	Priority     *int                      `json:"priority,omitempty"`
	State        *v1.TaskState             `json:"state,omitempty"`
	Repositories []httpTaskRepositoryInput `json:"repositories,omitempty"`
	Position     *int                      `json:"position,omitempty"`
	Metadata     map[string]interface{}    `json:"metadata,omitempty"`
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
				RepositoryID:  r.RepositoryID,
				BaseBranch:    r.BaseBranch,
				LocalPath:     r.LocalPath,
				Name:          r.Name,
				DefaultBranch: r.DefaultBranch,
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
		Repositories: convertToServiceRepos(repos),
		Position:     body.Position,
		Metadata:     body.Metadata,
	})
	if err != nil {
		handleNotFound(c, h.logger, err, "task not updated")
		return
	}
	c.JSON(http.StatusOK, dto.FromTask(task))
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
	result, err := h.service.MoveTask(
		c.Request.Context(), c.Param("id"),
		body.WorkflowID, body.WorkflowStepID, body.Position,
	)
	if err != nil {
		handleNotFound(c, h.logger, err, "task not moved")
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
	deleteCtx, cancel := context.WithTimeout(context.Background(), constants.TaskDeleteTimeout)
	defer cancel()
	if err := h.service.DeleteTask(deleteCtx, c.Param("id")); err != nil {
		handleNotFound(c, h.logger, err, "task not deleted")
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
}

func (h *TaskHandlers) httpArchiveTask(c *gin.Context) {
	if err := h.service.ArchiveTask(c.Request.Context(), c.Param("id")); err != nil {
		handleNotFound(c, h.logger, err, "task not archived")
		return
	}
	c.JSON(http.StatusOK, dto.SuccessResponse{Success: true})
}

// httpStartQuickChatRequest is the request body for starting a quick chat session.
type httpStartQuickChatRequest struct {
	Title             string `json:"title,omitempty"`
	RepositoryID      string `json:"repository_id,omitempty"`
	AgentProfileID    string `json:"agent_profile_id,omitempty"`
	ExecutorID        string `json:"executor_id,omitempty"`
	Prompt            string `json:"prompt,omitempty"`
	LocalPath         string `json:"local_path,omitempty"`
	RepositoryName    string `json:"repository_name,omitempty"`
	DefaultBranch     string `json:"default_branch,omitempty"`
	BaseBranch        string `json:"base_branch,omitempty"`
	LaunchImmediately bool   `json:"launch_immediately,omitempty"`
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
	executorID := body.ExecutorID
	if executorID == "" && workspace.DefaultExecutorID != nil {
		executorID = *workspace.DefaultExecutorID
	}

	metadata := make(map[string]interface{})
	if agentProfileID != "" {
		metadata["agent_profile_id"] = agentProfileID
	}
	if executorID != "" {
		metadata["executor_id"] = executorID
	}

	title := body.Title
	if title == "" {
		title = "Quick Chat"
	}

	return quickChatParams{
		agentProfileID: agentProfileID,
		executorID:     executorID,
		title:          title,
		repos:          body.buildRepositories(),
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

	task, err := h.service.CreateTask(ctx, &service.CreateTaskRequest{
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

	resp, err := h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:          task.ID,
		Intent:          orchestrator.IntentPrepare,
		AgentProfileID:  params.agentProfileID,
		ExecutorID:      params.executorID,
		LaunchWorkspace: true,
	})
	if err != nil {
		// Rollback: delete the ephemeral task to prevent orphans
		if deleteErr := h.service.DeleteTask(ctx, task.ID); deleteErr != nil {
			h.logger.Error("failed to rollback quick chat task",
				zap.String("task_id", task.ID),
				zap.Error(deleteErr))
		}
		h.logger.Error("failed to prepare quick chat session", zap.Error(err), zap.String("task_id", task.ID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to prepare session"})
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
		"config_mode":      true,
		"agent_profile_id": agentProfileID,
	}
	if executorID != "" {
		metadata["executor_id"] = executorID
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

	task, err := h.service.CreateTask(ctx, &service.CreateTaskRequest{
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

	resp, err := h.orchestrator.LaunchSession(ctx, &orchestrator.LaunchSessionRequest{
		TaskID:         task.ID,
		Intent:         orchestrator.IntentPrepare,
		AgentProfileID: agentProfileID,
		ExecutorID:     executorID,
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
