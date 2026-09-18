package runtime

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
	"github.com/kandev/kandev/internal/orchestration/models"
)

const workspaceSelectionKey = "assistant_workspace_selection"

type workspaceSelection struct {
	Binding   *models.AssistantBinding
	Grant     *models.WorkspaceGrant
	Operation string
	Export    string
}

// The signed credential remains bound to the home conversation. Only this
// request-local copy carries an explicitly authorized linked target.
func (h *Handler) resolveWorkspaceTarget(c *gin.Context, claims *runtimeauth.AgentClaims) (*runtimeauth.AgentClaims, bool) {
	workspace := c.Query(workspaceIDKey)
	if workspace == "" || workspace == claims.WorkspaceID {
		if c.Query("workspace_grant_revision") != "" {
			c.AbortWithStatus(400)
			return nil, false
		}
		return claims, true
	}
	operation, export := workspaceRouteScope(c)
	revision, err := strconv.ParseInt(c.Query("workspace_grant_revision"), 10, 64)
	if claims.Capabilities != assistantBrokerAudience || operation == "" || err != nil || revision < 1 {
		c.AbortWithStatus(403)
		return nil, false
	}
	b, err := h.Service.Repo.AssistantForConversation(c.Request.Context(), claims.TaskID)
	if err != nil || b.WorkspaceID != claims.WorkspaceID || b.OrchestratorID != claims.AgentProfileID {
		c.AbortWithStatus(403)
		return nil, false
	}
	g, err := h.Service.currentWorkspaceGrant(c.Request.Context(), b, workspace, revision, operation, export)
	if err != nil {
		c.AbortWithStatusJSON(403, gin.H{errorResponseKey: "workspace_grant_unavailable_or_superseded"})
		return nil, false
	}
	c.Set(workspaceSelectionKey, workspaceSelection{Binding: b, Grant: g, Operation: operation, Export: export})
	c.Request = c.Request.WithContext(h.Service.workspaceEffectContext(c.Request.Context(), b, g, operation, export))
	selected := *claims
	selected.WorkspaceID = workspace
	return &selected, true
}

func workspaceRouteScope(c *gin.Context) (string, string) {
	path := strings.TrimPrefix(c.FullPath(), "/api/v1/orchestration")
	if c.Request.Method == "GET" {
		switch path {
		case "/runtime/workspace":
			return workspaceObserve, "directory"
		case runtimeTasksPath, "/runtime/tasks/:id/details":
			return workspaceObserve, workspaceTaskSummaryExport
		case runtimeObjectivesPath:
			return workspaceObserve, workspaceTaskSummaryExport
		case "/runtime/attention":
			return workspaceObserve, workspaceTaskSummaryExport
		case "/runtime/attention/:id/input":
			return workspaceObserve, "task_input"
		case "/runtime/context/:id", "/runtime/context/:id/memory":
			return workspaceCoordinate, "handoff"
		}
	} else {
		switch path {
		case runtimeObjectivesPath, "/runtime/objectives/:id":
			return workspaceObserve, workspaceTaskSummaryExport
		case runtimeTasksPath, "/runtime/tasks/:id/manage", "/runtime/tasks/:id/status":
			return workspaceCoordinate, "handoff"
		case "/runtime/attention/:id/answer":
			return workspaceCoordinate, "task_input"
		}
	}
	return "", ""
}

func (s *Service) workspaceEffectContext(ctx context.Context, b *models.AssistantBinding, g *models.WorkspaceGrant, operation, export string) context.Context {
	return models.WithWorkspaceEffectGuard(ctx, func(ctx context.Context) error {
		if _, err := s.currentWorkspaceGrant(ctx, b, g.WorkspaceID, g.Revision, operation, export); err != nil {
			return rejectOperation(403, "workspace_grant_superseded")
		}
		return nil
	})
}

func (s *Service) recordSelectedExport(ctx context.Context, selection workspaceSelection, kind string) error {
	g, err := s.currentWorkspaceGrant(ctx, selection.Binding, selection.Grant.WorkspaceID, selection.Grant.Revision, selection.Operation, kind)
	if err != nil {
		return err
	}
	return s.Repo.RecordWorkspaceExport(ctx, selection.Binding, g, kind)
}

func (h *Handler) workspaceResponse(c *gin.Context, result any, exports ...string) {
	raw, err := json.Marshal(result)
	if err != nil || len(raw) > 64*1024 {
		c.AbortWithStatus(422)
		return
	}
	if _, ok := h.caller(c); !ok {
		return
	}
	if value, linked := c.Get(workspaceSelectionKey); linked {
		selection := value.(workspaceSelection)
		for _, kind := range exports {
			if err = h.Service.recordSelectedExport(c.Request.Context(), selection, kind); err != nil {
				c.AbortWithStatusJSON(403, gin.H{errorResponseKey: "workspace_export_denied"})
				return
			}
		}
	}
	c.Data(200, "application/json", raw)
}

type WorkspaceReader interface {
	AssistantWorkspaceDirectory(context.Context, string) ([]models.WorkspaceDirectoryEntry, error)
	AssistantWorkspaceTask(context.Context, string, string, bool) (models.WorkspaceTaskView, error)
	AssistantWorkspaceTasks(context.Context, string, int, int) ([]models.WorkspaceTaskSummary, bool, error)
}

func (h *Handler) workspaceReader(c *gin.Context) (WorkspaceReader, bool) {
	reader, ok := h.Service.Manager.(WorkspaceReader)
	if !ok {
		c.AbortWithStatusJSON(503, gin.H{errorResponseKey: "workspace reader unavailable"})
	}
	return reader, ok
}

func (h *Handler) checkSelectedWorkspace(c *gin.Context, record bool) error {
	value, linked := c.Get(workspaceSelectionKey)
	if !linked {
		return nil
	}
	selection := value.(workspaceSelection)
	_, err := h.Service.currentWorkspaceGrant(c.Request.Context(), selection.Binding, selection.Grant.WorkspaceID, selection.Grant.Revision, selection.Operation, selection.Export)
	if err == nil && record {
		err = h.Service.recordSelectedExport(c.Request.Context(), selection, selection.Export)
	}
	if err != nil {
		return rejectOperation(403, "workspace_grant_unavailable_or_superseded")
	}
	return nil
}
