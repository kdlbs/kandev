package lsp

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

type HTTPService interface {
	Snapshot(ctx context.Context, taskID string) (*TaskSnapshot, error)
	SetPolicy(
		ctx context.Context,
		taskID, language string,
		policy Policy,
		origin Origin,
	) (*LanguageSnapshot, error)
	Start(ctx context.Context, taskID, language string, origin Origin) (*LanguageSnapshot, error)
	Stop(ctx context.Context, taskID, language string, origin Origin) (*LanguageSnapshot, error)
	Restart(ctx context.Context, taskID, language string, origin Origin) (*LanguageSnapshot, error)
}

type HTTPHandler struct {
	service HTTPService
}

func RegisterRoutes(router *gin.Engine, service HTTPService) {
	handler := &HTTPHandler{service: service}
	// Keep the wildcard name aligned with the existing task routes. Gin treats
	// different names at the same path position as a registration conflict.
	router.GET("/api/v1/tasks/:id/lsp", handler.snapshot)
	router.PUT("/api/v1/tasks/:id/lsp/:language/policy", handler.setPolicy)
	router.POST("/api/v1/tasks/:id/lsp/:language/start", handler.start)
	router.POST("/api/v1/tasks/:id/lsp/:language/stop", handler.stop)
	router.POST("/api/v1/tasks/:id/lsp/:language/restart", handler.restart)
}

func (h *HTTPHandler) snapshot(c *gin.Context) {
	result, err := h.service.Snapshot(c.Request.Context(), c.Param("id"))
	writeHTTPResult(c, result, err)
}

type policyBody struct {
	Policy Policy `json:"policy"`
}

func (h *HTTPHandler) setPolicy(c *gin.Context) {
	var body policyBody
	if err := decodeStrictBody(c, &body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{string(PhaseError): err.Error()})
		return
	}
	result, err := h.service.SetPolicy(
		c.Request.Context(), c.Param("id"), c.Param("language"), body.Policy,
		Origin{Initiator: InitiatorUser, Reason: "user_set_policy"},
	)
	writeHTTPResult(c, result, err)
}

func (h *HTTPHandler) start(c *gin.Context) {
	h.control(c, ActionStart)
}

func (h *HTTPHandler) stop(c *gin.Context) {
	h.control(c, ActionStop)
}

func (h *HTTPHandler) restart(c *gin.Context) {
	h.control(c, ActionRestart)
}

func (h *HTTPHandler) control(c *gin.Context, action Action) {
	if err := requireEmptyBody(c); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{string(PhaseError): err.Error()})
		return
	}
	origin := Origin{Initiator: InitiatorUser, Reason: "user_" + string(action)}
	var (
		result *LanguageSnapshot
		err    error
	)
	switch action {
	case ActionStart:
		result, err = h.service.Start(c.Request.Context(), c.Param("id"), c.Param("language"), origin)
	case ActionStop:
		result, err = h.service.Stop(c.Request.Context(), c.Param("id"), c.Param("language"), origin)
	case ActionRestart:
		result, err = h.service.Restart(c.Request.Context(), c.Param("id"), c.Param("language"), origin)
	}
	writeHTTPResult(c, result, err)
}

func decodeStrictBody(c *gin.Context, target any) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return errors.New("invalid request body")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func requireEmptyBody(c *gin.Context) error {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		return errors.New("invalid request body")
	}
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("{}")) {
		return nil
	}
	return errors.New("request body must be empty")
}

func writeHTTPResult(c *gin.Context, value any, err error) {
	if err == nil {
		c.JSON(http.StatusOK, value)
		return
	}
	status := http.StatusInternalServerError
	message := "task language server request failed"
	switch {
	case errors.Is(err, ErrUnsupportedLanguage), errors.Is(err, ErrInvalidPolicy):
		status, message = http.StatusBadRequest, err.Error()
	case errors.Is(err, ErrServerDisabled), errors.Is(err, ErrAttachmentNotReady):
		status, message = http.StatusConflict, err.Error()
	case errors.Is(err, ErrTaskNotReady), errors.Is(err, ErrExecutorUnsupported):
		status, message = http.StatusUnprocessableEntity, err.Error()
	case errors.Is(err, repoerrors.ErrTaskNotFound), strings.Contains(strings.ToLower(err.Error()), "not found"):
		status, message = http.StatusNotFound, "task not found"
	}
	c.JSON(status, gin.H{string(PhaseError): message})
}
