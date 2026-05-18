package agents

import (
	"errors"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
)

const (
	ctxKeyAgentClaims = "agent_claims"
	ctxKeyAgentCaller = "agent_caller"
)

// Handler provides HTTP handlers for agent routes.
type Handler struct {
	svc    *AgentService
	logger *logger.Logger
}

// RegisterRoutes registers agent and instruction HTTP routes on the given router group.
func RegisterRoutes(group *gin.RouterGroup, svc *AgentService, log *logger.Logger) {
	h := &Handler{svc: svc, logger: log.WithFields(zap.String("component", "office-agents-handler"))}

	group.GET("/workspaces/:wsId/agents", h.listAgents)
	group.POST("/workspaces/:wsId/agents", h.createAgent)
	group.GET("/agents/:id", h.getAgent)
	group.PATCH("/agents/:id", h.updateAgent)
	group.PATCH("/agents/:id/status", h.updateAgentStatus)
	group.DELETE("/agents/:id", h.deleteAgent)

	group.GET("/agents/:id/instructions", h.listInstructions)
	group.GET("/agents/:id/instructions/:filename", h.getInstruction)
	group.PUT("/agents/:id/instructions/:filename", h.upsertInstruction)
	group.DELETE("/agents/:id/instructions/:filename", h.deleteInstruction)

	group.GET("/agents/:id/utilization", h.getAgentUtilization)

	group.GET("/agents/:id/memory", h.listMemory)
	group.PUT("/agents/:id/memory", h.upsertMemory)
	group.DELETE("/agents/:id/memory/all", h.deleteAllMemory)
	group.DELETE("/agents/:id/memory/:entryId", h.deleteMemory)
	group.GET("/agents/:id/memory/summary", h.memorySummary)
	group.GET("/agents/:id/memory/export", h.exportMemory)
}

// AgentAuthMiddleware returns a gin.HandlerFunc that extracts and validates agent JWTs.
// Requests without a JWT are treated as UI/admin requests and pass through.
func AgentAuthMiddleware(svc *AgentService) gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
			c.Next()
			return
		}
		token := strings.TrimPrefix(auth, "Bearer ")
		claims, err := svc.ValidateAgentJWT(token)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			return
		}
		agent, err := svc.GetAgentInstance(c.Request.Context(), claims.AgentProfileID)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "agent not found"})
			return
		}
		if wsID := c.Param("wsId"); wsID != "" && claims.WorkspaceID != wsID {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "token workspace mismatch"})
			return
		}
		c.Set(ctxKeyAgentClaims, claims)
		c.Set(ctxKeyAgentCaller, agent)
		c.Next()
	}
}

// agentCallerFromCtx returns the authenticated agent or nil for UI requests.
func agentCallerFromCtx(c *gin.Context) *models.AgentInstance {
	val, ok := c.Get(ctxKeyAgentCaller)
	if !ok {
		return nil
	}
	agent, _ := val.(*models.AgentInstance)
	return agent
}

// CallerFromContext exposes the authenticated agent (or nil for UI
// requests) to other office packages that need to enforce agent-scoped
// authorization without depending on the agents package's internal
// context-key constants.
func CallerFromContext(c *gin.Context) *models.AgentInstance {
	return agentCallerFromCtx(c)
}

// -- Agent handlers --

func (h *Handler) listAgents(c *gin.Context) {
	filter := AgentListFilter{
		Role:      c.Query("role"),
		Status:    c.Query("status"),
		ReportsTo: c.Query("reports_to"),
	}
	var (
		agents []*models.AgentInstance
		err    error
	)
	if filter.Role != "" || filter.Status != "" || filter.ReportsTo != "" {
		agents, err = h.svc.ListAgentInstancesFiltered(c.Request.Context(), c.Param("wsId"), filter)
	} else {
		agents, err = h.svc.ListAgentsFromConfig(c.Request.Context(), c.Param("wsId"))
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AgentListResponse{Agents: agents})
}

