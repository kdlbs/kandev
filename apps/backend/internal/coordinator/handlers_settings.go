package coordinator

import (
	"context"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// maxSettingsBodyBytes bounds a settings request body.
const maxSettingsBodyBytes = 64 << 10

// httpGetSettings backs GET /api/v1/workspaces/:id/coordinators/:cid/settings.
func (h *Handlers) httpGetSettings(c *gin.Context) {
	settings, err := h.service.GetSettings(c.Request.Context(), c.Param("id"), c.Param("cid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

// httpPutSettings backs PUT /api/v1/workspaces/:id/coordinators/:cid/settings.
// The response is the coordinator DTO carrying the saved policy and Watches.
func (h *Handlers) httpPutSettings(c *gin.Context) {
	ctx := c.Request.Context()
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxSettingsBodyBytes))
	if err != nil {
		h.respondError(c, bodyErr("", "request body is unreadable or too large"))
		return
	}
	saved, err := h.service.SaveSettings(ctx, c.Param("id"), c.Param("cid"), body)
	if err != nil {
		h.respondError(c, err)
		return
	}
	found, _, _, err := h.service.GetCoordinator(ctx, c.Param("id"), c.Param("cid"))
	if err != nil {
		h.respondError(c, err)
		return
	}
	dto := NewCoordinatorDTO(found)
	dto.CoordinatorPhase2 = saved
	c.JSON(http.StatusOK, dto)
}

// httpSetupCoordinator backs POST /api/v1/workspaces/:id/coordinators/setup.
// The body is read raw and decoded by the service after the caller is
// authorized; the 201 body is the coordinator DTO of the phase-1 create.
func (h *Handlers) httpSetupCoordinator(c *gin.Context) {
	ctx := c.Request.Context()
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxSettingsBodyBytes))
	if err != nil {
		h.respondError(c, bodyErr("", "request body is unreadable or too large"))
		return
	}
	created, err := h.service.CreateSetup(ctx, c.Param("id"), body)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusCreated, h.setupResponse(ctx, created))
}

// setupResponse renders a committed setup. The rows already exist, so a failed
// policy read degrades to the base coordinator shape instead of an error status.
func (h *Handlers) setupResponse(ctx context.Context, created *Coordinator) *CoordinatorDTO {
	dto, err := h.coordinatorDTO(ctx, created)
	if err != nil {
		h.logger.Warn("setup committed but policy read failed", zap.String("coordinator_id", created.ID), zap.Error(err))
		return NewCoordinatorDTO(created)
	}
	return dto
}
