package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/service"
)

// deferredLaunchDiscardResponse is the body of
// POST /api/v1/tasks/:id/deferred-launch/discard.
//
// This route, its verb, and these fields are a candidate contract proposed to
// upstream, not an agreed one; the durable safety rules live in the shared
// settlement service, so a different public shape does not weaken them. A
// client must branch on outcome and never infer success from the 200 alone. A
// retained outcome means the durable record remains; after clear_incomplete,
// its automation run may already be terminal and the next sweep can finish the
// cleanup without replaying it.
type deferredLaunchDiscardResponse struct {
	Outcome models.DeferredLaunchDiscardOutcome `json:"outcome"`
	Reason  models.DeferredLaunchDiscardReason  `json:"reason,omitempty"`
}

// httpDiscardDeferredLaunch serves
// POST /api/v1/tasks/:id/deferred-launch/discard: an explicit, task-scoped
// request to remove a pending deferred launch. It starts and stops no session.
func (h *TaskHandlers) httpDiscardDeferredLaunch(c *gin.Context) {
	result, err := h.service.DiscardDeferredLaunch(c.Request.Context(), c.Param("id"))
	if err != nil {
		if errors.Is(err, service.ErrDeferredLaunchDiscardUnavailable) {
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
			return
		}
		handleNotFound(c, h.logger, err, "task not found")
		return
	}
	c.JSON(http.StatusOK, deferredLaunchDiscardResponse{
		Outcome: result.Outcome,
		Reason:  result.Reason,
	})
}
