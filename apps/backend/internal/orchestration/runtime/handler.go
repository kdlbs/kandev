package runtime

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
	"net/http"
	"strconv"
)

const (
	statusResolved       = "resolved"
	revisionResponseKey  = "revision"
	executionModeExecute = "execute"
	taskIDKey            = "task_id"
)

const (
	statusAcknowledged          = "acknowledged"
	intentRevisionKey           = "intent_revision"
	attentionKindQuestion       = "question"
	attentionKindPermission     = "permission"
	attentionKindAuthentication = "authentication"
	attentionKindFailure        = "failure"
	entriesKey                  = "entries"
	statusActive                = "active"
	authorTypeAgent             = "agent"
	executionModeDesign         = "design"
	errorResponseKey            = "error"
	statusFailed                = "failed"
	nextCursorKey               = "next_cursor"
	healthUnavailable           = "unavailable"
	statusUnknown               = "unknown"
	authorTypeUser              = "user"
	scopeWorkspace              = "workspace"
)

type Handler struct {
	Service   *Service
	Authorize func(context.Context, string) error
}

func RegisterRoutes(g *gin.RouterGroup, h *Handler) {
	assistant := g.Group("", h.requireAssistant)
	assistant.GET("/assistant", h.assistant)
	assistant.PUT("/assistant", h.selectAssistant)
	assistant.POST("/assistant/control", h.assistantControl)
	assistant.GET("/assistant/objectives", h.objectives)
	h.registerMaintenanceRoutes(assistant)
	h.registerWorkspaceGrantRoutes(assistant)
	assistant.GET("/assistant/attention", h.attention)
	assistant.GET("/assistant/attention/:id/input", h.attentionInput)
	assistant.GET("/runtime/attention/:id/input", h.attentionInput)
	assistant.POST("/assistant/attention/:id/resolve", h.resolveAttention)
	assistant.POST("/runtime/attention/:id/answer", h.resolveAttention)
	assistant.GET("/runtime/attention", h.attention)
	assistant.GET("/assistant/capabilities", h.capabilities)
	assistant.GET("/runtime/capabilities", h.capabilities)
	assistant.GET("/runtime/memory", h.runtimeMemory)
	assistant.GET("/assistant/memory", h.assistantMemory)
	assistant.GET("/assistant/memory/:id", h.assistantMemory)
	assistant.GET("/assistant/memory/:id/source", h.assistantMemorySource)
	assistant.PUT("/assistant/memory/:id", h.editAssistantMemory)
	assistant.DELETE("/assistant/memory/:id", h.forgetAssistantMemory)
	assistant.GET("/assistant/credentials", h.credential)
	assistant.GET("/assistant/credentials/:id", h.credential)
	assistant.PUT("/assistant/credentials/:id", h.editCredential)
	assistant.DELETE("/assistant/credentials/:id", h.forgetCredential)
	assistant.GET("/runtime/objectives", h.objectives)
	assistant.POST("/runtime/objectives", h.createObjective)
	assistant.PATCH("/runtime/objectives/:id", h.updateObjective)
	assistant.GET("/runtime/context/:id", h.contextPacket)
	assistant.GET("/runtime/context/:id/memory", h.contextMemory)
	g.GET("/tasks/:id", h.conversation)
	g.GET("/tasks/:id/comments", h.comments)
	g.POST("/tasks/:id/comments", h.comment)
	g.POST("/tasks/:id/retry", h.retry)
	g.GET("/runtime/workspace", h.catalog)
	assistant.GET("/runtime/tasks", h.workspaceTasks)
	g.GET("/runtime/tasks/:id/details", h.details)
	g.POST("/runtime/tasks", h.createTask)
	g.POST("/runtime/tasks/:id/manage", h.manageTask)
	g.POST("/runtime/tasks/:id/status", h.updateTask)
	g.POST("/runtime/comments", h.runtimeComment)
	g.GET("/agents/:id/memory", h.memory)
	g.GET("/agents/:id/memory/summary", h.memory)
	g.PUT("/agents/:id/memory", h.setMemory)
}
func fail(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: err.Error()})
}
func (h *Handler) caller(c *gin.Context) (*runtimeauth.AgentClaims, bool) {
	raw, ok := c.Get("agent_claims")
	claims, valid := raw.(*runtimeauth.AgentClaims)
	if !ok || !valid || (claims.Capabilities != workspaceCoordinatorAudience && claims.Capabilities != assistantBrokerAudience) {
		c.AbortWithStatusJSON(403, gin.H{errorResponseKey: "coordinator token required"})
		return nil, false
	}
	run, err := h.Service.Runs.GetRunByID(c.Request.Context(), claims.RunID)
	if err != nil || run == nil || run.AgentProfileID != claims.AgentProfileID || run.Status != statusClaimed || run.SessionID != claims.SessionID {
		c.AbortWithStatusJSON(403, gin.H{errorResponseKey: "run is no longer active"})
		return nil, false
	}
	role, err := h.Service.Repo.OrchestratorRoleID(c.Request.Context(), claims.AgentProfileID)
	if err != nil || role == "" {
		c.AbortWithStatus(403)
		return nil, false
	}
	if !h.currentRunAuthority(c, claims, run.Payload) {
		return nil, false
	}
	if !h.assistantInvocation(c, claims, run.Payload) {
		return nil, false
	}
	return h.resolveWorkspaceTarget(c, claims)
}

