package handlers

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/orchestrate/dto"
)

func (h *Handlers) gitClone(c *gin.Context) {
	var req dto.GitCloneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.RepoURL == "" || req.WorkspaceName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "repoUrl and workspaceName are required"})
		return
	}

	gm := h.ctrl.Svc.GitManager()
	if gm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "git manager not initialized"})
		return
	}

	if err := gm.CloneWorkspace(c.Request.Context(), req.RepoURL, req.Branch, req.WorkspaceName); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) gitPull(c *gin.Context) {
	wsID := c.Param("wsId")
	gm := h.ctrl.Svc.GitManager()
	if gm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "git manager not initialized"})
		return
	}

	if err := gm.PullWorkspace(c.Request.Context(), wsID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) gitPush(c *gin.Context) {
	wsID := c.Param("wsId")
	var req dto.GitPushRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Message == "" {
		req.Message = "Update workspace configuration"
	}

	gm := h.ctrl.Svc.GitManager()
	if gm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "git manager not initialized"})
		return
	}

	if err := gm.PushWorkspace(c.Request.Context(), wsID, req.Message); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func (h *Handlers) gitStatus(c *gin.Context) {
	wsID := c.Param("wsId")
	gm := h.ctrl.Svc.GitManager()
	if gm == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "git manager not initialized"})
		return
	}

	if !gm.IsGitWorkspace(wsID) {
		c.JSON(http.StatusOK, dto.GitStatusResponse{IsGit: false})
		return
	}

	status, err := gm.GetWorkspaceGitStatus(c.Request.Context(), wsID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, dto.GitStatusResponse{
		IsGit:       true,
		Branch:      status.Branch,
		IsDirty:     status.IsDirty,
		HasRemote:   status.HasRemote,
		Ahead:       status.Ahead,
		Behind:      status.Behind,
		CommitCount: status.CommitCount,
	})
}
