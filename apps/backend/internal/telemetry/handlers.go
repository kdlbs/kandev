package telemetry

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes the consent record and the frontend event intake.
type Handler struct {
	svc *Service
}

// RegisterRoutes mounts the telemetry HTTP surface. The routes exist
// even when telemetry is env-disabled so the frontend can read that
// state (and hide its consent prompts) instead of guessing from 404s.
func RegisterRoutes(router gin.IRouter, svc *Service) {
	h := &Handler{svc: svc}
	api := router.Group("/api/v1/telemetry")
	api.GET("/consent", h.getConsent)
	api.PUT("/consent", h.putConsent)
	api.POST("/events", h.postEvents)
}

func (h *Handler) getConsent(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.Consent())
}

type putConsentRequest struct {
	Status ConsentStatus `json:"status"`
}

func (h *Handler) putConsent(c *gin.Context) {
	var req putConsentRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid consent payload"})
		return
	}
	if req.Status != ConsentGranted && req.Status != ConsentDenied {
		c.JSON(http.StatusBadRequest, gin.H{"error": "status must be granted or denied"})
		return
	}
	state, err := h.svc.SetConsent(c.Request.Context(), req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save consent"})
		return
	}
	c.JSON(http.StatusOK, state)
}

type postEventsRequest struct {
	Events []UIEventSubmission `json:"events"`
}

// maxEventsBodyBytes bounds the /events request body before JSON
// decoding; legitimate payloads are a few hundred bytes.
const maxEventsBodyBytes = 16 << 10

// postEvents accepts allowlisted UI events. It always answers 202:
// invalid entries are dropped server-side and consent gating happens at
// enqueue time, so the response deliberately carries no consent signal
// beyond the accepted count.
func (h *Handler) postEvents(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxEventsBodyBytes)
	var req postEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid events payload"})
		return
	}
	accepted := h.svc.EnqueueUI(req.Events)
	c.JSON(http.StatusAccepted, gin.H{"accepted": accepted})
}
