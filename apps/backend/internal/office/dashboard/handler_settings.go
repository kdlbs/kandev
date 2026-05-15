package dashboard

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"

	"go.uber.org/zap"
)

// -- Git --

func (h *Handler) gitClone(c *gin.Context) {
	wsID := c.Param("wsId")
	var req GitCloneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.RepoURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repoUrl is required"})
		return
	}
	// Body-supplied workspaceName must match the URL :wsId so a caller
	// cannot POST to /workspaces/ws-A/git/clone with workspaceName=ws-B
	// and land the clone on ws-B. Empty body field falls back to wsID.
	if req.WorkspaceName != "" && req.WorkspaceName != wsID {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspaceName must match URL wsId"})
		return
	}
	if h.gitMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "git manager not initialized"})
		return
	}
	if err := h.gitMgr.CloneWorkspace(c.Request.Context(), req.RepoURL, req.Branch, wsID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) gitPull(c *gin.Context) {
	wsID := c.Param("wsId")
	if h.gitMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "git manager not initialized"})
		return
	}
	if err := h.gitMgr.PullWorkspace(c.Request.Context(), wsID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) gitPush(c *gin.Context) {
	wsID := c.Param("wsId")
	var req GitPushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Message == "" {
		req.Message = "Update workspace configuration"
	}
	if h.gitMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "git manager not initialized"})
		return
	}
	if err := h.gitMgr.PushWorkspace(c.Request.Context(), wsID, req.Message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) gitStatus(c *gin.Context) {
	wsID := c.Param("wsId")
	if h.gitMgr == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "git manager not initialized"})
		return
	}
	if !h.gitMgr.IsGitWorkspace(wsID) {
		c.JSON(http.StatusOK, GitStatusResponse{IsGit: false})
		return
	}
	status, err := h.gitMgr.GetWorkspaceGitStatus(c.Request.Context(), wsID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, GitStatusResponse{
		IsGit:       true,
		Branch:      status.Branch,
		IsDirty:     status.IsDirty,
		HasRemote:   status.HasRemote,
		Ahead:       status.Ahead,
		Behind:      status.Behind,
		CommitCount: status.CommitCount,
	})
}

// -- Meta --

func (h *Handler) getMeta(c *gin.Context) {
	c.JSON(http.StatusOK, MetaResponse{
		Statuses:           models.AllStatuses(),
		Priorities:         models.AllPriorities(),
		Roles:              models.AllRoles(),
		ExecutorTypes:      models.AllExecutorTypes(),
		SkillSourceTypes:   models.AllSkillSourceTypes(),
		ProjectStatuses:    models.AllProjectStatuses(),
		AgentStatuses:      models.AllAgentStatuses(),
		RoutineRunStatuses: models.AllRoutineRunStatuses(),
		InboxItemTypes:     models.AllInboxItemTypes(),
		Permissions:        allPermissionMeta(),
		PermissionDefaults: allPermissionDefaults(),
	})
}

func allPermissionMeta() []PermissionMeta {
	return []PermissionMeta{
		{
			Key:         shared.PermCanCreateTasks,
			Label:       "Create tasks",
			Description: "Allow this agent to create new tasks",
			Type:        "bool",
		},
		{
			Key:         shared.PermCanAssignTasks,
			Label:       "Assign tasks",
			Description: "Allow this agent to assign tasks to other agents",
			Type:        "bool",
		},
		{
			Key:         shared.PermCanCreateAgents,
			Label:       "Create agents",
			Description: "Allow this agent to create new agent instances",
			Type:        "bool",
		},
		{
			Key:         shared.PermCanApprove,
			Label:       "Approve requests",
			Description: "Allow this agent to approve or reject approval requests",
			Type:        "bool",
		},
		{
			Key:         shared.PermCanManageOwnSkills,
			Label:       "Manage own skills",
			Description: "Allow this agent to create and update its own skills",
			Type:        "bool",
		},
		{
			Key:         shared.PermMaxSubtaskDepth,
			Label:       "Max subtask depth",
			Description: "Maximum depth of subtasks this agent can create",
			Type:        "int",
		},
	}
}

func allPermissionDefaults() map[string]map[string]interface{} {
	roles := []models.AgentRole{
		models.AgentRoleCEO,
		models.AgentRoleWorker,
		models.AgentRoleSpecialist,
		models.AgentRoleAssistant,
		models.AgentRoleSecurity,
		models.AgentRoleQA,
		models.AgentRoleDevOps,
	}
	defaults := make(map[string]map[string]interface{}, len(roles))
	for _, r := range roles {
		defaults[string(r)] = shared.ResolvePermissions(shared.AgentRole(r), "")
	}
	return defaults
}

// -- Workspace Settings --

func (h *Handler) getWorkspaceSettings(c *gin.Context) {
	wsID := c.Param("wsId")
	settings, err := h.svc.GetWorkspaceSettings(c.Request.Context(), wsID, wsID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if settings == nil {
		settings = &WorkspaceSettings{Name: wsID, PermissionHandlingMode: "human"}
	}
	c.JSON(http.StatusOK, WorkspaceSettingsResponse{Settings: settings})
}

func (h *Handler) updateWorkspaceSettings(c *gin.Context) {
	wsID := c.Param("wsId")
	var req UpdateWorkspaceSettingsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.PermissionHandlingMode != nil {
		mode := *req.PermissionHandlingMode
		if !isValidPermissionHandlingMode(mode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid permission_handling_mode: must be 'human' or 'auto_approve'"})
			return
		}
		if err := h.svc.UpdatePermissionHandlingMode(wsID, mode); err != nil {
			h.logger.Warn("failed to update permission_handling_mode",
				zap.String("workspace", wsID),
				zap.Error(err))
		}
	}

	if req.RecoveryLookbackHours != nil {
		hours := *req.RecoveryLookbackHours
		if hours < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "recovery_lookback_hours must be non-negative"})
			return
		}
		if err := h.svc.UpdateRecoveryLookbackHours(wsID, hours); err != nil {
			h.logger.Warn("failed to update recovery_lookback_hours",
				zap.String("workspace", wsID),
				zap.Error(err))
		}
	}

	h.svc.UpdateGovernanceApprovalFlags(c.Request.Context(), wsID,
		req.RequireApprovalForNewAgents,
		req.RequireApprovalForTaskCompletion,
		req.RequireApprovalForSkillChanges,
	)
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

// isValidPermissionHandlingMode returns true for the two supported modes.
func isValidPermissionHandlingMode(mode string) bool {
	return mode == "human" || mode == "auto_approve"
}
