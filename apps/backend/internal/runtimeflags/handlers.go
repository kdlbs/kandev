package runtimeflags

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	svc *Service
}

func RegisterRoutes(router gin.IRouter, svc *Service) {
	h := &Handler{svc: svc}
	api := router.Group("/api/v1/runtime-flags")
	api.GET("", h.list)
	api.PATCH("/:key", h.patch)
}

func (h *Handler) list(c *gin.Context) {
	states, err := h.svc.ListStates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read runtime flags"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"flags": states})
}

type patchRequest struct {
	Override *bool `json:"override"`
}

func (h *Handler) patch(c *gin.Context) {
	var req patchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if !errors.Is(err, io.EOF) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid runtime flag payload"})
			return
		}
	}
	states, err := h.svc.SetOverride(c.Request.Context(), c.Param("key"), req.Override)
	if err != nil {
		status := http.StatusInternalServerError
		msg := "failed to update runtime flag"
		if errors.Is(err, ErrEnvLocked) {
			status = http.StatusConflict
			msg = "runtime flag is controlled by environment"
		} else if errors.Is(err, ErrUnknownFlag) {
			status = http.StatusNotFound
			msg = "runtime flag not found"
		}
		c.JSON(status, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusOK, gin.H{"flags": states})
}
