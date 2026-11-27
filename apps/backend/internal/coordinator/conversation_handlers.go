package coordinator

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/task/service"
	"go.uber.org/zap"
)

// conversationHandlers provides the conversation route's HTTP handler,
// registered separately from Handlers/RegisterRoutes because it is added by a
// later work package (task 03) than the CRUD/proposals-read/stalls-read
// routes (Build decision 14).
type conversationHandlers struct {
	service *Service
	logger  *logger.Logger
}

// RegisterConversationRoute registers
// POST /api/v1/workspaces/:id/coordinators/:cid/conversation
// (copilot.md#conversation-task). Called only when features.coordinator is
// enabled, from backendapp's registerCoordinatorConversation.
func RegisterConversationRoute(router *gin.Engine, svc *Service, log *logger.Logger) {
	h := &conversationHandlers{service: svc, logger: log.WithFields(zap.String("component", "coordinator-conversation-handlers"))}
	router.POST("/api/v1/workspaces/:id/coordinators/:cid/conversation", h.httpOpenConversation)
}

// httpOpenConversation backs
// POST /api/v1/workspaces/:id/coordinators/:cid/conversation.
func (h *conversationHandlers) httpOpenConversation(c *gin.Context) {
	ctx := c.Request.Context()
	result, err := h.service.OpenConversation(ctx, c.Param("id"), c.Param("cid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, &ConversationResponse{
		TaskID:       result.TaskID,
		SessionID:    result.SessionID,
		ArchiveState: false,
	})
}

// respondError maps an OpenConversation error to its response shape: a
// profile-unavailable or conversation-conflict error gets its own 409 body;
// otherwise it falls back to the CRUD routes' shared mapping (respondError on
// *Handlers), reusing the same *FieldError/ErrNotFound/ErrForbidden cases.
func (h *conversationHandlers) respondError(c *gin.Context, err error) {
	var profileErr *ProfileUnavailableError
	switch {
	case errors.As(err, &profileErr):
		c.JSON(http.StatusConflict, NewCoordinatorProfileUnavailableResponse(profileErr.AgentStatus, profileErr.ExecutorStatus))
	case errors.Is(err, ErrConversationConflict):
		c.JSON(http.StatusConflict, NewConversationConflictResponse())
	case errors.Is(err, ErrConversationSessionUnavailable):
		c.JSON(http.StatusBadGateway, NewErrorResponse("conversation session unavailable"))
	case errors.Is(err, ErrNotFound), errors.Is(err, repoerrors.ErrWorkspaceNotFound):
		c.JSON(http.StatusNotFound, NewErrorResponse("not found"))
	case errors.Is(err, service.ErrForbidden):
		c.JSON(http.StatusForbidden, NewErrorResponse("forbidden"))
	default:
		h.logger.Error("coordinator conversation route failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, NewErrorResponse("internal error"))
	}
}
