// Package orchestration exposes workspace coordinators on core task execution.
package orchestration

import (
	"context"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/orchestration/models"
	"github.com/kandev/kandev/internal/orchestration/personas"
	"github.com/kandev/kandev/internal/orchestration/repository/sqlite"
	"net/http"
)

const (
	errorResponseKey = "error"
)

type Handler struct {
	Registry         *sqlite.Repository
	Repo             *sqlite.Repository
	Agents           *personas.Service
	Authorize        func(context.Context, string) error
	RoleWrite        gin.HandlerFunc
	ValidateExecutor func(context.Context, string) error
}

func RegisterRoutes(group *gin.RouterGroup, h *Handler) {
	group.Use(func(c *gin.Context) {
		if _, present := c.Get("agent_caller"); present {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{errorResponseKey: "orchestrator configuration requires a user"})
			return
		}
		if ws := c.Param("wsId"); ws != "" && h.Authorize != nil {
			if err := h.Authorize(c.Request.Context(), ws); err != nil {
				c.AbortWithStatusJSON(http.StatusNotFound, gin.H{errorResponseKey: "workspace not found"})
				return
			}
		}
		c.Next()
	})
	group.GET("/roles", h.listRoles)
	group.POST("/roles", h.roleWrite, h.saveRole)
	group.PUT("/roles/:roleId", h.roleWrite, h.saveRole)
	group.DELETE("/roles/:roleId", h.roleWrite, h.deleteRole)
	group.GET("/workspaces/:wsId/profiles", h.profiles)
	group.GET("/workspaces/:wsId/orchestrators", h.list)
	group.POST("/workspaces/:wsId/orchestrators", h.create)
	group.GET("/workspaces/:wsId/orchestrators/:id", h.get)
	group.GET("/workspaces/:wsId/orchestrators/:id/tasks", h.tasks)
	group.POST("/workspaces/:wsId/import/:id", h.importAgent)
	group.PUT("/workspaces/:wsId/orchestrators/:id", h.update)
	group.DELETE("/workspaces/:wsId/orchestrators/:id", h.remove)
	group.POST("/workspaces/:wsId/orchestrators/:id/conversation", h.conversation)
	group.POST("/workspaces/:wsId/orchestrators/:id/status", h.status)
}
func fail(c *gin.Context, err error) {
	c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: err.Error()})
}
func (h *Handler) listRoles(c *gin.Context) {
	rows, err := h.Registry.ListOrchestratorRoles(c.Request.Context())
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": rows})
}
func (h *Handler) saveRole(c *gin.Context) {
	var row models.OrchestratorRole
	if err := c.ShouldBindJSON(&row); err != nil {
		fail(c, err)
		return
	}
	row.ID = c.Param("roleId")
	if row.ID == "" {
		row.ID = uuid.NewString()
	}
	if err := h.Registry.SaveOrchestratorRole(c.Request.Context(), &row); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}
func (h *Handler) deleteRole(c *gin.Context) {
	if err := h.Registry.DeleteOrchestratorRole(c.Request.Context(), c.Param("roleId")); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
func (h *Handler) scoped(c *gin.Context) *models.AgentInstance {
	id := c.Param("id")
	if err := h.Repo.AuthorizePersona(c.Request.Context(), id); err != nil {
		c.AbortWithStatus(http.StatusNotFound)
		return nil
	}
	role, err := h.Registry.OrchestratorRoleID(c.Request.Context(), id)
	if err != nil || role == "" {
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: "orchestrator not found"})
		return nil
	}
	a, err := h.Agents.GetAgentInstance(c.Request.Context(), id)
	if err != nil || a.WorkspaceID != c.Param("wsId") {
		c.JSON(http.StatusNotFound, gin.H{errorResponseKey: "orchestrator not found"})
		return nil
	}
	return a
}
func (h *Handler) list(c *gin.Context) {
	ids, err := h.Registry.ListOrchestratorIDs(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		fail(c, err)
		return
	}
	rows := []any{}
	for _, id := range ids {
		if h.Repo.AuthorizePersona(c.Request.Context(), id) != nil {
			continue
		}
		a, e := h.Agents.GetAgentInstance(c.Request.Context(), id)
		if e != nil {
			fail(c, e)
			return
		}
		row, e := h.describe(c.Request.Context(), a)
		if e != nil {
			fail(c, e)
			return
		}
		rows = append(rows, row)
	}
	c.JSON(http.StatusOK, gin.H{"orchestrators": rows})
}
func (h *Handler) get(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	row, err := h.describe(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, row)
}
func (h *Handler) remove(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	if err := h.Agents.DeleteAgentInstance(c.Request.Context(), a.ID); err != nil {
		fail(c, err)
		return
	}
	if err := h.Registry.UnregisterOrchestrator(c.Request.Context(), a.ID); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}
func (h *Handler) conversation(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	row, err := h.Repo.EnsureAgentConversation(c.Request.Context(), a)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"task_id": row.TaskID})
}
func (h *Handler) status(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	var req struct {
		Status models.AgentStatus `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		fail(c, err)
		return
	}
	if req.Status != models.AgentStatusPaused && req.Status != models.AgentStatusIdle {
		c.JSON(http.StatusBadRequest, gin.H{errorResponseKey: "select paused or idle"})
		return
	}
	if _, err := h.Agents.UpdateAgentStatus(c.Request.Context(), a.ID, req.Status, ""); err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handler) profiles(c *gin.Context) {
	rows, err := h.Registry.ExecutionProfileDirectory(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"profiles": rows})
}

func (h *Handler) roleWrite(c *gin.Context) {
	if h.RoleWrite != nil {
		h.RoleWrite(c)
	} else {
		c.Next()
	}
}

func (h *Handler) tasks(c *gin.Context) {
	a := h.scoped(c)
	if a == nil {
		return
	}
	tasks, err := h.Registry.OrchestratedTasks(c.Request.Context(), a.WorkspaceID, a.ID)
	if err != nil {
		fail(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}
