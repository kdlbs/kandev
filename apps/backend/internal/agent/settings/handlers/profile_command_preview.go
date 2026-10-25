package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/agent/settings/controller"
)

func (h *Handlers) httpPreviewSavedProfileCommand(c *gin.Context) {
	preview, err := h.controller.PreviewSavedProfileCommand(c.Request.Context(), c.Param("id"))
	if err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, controller.ErrProfileCommandPreviewMissing) {
			status = http.StatusNotFound
		} else if errors.Is(err, controller.ErrInvalidCommandPrefix) {
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{errorResponseKey: "profile command preview unavailable"})
		return
	}
	c.JSON(http.StatusOK, preview)
}
