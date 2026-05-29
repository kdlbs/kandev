// Package handlers exposes the HTTP surface for the voice-input transcription
// fallback. The endpoint is unauthenticated (matches /api/v1/features) — the
// Web Speech API path is preferred by the frontend, so this server-side
// fallback only runs when the browser cannot do it locally.
package handlers

import (
	"errors"
	"io"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/voice/transcribe"
)

// maxAudioBytes caps the multipart audio payload. Whisper accepts up to 25 MB
// per request; we cap lower so a stuck mic doesn't blow up backend memory or
// burn API spend on a stuck recording.
const maxAudioBytes = 10 * 1024 * 1024

// Handlers wires the transcribe service into Gin routes.
type Handlers struct {
	svc *transcribe.Service
	log *logger.Logger
}

// NewHandlers constructs a Handlers from a transcribe Service.
func NewHandlers(svc *transcribe.Service, log *logger.Logger) *Handlers {
	return &Handlers{
		svc: svc,
		log: log.WithFields(zap.String("component", "voice-handlers")),
	}
}

// RegisterRoutes mounts the voice transcription endpoint.
func RegisterRoutes(router *gin.Engine, svc *transcribe.Service, log *logger.Logger) {
	h := NewHandlers(svc, log)
	api := router.Group("/api/v1")
	api.POST("/transcribe", h.httpTranscribe)
}

func (h *Handlers) httpTranscribe(c *gin.Context) {
	if h.svc == nil || !h.svc.Configured() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "voice transcription is not configured on this server",
		})
		return
	}

	// MaxBytesReader makes the io.ReadAll below short-circuit with an error
	// once the cap is exceeded, instead of letting Gin buffer the whole body.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxAudioBytes)

	fh, err := c.FormFile("audio")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "audio file is required (multipart field 'audio')"})
		return
	}

	file, err := fh.Open()
	if err != nil {
		h.log.Warn("open uploaded audio failed", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot open uploaded audio"})
		return
	}
	defer func() { _ = file.Close() }()

	data, err := io.ReadAll(file)
	if err != nil {
		h.log.Warn("read uploaded audio failed", zap.Error(err))
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "audio payload too large or unreadable"})
		return
	}
	if len(data) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "audio file is empty"})
		return
	}

	mime := fh.Header.Get("Content-Type")
	text, err := h.svc.Transcribe(c.Request.Context(), data, mime, fh.Filename)
	if err != nil {
		h.respondError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"text": text})
}

func (h *Handlers) respondError(c *gin.Context, err error) {
	if errors.Is(err, transcribe.ErrNotConfigured) {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "voice transcription is not configured on this server",
		})
		return
	}
	var upstream *transcribe.UpstreamError
	if errors.As(err, &upstream) {
		h.log.Warn("whisper upstream error",
			zap.Int("status", upstream.StatusCode),
			zap.String("body", upstream.Body),
		)
		c.JSON(http.StatusBadGateway, gin.H{"error": "upstream transcription error"})
		return
	}
	h.log.Error("transcription failed", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "transcription failed"})
}