func (h *Handler) createAgent(c *gin.Context) {
	if err := checkAgentPermission(c, shared.PermCanCreateAgents); err != nil {
		return
	}
	var req CreateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := checkNoEscalation(c, req.Permissions); err != nil {
		return
	}
	// The office agent gets a fresh row in agent_profiles. The repo's
	// CreateAgentInstance derives a default agent_id (CLI tool) from the
	// workspace's existing agents when AgentID is empty.
	agent := &models.AgentInstance{
		WorkspaceID:           c.Param("wsId"),
		Name:                  req.Name,
		Role:                  models.AgentRole(req.Role),
		Icon:                  req.Icon,
		Status:                models.AgentStatusIdle,
		ReportsTo:             req.ReportsTo,
		Permissions:           req.Permissions,
		BudgetMonthlyCents:    req.BudgetMonthlyCents,
		MaxConcurrentSessions: req.MaxConcurrentSessions,
		DesiredSkills:         req.DesiredSkills,
		ExecutorPreference:    req.ExecutorPreference,
	}
	if err := h.svc.CreateAgentInstanceWithCaller(c.Request.Context(), agent, agentCallerFromCtx(c), req.Reason); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, AgentResponse{Agent: agent})
}

func (h *Handler) getAgent(c *gin.Context) {
	agent, err := h.svc.GetAgentFromConfig(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AgentResponse{Agent: agent})
}

func (h *Handler) updateAgent(c *gin.Context) {
	ctx := c.Request.Context()
	targetID := c.Param("id")
	if caller := agentCallerFromCtx(c); caller != nil {
		if caller.ID != targetID && !isAdminRole(caller.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only CEO or admin can update other agents"})
			return
		}
	}
	agent, err := h.svc.GetAgentFromConfig(ctx, targetID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	var req UpdateAgentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Permissions != nil {
		if err := checkNoEscalation(c, *req.Permissions); err != nil {
			return
		}
	}
	if req.Routing != nil {
		if err := h.applyRoutingOverride(c, agent, *req.Routing); err != nil {
			return
		}
	}
	applyAgentUpdates(agent, &req)
	if err := h.svc.UpdateAgentInstance(ctx, agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AgentResponse{Agent: agent})
}

// applyRoutingOverride validates the override blob and writes it onto
// the agent's Settings JSON. Writes a structured 400 with per-field
// details on validation failure (and returns the error so the caller
// skips the persistence step).
//
// In addition to the catalogue-only checks (known/dup/tier shape), the
// agent's chosen tier override is checked against the workspace routing
// config: rejecting a tier that no provider has mapped at save time
// prevents the silent "every launch immediately blocks" failure mode.
func (h *Handler) applyRoutingOverride(
	c *gin.Context, agent *models.AgentInstance, ov routing.AgentOverrides,
) error {
	known := h.svc.KnownProviders()
	cfg, err := h.svc.GetWorkspaceRouting(c.Request.Context(), agent.WorkspaceID)
	if err != nil {
		h.logger.Warn("workspace routing lookup failed; skipping cross-check",
			zap.String("workspace_id", agent.WorkspaceID), zap.Error(err))
		cfg = nil
	}
	if vErr := routing.ValidateAgentOverridesAgainstWorkspace(ov, known, cfg); vErr != nil {
		respondRoutingValidation(c, vErr)
		return vErr
	}
	merged, err := routing.WriteAgentOverrides(agent.Settings, ov)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return err
	}
	agent.Settings = merged
	return nil
}

// respondRoutingValidation translates a ValidationError into the same
// structured 400 shape the dashboard routing handler uses.
func respondRoutingValidation(c *gin.Context, err error) {
	var ve *routing.ValidationError
	if errors.As(err, &ve) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   ve.Message,
			"field":   ve.Field,
			"details": ve.Details,
		})
		return
	}
	c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
}