func (h *Handler) currentRunAuthority(c *gin.Context, claims *runtimeauth.AgentClaims, payload string) bool {
	if c.Request.Method == http.MethodGet {
		if claims.Capabilities == assistantBrokerAudience {
			return h.currentIntent(c, claims.TaskID, payload)
		}
		return h.currentBinding(c, claims.TaskID, payload)
	}
	if c.GetHeader("X-Kandev-Run-Id") != claims.RunID {
		c.AbortWithStatus(http.StatusForbidden)
		return false
	}
	return h.currentIntent(c, claims.TaskID, payload)
}
func (h *Handler) scopedConversation(c *gin.Context) (string, string, bool) {
	id := c.Param("id")
	owner, ws, err := h.Service.Repo.ConversationOwner(c.Request.Context(), id)
	if err != nil {
		c.AbortWithStatus(404)
		return "", "", false
	}
	if _, ok := c.Get("agent_claims"); ok {
		claims, valid := h.caller(c)
		if !valid {
			return "", "", false
		}
		if claims.AgentProfileID != owner || claims.WorkspaceID != ws {
			c.AbortWithStatus(403)
			return "", "", false
		}
	} else if h.Authorize != nil {
		if err := h.Authorize(c.Request.Context(), ws); err != nil {
			c.AbortWithStatus(404)
			return "", "", false
		}
	}
	if _, runtime := c.Get("agent_claims"); !runtime && !h.privateConversationAllowed(c, id) {
		c.AbortWithStatus(404)
		return "", "", false
	}
	if c.Request.Method != "GET" {
		_, _, err := h.Service.bindingSnapshot(c.Request.Context(), id)
		if !bindingCheck(c, err) {
			return "", "", false
		}
	}
	return owner, ws, true
}
func (h *Handler) conversation(c *gin.Context) {
	if _, ok := c.Get("agent_claims"); ok {
		h.runtimeTask(c)
		return
	}
	owner, _, ok := h.scopedConversation(c)
	if !ok {
		return
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, gin.H{"id": task.ID, "title": task.Title, "workspace_id": task.WorkspaceID, "orchestrator_id": owner})
}
func (h *Handler) comments(c *gin.Context) {
	ok := h.canReadTask(c)
	if !ok {
		return
	}
	limit := 500
	if _, runtime := c.Get("agent_claims"); runtime {
		limit = 20
	}
	if raw := c.Query("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > 500 {
			fail(c, fmt.Errorf("limit must be between 1 and 500"))
			return
		}
		limit = n
	}
	rows, err := h.Service.Repo.CommentsBefore(c.Request.Context(), c.Param("id"), c.Query("before"), limit+1)
	if err != nil {
		fail(c, err)
		return
	}
	nextCursor := ""
	if len(rows) > limit {
		rows = rows[:limit]
		nextCursor = rows[len(rows)-1].ID
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	statuses, err := h.Service.Runs.GetRunsByCommentIDs(c.Request.Context(), ids)
	if err != nil {
		fail(c, err)
		return
	}
	for _, row := range rows {
		if run, ok := statuses[row.ID]; ok {
			row.RunID = run.RunID
			row.RunStatus = run.Status
			row.RunError = run.ErrorMessage
		}
	}
	for i, j := 0, len(rows)-1; i < j; i, j = i+1, j-1 {
		rows[i], rows[j] = rows[j], rows[i]
	}
	c.JSON(200, gin.H{"comments": rows, nextCursorKey: nextCursor})
}
func (h *Handler) comment(c *gin.Context) {
	if _, ok := c.Get("agent_claims"); ok {
		h.runtimeComment(c)
		return
	}
	owner, _, ok := h.scopedConversation(c)
	if !ok {
		return
	}
	persona, err := h.Service.Personas.GetAgentInstance(c.Request.Context(), owner)
	if err != nil {
		fail(c, err)
		return
	}
	if paused(persona) {
		fail(c, fmt.Errorf("coordinator is paused"))
		return
	}
	h.acceptComment(c, owner)
}
func (h *Handler) catalog(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	if _, linked := c.Get(workspaceSelectionKey); linked {
		h.workspaceDirectory(c, claims)
		return
	}
	result, err := h.Service.Manager.WorkspaceCatalog(c.Request.Context(), claims.WorkspaceID)
	if err != nil {
		fail(c, err)
		return
	}
	profiles, err := h.Service.Repo.ExecutionProfileDirectory(c.Request.Context(), claims.WorkspaceID)
	if err != nil {
		fail(c, err)
		return
	}
	if data, ok := result.(map[string]any); ok {
		data["execution_profiles"] = profiles
	}
	c.JSON(200, result)
}
func (h *Handler) details(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok || !h.privateRuntimeAllowed(c, claims, c.Param("id")) {
		return
	}
	if _, linked := c.Get(workspaceSelectionKey); linked {
		h.workspaceTask(c, claims)
		return
	}
	result, err := h.Service.Manager.WorkspaceTaskDetails(c.Request.Context(), claims.WorkspaceID, c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, result)
}
func (h *Handler) createTask(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	var req struct {
		ProjectID string `json:"project_id"`
		models.OperationRequest
		models.DelegationReference
		Title          string `json:"title"`
		Description    string `json:"description"`
		WorkflowID     string `json:"workflow_id"`
		WorkflowStepID string `json:"workflow_step_id"`
		ExecutionMode  string `json:"execution_mode"`
		RepositoryID   string `json:"repository_id"`
		AssigneeID     string `json:"assignee"`
		ExternalID     string `json:"external_id"`
		ParentID       string `json:"parent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if req.ProjectID != "" {
		fail(c, fmt.Errorf("use a workspace workflow, not an Office project"))
		return
	}
	if req.ExecutionMode != "" && req.ExecutionMode != executionModeExecute && req.ExecutionMode != executionModeDesign {
		c.AbortWithStatusJSON(422, gin.H{errorResponseKey: "answer and inspect stay in the assistant conversation"})
		return
	}
	h.performOperation(c, claims, req.OperationRequest, req, http.StatusCreated, func() (any, error) {
		objective, err := h.delegationObjective(c, claims, req.ObjectiveID, req.ExecutionMode)
		if err != nil {
			return nil, rejectOperation(422, err.Error())
		}
		ref := delegationReference(objective, req.ContextRef, req.OperationID)
		if err := h.attachDelegationContext(c, claims, objective, &ref); err != nil {
			return nil, rejectOperation(422, err.Error())
		}
		req.AssigneeID, err = newTaskContextProfile(ref.Packet, req.RepositoryID, req.AssigneeID)
		if err != nil {
			return nil, err
		}
		if err := h.authorizeTaskEffect(c, claims, req.ExecutionMode); err != nil {
			return nil, err
		}
		id, err := h.Service.Manager.CreateWorkspaceTask(c.Request.Context(), models.WorkspaceTaskSpec{DelegationReference: ref, DirectProfile: true, WorkspaceID: claims.WorkspaceID, ChiefID: claims.AgentProfileID, WorkflowID: req.WorkflowID, WorkflowStepID: req.WorkflowStepID, ExecutionMode: req.ExecutionMode, RepositoryID: req.RepositoryID, AssigneeID: req.AssigneeID, Title: req.Title, Description: req.Description, ExternalID: req.ExternalID, ParentID: req.ParentID})
		if err == nil && objective != nil {
			err = h.Service.Repo.LinkObjectiveTask(c.Request.Context(), models.ObjectiveTask{ObjectiveID: objective.ID, TaskID: id, Role: "implementation", ContextRef: req.ContextRef, OperationID: req.OperationID})
		}
		return gin.H{"id": id}, err
	})
}
func (h *Handler) manageTask(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok || !h.privateRuntimeAllowed(c, claims, c.Param("id")) {
		return
	}
	var req models.WorkspaceTaskCommand
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	req.DirectProfile = true
	req.WorkspaceID = claims.WorkspaceID
	req.ChiefID = claims.AgentProfileID
	req.TaskID = c.Param("id")
	h.performOperation(c, claims, req.OperationRequest, req, http.StatusOK, func() (any, error) {
		if err := h.rejectMaintenanceTaskControl(c, req.TaskID); err != nil {
			return nil, err
		}
		objective, err := h.delegationObjective(c, claims, req.ObjectiveID, "")
		if err != nil {
			return nil, rejectOperation(422, err.Error())
		}
		req.DelegationReference = delegationReference(objective, req.ContextRef, req.OperationID)
		if req.Action != "stop" {
			if err := h.attachDelegationContext(c, claims, objective, &req.DelegationReference); err != nil {
				return nil, rejectOperation(422, err.Error())
			}
		}
		mode := ""
		if objective != nil {
			mode = objective.Mode
		}
		if err := h.authorizeTaskEffect(c, claims, mode); err != nil {
			return nil, err
		}
		err = h.Service.Manager.ManageWorkspaceTask(c.Request.Context(), req)
		if err == nil && objective != nil && req.Action != "delete" {
			err = h.Service.Repo.LinkObjectiveTask(c.Request.Context(), models.ObjectiveTask{ObjectiveID: objective.ID, TaskID: req.TaskID, SessionID: req.SessionID, Role: "implementation", ContextRef: req.ContextRef, OperationID: req.OperationID})
		}
		return gin.H{"ok": true}, err
	})
}
func (h *Handler) updateTask(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok || !h.privateRuntimeAllowed(c, claims, c.Param("id")) {
		return
	}
	var req struct {
		Status string `json:"status"`
		models.OperationRequest
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	h.performOperation(c, claims, req.OperationRequest, req, http.StatusOK, func() (any, error) {
		if err := h.rejectMaintenanceTaskControl(c, c.Param("id")); err != nil {
			return nil, err
		}
		if err := h.authorizeTaskEffect(c, claims, ""); err != nil {
			return nil, err
		}
		return gin.H{"ok": true}, h.Service.UpdateStatus(c.Request.Context(), claims.WorkspaceID, c.Param("id"), req.Status)
	})
}
func (h *Handler) runtimeComment(c *gin.Context) {
	claims, ok := h.caller(c)
	if !ok {
		return
	}
	var req struct {
		TaskID string `json:"task_id"`
		Body   string `json:"body"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if id := c.Param("id"); id != "" {
		req.TaskID = id
	}
	if req.TaskID == "" {
		req.TaskID = claims.TaskID
	}
	if claims.Capabilities == assistantBrokerAudience && req.TaskID != claims.TaskID {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	if !h.privateRuntimeAllowed(c, claims, req.TaskID) {
		return
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), req.TaskID)
	if err != nil || task.WorkspaceID != claims.WorkspaceID {
		c.AbortWithStatus(403)
		return
	}
	if len(req.Body) == 0 || len(req.Body) > 32000 {
		fail(c, fmt.Errorf("invalid comment length"))
		return
	}
	row := &models.TaskComment{TaskID: req.TaskID, AuthorID: claims.AgentProfileID, AuthorType: authorTypeAgent, Body: req.Body, Source: authorTypeAgent}
	if err := h.Service.Repo.PutComment(c.Request.Context(), row); err != nil {
		fail(c, err)
		return
	}
	c.JSON(201, row)
}

func (h *Handler) canReadTask(c *gin.Context) bool {
	if _, ok := c.Get("agent_claims"); ok {
		return h.authorizeRuntimeTask(c)
	}
	_, _, ok := h.scopedConversation(c)
	return ok
}
func (h *Handler) authorizeRuntimeTask(c *gin.Context) bool {
	claims, ok := h.caller(c)
	if !ok || !h.privateRuntimeAllowed(c, claims, c.Param("id")) {
		return false
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil || task.WorkspaceID != claims.WorkspaceID {
		c.AbortWithStatus(404)
		return false
	}
	return true
}
func (h *Handler) runtimeTask(c *gin.Context) {
	if !h.authorizeRuntimeTask(c) {
		return
	}
	task, err := h.Service.Tasks.GetTask(c.Request.Context(), c.Param("id"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(200, task)
}

func (h *Handler) registerMaintenanceRoutes(assistant *gin.RouterGroup) {
	assistant.GET("/assistant/improvements", h.improvements)
	assistant.GET("/assistant/maintenance-options", h.maintenanceOptions)
	assistant.GET("/runtime/improvements", h.improvements)
	assistant.GET("/assistant/improvements/:id", h.improvement)
	assistant.GET("/runtime/improvements/:id", h.improvement)
	assistant.GET("/assistant/improvements/:id/evidence", h.improvementEvidence)
	assistant.GET("/runtime/improvements/:id/evidence", h.improvementEvidence)
	assistant.PUT("/assistant/improvements/:id/grant", h.saveMaintenanceGrant)
	assistant.DELETE("/assistant/improvements/:id/grant", h.revokeMaintenanceGrant)
	assistant.POST("/assistant/improvements/:id/maintenance", h.maintenanceAction)
	assistant.POST("/runtime/improvements/:id/maintenance", h.maintenanceAction)
	assistant.GET("/assistant/improvements/:id/file", h.maintenanceFile)
	assistant.GET("/runtime/improvements/:id/file", h.maintenanceFile)
	assistant.GET("/assistant/improvements/:id/artifact", h.maintenanceArtifact)
	assistant.GET("/runtime/improvements/:id/artifact", h.maintenanceArtifact)
	assistant.POST("/assistant/improvements/:id/review", h.reviewImprovement)
	assistant.POST("/assistant/improvements/:id/reconcile", h.reconcileMaintenance)
	assistant.GET("/assistant/improvements/:id/successes", h.maintenanceSuccesses)
}
