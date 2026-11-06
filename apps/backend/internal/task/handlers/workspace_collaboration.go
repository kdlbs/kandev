package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
	ws "github.com/kandev/kandev/pkg/websocket"
)

// RegisterWorkspaceCollaborationRoutes is called only for the enabled feature.
// The service repeats the enabled and human/native authorization checks.
func RegisterWorkspaceCollaborationRoutes(router *gin.Engine, dispatcher *ws.Dispatcher, svc *service.Service) {
	h := &workspaceCollaborationHandlers{service: svc}
	api := router.Group("/api/v1/tasks/:id")
	api.GET("/workspace-collaboration", h.list)
	api.PUT("/workspace-collaboration/:workspaceId", h.put)
	api.POST("/sessions/:sessionId/collaboration-report", h.report)
	dispatcher.RegisterFunc("workspace.collaboration.report", h.reportWS)
}

type workspaceCollaborationHandlers struct{ service *service.Service }

type workspaceCollaborationFlags struct {
	Read    bool `json:"read"`
	Report  bool `json:"report"`
	Prompt  bool `json:"prompt"`
	Control bool `json:"control"`
}

func (f *workspaceCollaborationFlags) UnmarshalJSON(data []byte) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(data, &fields); err != nil {
		return err
	}
	if fields == nil {
		return errors.New("collaboration grant must be an object")
	}
	for field, raw := range fields {
		var value *bool
		if err := json.Unmarshal(raw, &value); err != nil || value == nil {
			return fmt.Errorf("%s must be a boolean", field)
		}
		switch field {
		case "read":
			f.Read = *value
		case "report":
			f.Report = *value
		case "prompt":
			f.Prompt = *value
		case "control":
			f.Control = *value
		default:
			return fmt.Errorf("unknown collaboration field %q", field)
		}
	}
	return nil
}

func (h *workspaceCollaborationHandlers) list(c *gin.Context) {
	grants, err := h.service.ListWorkspaceCollaborationGrants(c.Request.Context(), c.Param("id"))
	if err != nil {
		collaborationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"grants": grants})
}

func (h *workspaceCollaborationHandlers) put(c *gin.Context) {
	var flags workspaceCollaborationFlags
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&flags); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid collaboration grant"})
		return
	}
	grant, err := h.service.PutWorkspaceCollaborationGrant(c.Request.Context(), c.Param("id"), c.Param("workspaceId"), models.WorkspaceCollaborationGrant{
		Read: flags.Read, Report: flags.Report, Prompt: flags.Prompt, Control: flags.Control,
	})
	if err != nil {
		collaborationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, grant)
}

func (h *workspaceCollaborationHandlers) report(c *gin.Context) {
	var body struct {
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report"})
		return
	}
	message, err := h.service.CreateWorkspaceCollaborationReport(c.Request.Context(), c.Param("id"), c.Param("sessionId"), body.Content)
	if err != nil {
		collaborationHTTPError(c, err)
		return
	}
	c.JSON(http.StatusOK, message)
}

func (h *workspaceCollaborationHandlers) reportWS(ctx context.Context, msg *ws.Message) (*ws.Message, error) {
	var body struct {
		TaskID    string `json:"task_id"`
		SessionID string `json:"session_id"`
		Content   string `json:"content"`
	}
	if err := msg.ParsePayload(&body); err != nil {
		return ws.NewError(msg.ID, msg.Action, ws.ErrorCodeBadRequest, "invalid report", nil)
	}
	message, err := h.service.CreateWorkspaceCollaborationReport(ctx, body.TaskID, body.SessionID, body.Content)
	if err != nil {
		code := ws.ErrorCodeInternalError
		if service.IsForbidden(err) {
			code = ws.ErrorCodeForbidden
		}
		return ws.NewError(msg.ID, msg.Action, code, err.Error(), nil)
	}
	return ws.NewResponse(msg.ID, msg.Action, message)
}

func collaborationHTTPError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrWorkspaceCollaborationDisabled):
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace_collaboration_disabled"})
	case errors.Is(err, service.ErrWorkspaceCollaborationUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "workspace_collaboration_unavailable"})
	case errors.Is(err, service.ErrWorkspaceCollaborationReportInvalid):
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case service.IsForbidden(err):
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
	case isNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{"error": "workspace collaboration request failed"})
	}
}
