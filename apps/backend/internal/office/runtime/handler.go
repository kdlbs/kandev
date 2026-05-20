package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
)

// Handler exposes run-scoped runtime actions to agent processes.
type Handler struct {
	agentSvc    *agents.AgentService
	actions     *Actions
	skillLister SkillLister
	runEvents   RunEventAppender
}

// RunEventAppender records runtime behavior against a run.
type RunEventAppender interface {
	AppendRunEvent(ctx context.Context, runID, eventType, level string, payload map[string]interface{})
}

// NewHandler creates a runtime handler.
func NewHandler(
	agentSvc *agents.AgentService,
	actions *Actions,
	skillLister SkillLister,
	runEvents RunEventAppender,
) *Handler {
	return &Handler{
		agentSvc:    agentSvc,
		actions:     actions,
		skillLister: skillLister,
		runEvents:   runEvents,
	}
}

// RegisterRoutes mounts runtime syscall routes.
func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	group.POST("/runtime/comments", h.postComment)
	group.POST("/runtime/tasks/:id/status", h.updateTaskStatus)
	group.POST("/runtime/tasks/:id/subtasks", h.createSubtask)
	group.POST("/runtime/agents", h.createAgent)
	group.PATCH("/runtime/agents/:id", h.modifyAgent)
	group.POST("/runtime/agents/:id/runs", h.spawnAgentRun)
	group.POST("/runtime/approvals", h.requestApproval)
	group.GET("/runtime/memory/*path", h.getMemory)
	group.PUT("/runtime/memory/*path", h.putMemory)
	group.GET("/runtime/skills", h.listSkills)
	group.DELETE("/runtime/skills/:id", h.deleteSkill)
}

