package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/service"
	"io"
	"net/http"
)

func (h *Handlers) httpImportPromptSources(c *gin.Context)  { h.importPromptSources(c, false) }
func (h *Handlers) httpPreviewPromptSources(c *gin.Context) { h.importPromptSources(c, true) }
func (h *Handlers) importPromptSources(c *gin.Context, preview bool) {
	body, tooLarge, err := readImportBody(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read prompt source body"})
		return
	}
	if tooLarge {
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "prompt source body exceeds import limit"})
		return
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	var request models.PromptSourceImport
	if err := decoder.Decode(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expected one prompt source document"})
		return
	}
	result, err := h.controller.ImportPromptSources(c.Request.Context(), request, preview)
	if err != nil {
		writePromptSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func (h *Handlers) httpDiffPromptSources(c *gin.Context) {
	result, err := h.controller.DiffPromptSources(c.Request.Context())
	if err != nil {
		writePromptSourceError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}
func writePromptSourceError(c *gin.Context, err error) {
	code := http.StatusBadRequest
	if errors.Is(err, models.ErrPromptSourceConflict) || errors.Is(err, models.ErrPromptSourceLocked) {
		code = http.StatusConflict
	}
	if errors.Is(err, models.ErrPromptSourceForbidden) {
		code = http.StatusForbidden
	}
	if errors.Is(err, service.ErrNotVisible) {
		code = http.StatusNotFound
	}
	c.JSON(code, gin.H{"error": err.Error()})
}