func (h *Handler) deleteAgent(c *gin.Context) {
	if caller := agentCallerFromCtx(c); caller != nil {
		if !isAdminRole(caller.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only CEO or admin can delete agents"})
			return
		}
	}
	if err := h.svc.DeleteAgentInstance(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) updateAgentStatus(c *gin.Context) {
	if caller := agentCallerFromCtx(c); caller != nil {
		targetID := c.Param("id")
		if targetID != caller.ID && !isAdminRole(caller.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only CEO or admin can change other agents' status"})
			return
		}
	}
	var req UpdateAgentStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agent, err := h.svc.UpdateAgentStatus(
		c.Request.Context(), c.Param("id"),
		models.AgentStatus(req.Status), req.PauseReason)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, AgentResponse{Agent: agent})
}

// -- Instruction handlers --

// isValidFilename checks that a filename does not contain path separators or traversals.
func isValidFilename(s string) bool {
	return s != "" && !strings.Contains(s, "/") && !strings.Contains(s, "\\") && !strings.Contains(s, "..")
}

func (h *Handler) listInstructions(c *gin.Context) {
	files, err := h.svc.ListInstructions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, InstructionListResponse{Files: files})
}

func (h *Handler) getInstruction(c *gin.Context) {
	filename := c.Param("filename")
	if !isValidFilename(filename) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}
	file, err := h.svc.GetInstruction(c.Request.Context(), c.Param("id"), filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, InstructionFileResponse{File: file})
}

func (h *Handler) upsertInstruction(c *gin.Context) {
	if caller := agentCallerFromCtx(c); caller != nil {
		if caller.ID != c.Param("id") && !isAdminRole(caller.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only CEO or admin can modify another agent's instructions"})
			return
		}
	}
	var req UpsertInstructionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agentID := c.Param("id")
	filename := c.Param("filename")
	if !isValidFilename(filename) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}
	isEntry := filename == "AGENTS.md"
	if err := h.svc.UpsertInstruction(c.Request.Context(), agentID, filename, req.Content, isEntry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	file, err := h.svc.GetInstruction(c.Request.Context(), agentID, filename)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, InstructionFileResponse{File: file})
}

func (h *Handler) deleteInstruction(c *gin.Context) {
	if caller := agentCallerFromCtx(c); caller != nil {
		if caller.ID != c.Param("id") && !isAdminRole(caller.Role) {
			c.JSON(http.StatusForbidden, gin.H{"error": "only CEO or admin can modify another agent's instructions"})
			return
		}
	}
	filename := c.Param("filename")
	if !isValidFilename(filename) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid filename"})
		return
	}
	if err := h.svc.DeleteInstruction(c.Request.Context(), c.Param("id"), filename); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// -- Utilization handler --

func (h *Handler) getAgentUtilization(c *gin.Context) {
	usage, err := h.svc.GetAgentUtilization(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if usage == nil {
		c.JSON(http.StatusOK, gin.H{"utilization": nil})
		return
	}
	c.JSON(http.StatusOK, gin.H{"utilization": usage})
}

// -- Memory handlers --

// checkAgentMemoryAccess returns an error and writes 403 when an agent
// caller attempts to access another agent's memory. UI callers (no agent
// JWT) pass through unchanged. The CEO/admin role is allowed cross-agent
// memory access to mirror the existing instruction-handler convention.
func checkAgentMemoryAccess(c *gin.Context) error {
	caller := agentCallerFromCtx(c)
	if caller == nil {
		return nil
	}
	if caller.ID == c.Param("id") || isAdminRole(caller.Role) {
		return nil
	}
	c.JSON(http.StatusForbidden, gin.H{"error": "agents may only access their own memory"})
	return shared.ErrForbidden
}

func (h *Handler) listMemory(c *gin.Context) {
	if err := checkAgentMemoryAccess(c); err != nil {
		return
	}
	layer := c.Query("layer")
	entries, err := h.svc.ListMemory(c.Request.Context(), c.Param("id"), layer)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MemoryListResponse{Memory: entries})
}

