package handlers

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/common/httpmw"
	"github.com/kandev/kandev/internal/notifications/models"
	"github.com/kandev/kandev/internal/notifications/service"
	"github.com/kandev/kandev/internal/notifications/store"
)

func (h *Handlers) httpFormatter(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	if c.Request.Method != http.MethodGet {
		cancel := httpmw.BoundRequestBody(c, 5*time.Second, 8192)
		defer cancel()
	}
	settings, err := h.controller.FormatterSettings(c.Request.Context(), c.Param("id"))
	if err != nil {
		writeFormatterError(c, err)
		return
	}
	if c.Request.Method == http.MethodGet {
		c.JSON(http.StatusOK, settings)
		return
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		ExpectedRevision *string                  `json:"expected_revision"`
		Binding          *models.FormatterBinding `json:"binding"`
	}
	if err := decoder.Decode(&body); err != nil || body.ExpectedRevision == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid formatter selection"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid formatter selection"})
		return
	}
	binding, err := h.controller.ActivateFormatter(c.Request.Context(), c.Param("id"), *body.ExpectedRevision, body.Binding)
	if err != nil {
		writeFormatterError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"binding": binding})
}

func writeFormatterError(c *gin.Context, err error) {
	code, message := http.StatusInternalServerError, "notification formatter unavailable"
	switch {
	case errors.Is(err, service.ErrProviderNotFound):
		code, message = http.StatusNotFound, "provider not found"
	case errors.Is(err, store.ErrFormatterConflict):
		code, message = http.StatusConflict, "notification formatter changed; reload before retrying"
	case errors.Is(err, store.ErrFormatterInvalid):
		code, message = http.StatusBadRequest, "invalid notification formatter selection"
	case errors.Is(err, service.ErrFormatterCapture):
		code, message = http.StatusServiceUnavailable, "notification configuration capture in progress"
	}
	c.JSON(code, gin.H{"error": message})
}

func (h *Handlers) httpFormatterPreview(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	cancel := httpmw.BoundRequestBody(c, 5*time.Second, 128*1024)
	defer cancel()
	if _, err := h.controller.FormatterSettings(c.Request.Context(), c.Param("id")); err != nil {
		writeFormatterError(c, err)
		return
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var body struct {
		Binding models.FormatterBinding `json:"binding"`
		Event   string                  `json:"event"`
		Sample  string                  `json:"sample"`
		Draft   json.RawMessage         `json:"draft"`
	}
	if err := decoder.Decode(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid formatter preview"})
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid formatter preview"})
		return
	}
	result, err := h.controller.PreviewFormatter(c.Request.Context(), c.Param("id"), body.Binding, body.Event, body.Sample, body.Draft)
	if err != nil {
		writeFormatterError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