func (h *Handler) postComment(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	var req struct {
		TaskID string `json:"task_id"`
		Body   string `json:"body"`
	}
	if !bindJSON(c, &req) {
		return
	}
	taskID := firstNonEmpty(req.TaskID, runCtx.TaskID)
	if err := h.actions.PostComment(c.Request.Context(), runCtx, taskID, req.Body); err != nil {
		h.respondRuntimeError(c, runCtx, "post_comment", "task", taskID, err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "post_comment", "task", taskID)
	c.JSON(http.StatusCreated, gin.H{"ok": true})
}

func (h *Handler) updateTaskStatus(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	var req struct {
		Status  string `json:"status"`
		Comment string `json:"comment"`
	}
	if !bindJSON(c, &req) {
		return
	}
	if err := h.actions.UpdateTaskStatus(
		c.Request.Context(),
		runCtx,
		c.Param("id"),
		req.Status,
		req.Comment,
	); err != nil {
		h.respondRuntimeError(c, runCtx, "update_task_status", "task", c.Param("id"), err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "update_task_status", "task", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) createSubtask(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	var req CreateSubtaskInput
	if !bindJSON(c, &req) {
		return
	}
	taskID, err := h.actions.CreateSubtask(c.Request.Context(), runCtx, req)
	if err != nil {
		h.respondRuntimeError(c, runCtx, "create_subtask", "task", runCtx.TaskID, err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "create_subtask", "task", taskID)
	c.JSON(http.StatusCreated, gin.H{"task_id": taskID})
}

func (h *Handler) createAgent(c *gin.Context) {
	runCtx, caller, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	var req CreateAgentInput
	if !bindJSON(c, &req) {
		return
	}
	agent, err := h.actions.CreateAgent(c.Request.Context(), runCtx, caller, req)
	if err != nil {
		h.respondRuntimeError(c, runCtx, "create_agent", "agent", runCtx.AgentID, err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "create_agent", "agent", agent.ID)
	c.JSON(http.StatusCreated, gin.H{"agent": agent})
}

func (h *Handler) modifyAgent(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	var req ModifyAgentInput
	if !bindJSON(c, &req) {
		return
	}
	agent, err := h.actions.ModifyAgent(c.Request.Context(), runCtx, c.Param("id"), req)
	if err != nil {
		h.respondRuntimeError(c, runCtx, "modify_agents", "agent", c.Param("id"), err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "modify_agents", "agent", agent.ID)
	c.JSON(http.StatusOK, gin.H{"agent": agent})
}

func (h *Handler) spawnAgentRun(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	var req SpawnAgentRunInput
	if !bindJSON(c, &req) {
		return
	}
	req.AgentID = firstNonEmpty(req.AgentID, c.Param("id"))
	if err := h.actions.SpawnAgentRun(c.Request.Context(), runCtx, req); err != nil {
		h.respondRuntimeError(c, runCtx, "spawn_agent_run", "agent", req.AgentID, err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "spawn_agent_run", "agent", req.AgentID)
	c.JSON(http.StatusAccepted, gin.H{"ok": true})
}

func (h *Handler) requestApproval(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	var req RequestApprovalInput
	if !bindJSON(c, &req) {
		return
	}
	approval, err := h.actions.RequestApproval(c.Request.Context(), runCtx, req)
	if err != nil {
		h.respondRuntimeError(c, runCtx, "request_approval", "approval", req.TargetID, err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "request_approval", "approval", approval.ID)
	c.JSON(http.StatusCreated, gin.H{"approval": approval})
}

func (h *Handler) getMemory(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	ns, err := ParseMemoryNamespace(c.Param("path"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !CanAccessMemory(runCtx, ns, false) {
		h.respondRuntimeError(c, runCtx, "read_memory", "memory", c.Param("path"), ErrCapabilityDenied)
		return
	}
	layer, key := memoryLayerAndKey(ns)
	mem, err := h.agentSvc.GetMemory(c.Request.Context(), runCtx.AgentID, layer, key)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "memory not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"memory": mem})
}

func (h *Handler) putMemory(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	ns, err := ParseMemoryNamespace(c.Param("path"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if !CanAccessMemory(runCtx, ns, true) {
		h.respondRuntimeError(c, runCtx, "write_memory", "memory", c.Param("path"), ErrCapabilityDenied)
		return
	}
	var req struct {
		Content string `json:"content"`
	}
	if !bindJSON(c, &req) {
		return
	}
	layer, key := memoryLayerAndKey(ns)
	if err := h.agentSvc.UpsertAgentMemory(c.Request.Context(), &models.AgentMemory{
		AgentProfileID: runCtx.AgentID,
		Layer:          layer,
		Key:            key,
		Content:        req.Content,
	}); err != nil {
		h.respondRuntimeError(c, runCtx, "write_memory", "memory", c.Param("path"), err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "write_memory", "memory", c.Param("path"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) listSkills(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	if !runCtx.Capabilities.Allows(CapabilityListSkills) {
		h.respondRuntimeError(c, runCtx, "list_skills", "skills", runCtx.WorkspaceID, ErrCapabilityDenied)
		return
	}
	if h.skillLister == nil {
		h.respondRuntimeError(c, runCtx, "list_skills", "skills", runCtx.WorkspaceID, ErrRuntimeDependencyMissing)
		return
	}
	skills, err := h.skillLister.ListSkillsFromConfig(c.Request.Context(), runCtx.WorkspaceID)
	if err != nil {
		h.respondRuntimeError(c, runCtx, "list_skills", "skills", runCtx.WorkspaceID, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"skills": skills})
}

func (h *Handler) deleteSkill(c *gin.Context) {
	runCtx, _, ok := h.contextFromRequest(c)
	if !ok {
		return
	}
	if err := h.actions.DeleteSkill(c.Request.Context(), runCtx, c.Param("id")); err != nil {
		h.respondRuntimeError(c, runCtx, "delete_skills", "skill", c.Param("id"), err)
		return
	}
	h.appendActionRunEvent(c.Request.Context(), runCtx, "delete_skills", "skill", c.Param("id"))
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func memoryLayerAndKey(ns MemoryNamespace) (string, string) {
	parts := strings.SplitN(ns.Key, "/", 2)
	if ns.Kind == MemoryKindAgent {
		if len(parts) == 2 {
			return parts[0], parts[1]
		}
		return "knowledge", ns.Key
	}
	layer := ns.Kind + ":" + ns.ID
	return layer, ns.Key
}

func (h *Handler) contextFromRequest(c *gin.Context) (RunContext, *models.AgentInstance, bool) {
	token := bearerToken(c.GetHeader("Authorization"))
	if token == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "missing runtime token"})
		return RunContext{}, nil, false
	}
	claims, err := h.agentSvc.ValidateAgentJWT(token)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid runtime token"})
		return RunContext{}, nil, false
	}
	agent, err := h.agentSvc.GetAgentInstance(c.Request.Context(), claims.AgentProfileID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "agent not found"})
		return RunContext{}, nil, false
	}
	caps := FromAgent(agent)
	if claims.Capabilities != "" {
		_ = json.Unmarshal([]byte(claims.Capabilities), &caps)
	}
	runCtx := RunContext{
		WorkspaceID:  claims.WorkspaceID,
		AgentID:      claims.AgentProfileID,
		TaskID:       claims.TaskID,
		RunID:        claims.RunID,
		SessionID:    claims.SessionID,
		Capabilities: caps,
	}
	return runCtx, agent, true
}

func bindJSON(c *gin.Context, target any) bool {
	if err := c.ShouldBindJSON(target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return false
	}
	return true
}

func bearerToken(header string) string {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return ""
	}
	return strings.TrimPrefix(header, prefix)
}

func (h *Handler) respondRuntimeError(
	c *gin.Context,
	runCtx RunContext,
	action string,
	targetType string,
	targetID string,
	err error,
) {
	if errors.Is(err, shared.ErrForbidden) {
		h.appendDeniedRunEvent(c.Request.Context(), runCtx, action, targetType, targetID, err)
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

func (h *Handler) appendDeniedRunEvent(
	ctx context.Context,
	runCtx RunContext,
	action string,
	targetType string,
	targetID string,
	err error,
) {
	if h.runEvents == nil || runCtx.RunID == "" {
		return
	}
	h.runEvents.AppendRunEvent(ctx, runCtx.RunID, "runtime.denied", "warn", map[string]interface{}{
		"action":      action,
		"target_type": targetType,
		"target_id":   targetID,
		"agent_id":    runCtx.AgentID,
		"session_id":  runCtx.SessionID,
		"error":       err.Error(),
	})
}

func (h *Handler) appendActionRunEvent(
	ctx context.Context,
	runCtx RunContext,
	action string,
	targetType string,
	targetID string,
) {
	if h.runEvents == nil || runCtx.RunID == "" {
		return
	}
	h.runEvents.AppendRunEvent(ctx, runCtx.RunID, "runtime.action", "info", map[string]interface{}{
		"action":      action,
		"target_type": targetType,
		"target_id":   targetID,
		"agent_id":    runCtx.AgentID,
		"session_id":  runCtx.SessionID,
	})
}