func (h *Handler) upsertMemory(c *gin.Context) {
	if err := checkAgentMemoryAccess(c); err != nil {
		return
	}
	var req UpsertMemoryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	agentID := c.Param("id")
	for _, entry := range req.Entries {
		mem := &models.AgentMemory{
			AgentProfileID: agentID,
			Layer:          entry.Layer,
			Key:            entry.Key,
			Content:        entry.Content,
			Metadata:       entry.Metadata,
		}
		if err := h.svc.UpsertAgentMemory(c.Request.Context(), mem); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) deleteAllMemory(c *gin.Context) {
	if err := checkAgentMemoryAccess(c); err != nil {
		return
	}
	if err := h.svc.DeleteAllMemory(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) deleteMemory(c *gin.Context) {
	agentID := c.Param("id")
	entryID := c.Param("entryId")
	if err := h.svc.DeleteAgentMemoryOwned(c.Request.Context(), agentID, entryID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "memory entry not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) memorySummary(c *gin.Context) {
	if err := checkAgentMemoryAccess(c); err != nil {
		return
	}
	entries, err := h.svc.GetMemorySummary(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": len(entries), "memory": entries})
}

func (h *Handler) exportMemory(c *gin.Context) {
	if err := checkAgentMemoryAccess(c); err != nil {
		return
	}
	entries, err := h.svc.ExportMemory(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, MemoryListResponse{Memory: entries})
}

// -- Permission helpers --

// checkAgentPermission returns nil and does nothing for UI requests.
// For agent callers, it checks the specified permission and returns 403 if denied.
func checkAgentPermission(c *gin.Context, permKey string) error {
	caller := agentCallerFromCtx(c)
	if caller == nil {
		return nil
	}
	perms := shared.ResolvePermissions(shared.AgentRole(caller.Role), caller.Permissions)
	if !shared.HasPermission(perms, permKey) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden: missing " + permKey})
		return shared.ErrForbidden
	}
	return nil
}

// checkNoEscalation verifies an agent caller is not granting permissions it
// does not itself hold. No-op for UI requests or empty permission strings.
func checkNoEscalation(c *gin.Context, permsJSON string) error {
	caller := agentCallerFromCtx(c)
	if caller == nil || permsJSON == "" || permsJSON == "{}" {
		return nil
	}
	callerPerms := shared.ResolvePermissions(shared.AgentRole(caller.Role), caller.Permissions)
	if err := shared.ValidateNoEscalation(callerPerms, permsJSON); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return err
	}
	return nil
}

// isAdminRole returns true for roles that have administrative privileges.
func isAdminRole(role models.AgentRole) bool {
	return role == models.AgentRoleCEO
}

func applyAgentUpdates(agent *models.AgentInstance, req *UpdateAgentRequest) {
	if req.Name != nil {
		agent.Name = *req.Name
	}
	// Wave G: AgentInstance.ID is the agent_profiles row id; the legacy
	// AgentProfileID request field is now a no-op for updates (the row's id
	// cannot change after creation). Accepting the field keeps existing
	// clients valid; the value is intentionally ignored.
	if req.Role != nil {
		agent.Role = models.AgentRole(*req.Role)
	}
	if req.Icon != nil {
		agent.Icon = *req.Icon
	}
	if req.Status != nil {
		agent.Status = models.AgentStatus(*req.Status)
	}
	if req.ReportsTo != nil {
		agent.ReportsTo = *req.ReportsTo
	}
	if req.Permissions != nil {
		agent.Permissions = *req.Permissions
	}
	if req.BudgetMonthlyCents != nil {
		agent.BudgetMonthlyCents = *req.BudgetMonthlyCents
	}
	if req.MaxConcurrentSessions != nil {
		agent.MaxConcurrentSessions = *req.MaxConcurrentSessions
	}
	if req.DesiredSkills != nil {
		agent.DesiredSkills = *req.DesiredSkills
	}
	if req.SkillIDs != nil {
		agent.SkillIDs = *req.SkillIDs
	}
	if req.ExecutorPreference != nil {
		agent.ExecutorPreference = *req.ExecutorPreference
	}
	if req.PauseReason != nil {
		agent.PauseReason = *req.PauseReason
	}
}
