package pause

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/auth/authn"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	"github.com/kandev/kandev/internal/office/models"
	userstore "github.com/kandev/kandev/internal/user/store"
)

// Response field names repeated across the three endpoints' JSON bodies.
const (
	fieldWorkspaceID = "workspace_id"
	fieldPaused      = "paused"
	fieldPause       = "pause"
	fieldError       = "error"
)

// maxPauseRequestBodyBytes bounds the pause/resume request body via
// http.MaxBytesReader, writing the 413 response below when exceeded.
// pauseRequestBody carries only a short reason string, so this is generous
// relative to any legitimate payload — matching the established pattern at
// agents/handler.go's maxUpdateAgentBodyBytes and channels/handler.go.
const maxPauseRequestBodyBytes = 64 * 1024

// Handler exposes the pause/resume HTTP surface under the Office route group.
type Handler struct {
	svc *Service
}

// NewHandler creates a pause handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes mounts the pause/resume routes under the Office group.
// Routes carry :wsId, so officeWorkspaceScopeMiddleware authorizes them
// with no new resolver entry.
func RegisterRoutes(api *gin.RouterGroup, h *Handler) {
	api.GET("/workspaces/:wsId/pause", h.getPause)
	api.POST("/workspaces/:wsId/pause", h.postPause)
	api.POST("/workspaces/:wsId/resume", h.postResume)
}

type pauseRequestBody struct {
	Reason string `json:"reason"`
}

func pausePayload(p *models.WorkspacePause) gin.H {
	if p == nil {
		return nil
	}
	return gin.H{
		"id":              p.ID,
		"reason":          p.Reason,
		"created_by":      p.CreatedBy,
		"created_by_kind": p.CreatedByKind,
		"created_at":      p.CreatedAt,
	}
}

func (h *Handler) getPause(c *gin.Context) {
	workspaceID := c.Param("wsId")
	// PauseState alone cannot distinguish "unknown workspace" from
	// "running workspace" (both read (nil, nil)) — AC-006.9 requires the
	// same existence check the two mutations perform, checked first as it
	// is there. Routed through writeMutationError so a failed read maps to
	// 500 like the mutation endpoints, not a blanket 404.
	if err := h.svc.checkWorkspaceExists(c.Request.Context(), workspaceID); err != nil {
		h.writeMutationError(c, workspaceID, err)
		return
	}
	active, err := h.svc.PauseState(c.Request.Context(), workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{fieldError: err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		fieldWorkspaceID: workspaceID,
		fieldPaused:      active != nil,
		fieldPause:       pausePayload(active),
	})
}

func (h *Handler) postPause(c *gin.Context) {
	if officeagents.CallerFromContext(c) != nil {
		c.JSON(http.StatusForbidden, gin.H{fieldError: "agent callers may not pause a workspace"})
		return
	}
	workspaceID := c.Param("wsId")
	var body pauseRequestBody
	if !h.bindPauseRequestBody(c, &body) {
		return
	}

	result, err := h.svc.Pause(c.Request.Context(), workspaceID, body.Reason, actorID(c), actorKind(c))
	if err != nil {
		h.writeMutationError(c, workspaceID, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		fieldWorkspaceID: workspaceID,
		fieldPaused:      true,
		fieldPause:       pausePayload(result.Pause),
		"sweep":          result.Sweep,
	})
}

func (h *Handler) postResume(c *gin.Context) {
	if officeagents.CallerFromContext(c) != nil {
		c.JSON(http.StatusForbidden, gin.H{fieldError: "agent callers may not resume a workspace"})
		return
	}
	workspaceID := c.Param("wsId")
	var body pauseRequestBody
	if !h.bindPauseRequestBody(c, &body) {
		return
	}

	if _, err := h.svc.Resume(c.Request.Context(), workspaceID, body.Reason, actorID(c), actorKind(c)); err != nil {
		h.writeMutationError(c, workspaceID, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		fieldWorkspaceID: workspaceID,
		fieldPaused:      false,
		fieldPause:       nil,
	})
}

// bindPauseRequestBody decodes the request body into body, bounded by
// maxPauseRequestBodyBytes. Reports false (and has already written the
// response) only when the body exceeded the limit; any other bind error
// (malformed JSON, empty body) is ignored here exactly as before — Reason
// validation happens downstream in the service layer.
func (h *Handler) bindPauseRequestBody(c *gin.Context, body *pauseRequestBody) bool {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxPauseRequestBodyBytes)
	if err := c.ShouldBindJSON(body); err != nil {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{fieldError: "request body too large"})
			return false
		}
	}
	return true
}

// writeMutationError maps a Pause/Resume error to its HTTP status. The
// pause-contended 409 deliberately carries no "reason" field (paused:false)
// so a client can distinguish it from a blocked-caller 409, which is
// returned by the routine dispatch endpoints, not this handler.
func (h *Handler) writeMutationError(c *gin.Context, workspaceID string, err error) {
	switch {
	case errors.Is(err, ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, gin.H{fieldError: err.Error()})
	case errors.Is(err, ErrReasonRequired), errors.Is(err, ErrReasonTooLong):
		c.JSON(http.StatusBadRequest, gin.H{fieldError: err.Error()})
	case errors.Is(err, ErrPauseContended):
		c.JSON(http.StatusConflict, gin.H{
			fieldWorkspaceID: workspaceID,
			fieldError:       err.Error(),
			fieldPaused:      false,
		})
	default:
		c.JSON(http.StatusInternalServerError, gin.H{fieldError: err.Error()})
	}
}

// actorID resolves the caller's user id: the session user with
// authentication enabled, otherwise the synthetic-admin sentinel
// auth/httpmw already injects when auth is disabled.
func actorID(c *gin.Context) string {
	if identity, ok := authn.IdentityFromContext(c.Request.Context()); ok {
		return identity.UserID
	}
	return userstore.DefaultUserID
}

// actorKind is always "user": the 403 above already rejects every agent
// caller, so every request that reaches Pause/Resume was made by a human
// operator or the synthetic single-user identity, never an agent.
func actorKind(*gin.Context) string {
	return "user"
}
