package retention

import (
	"errors"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

const responseErrorKey = "error"

// HandlerConfig wires the HTTP surface to the package's own stores.
type HandlerConfig struct {
	SettingsStore *SettingsStore
	Sweeper       *Sweeper
	// OnSettingsChanged, when set, is called with the normalized document
	// after a successful PUT so the running scheduler re-arms its timers
	// from the new settings without a restart (AC-004.5).
	OnSettingsChanged func(Settings)
	LogError          func(string, error)
}

// Handler serves GET/PUT /api/v1/system/retention.
type Handler struct {
	config HandlerConfig
}

// NewHandler wires a Handler to its dependencies.
func NewHandler(config HandlerConfig) *Handler {
	return &Handler{config: config}
}

func (h *Handler) logError(message string, err error) {
	if h.config.LogError != nil {
		h.config.LogError(message, err)
	}
}

// RegisterRoutes wires GET/PUT /api/v1/system/retention: GET is
// member-readable, matching every sibling System-pages surface (storage,
// queue settings, sleep inhibition), and readable while retention is
// disabled (AC-OFFICE-RUN-HISTORY-RETENTION-004.8); PUT is admin-scoped.
func RegisterRoutes(read, admin *gin.RouterGroup, handler *Handler) {
	read.GET("/retention", handler.getRetention)
	admin.PUT("/retention", handler.putRetention)
}

// Status is the GET response body: effective settings, the most recently
// completed sweep (nil before the first one — AC-004.7), the
// separately-held skip record, and the per-thresholded-table retained-count
// census (AC-004.6).
type Status struct {
	Settings       Settings       `json:"settings"`
	LastSweep      *LastSweep     `json:"last_sweep"`
	SkipCount      int64          `json:"skip_count"`
	LastSkipAt     *time.Time     `json:"last_skip_at,omitempty"`
	RetainedCounts RetainedCounts `json:"retained_counts"`
}

func (h *Handler) getRetention(c *gin.Context) {
	settings, err := h.config.SettingsStore.GetSettings(c.Request.Context())
	if err != nil {
		h.logError("failed to load retention settings", err)
	}

	status := Status{
		Settings:       settings,
		RetainedCounts: h.config.Sweeper.CensusSnapshot(),
	}
	if last, ok := h.config.Sweeper.LastSweepSnapshot(); ok {
		status.LastSweep = &last
	}
	if count, lastAt := h.config.Sweeper.SkipSnapshot(); count > 0 {
		status.SkipCount = count
		status.LastSkipAt = &lastAt
	}
	c.JSON(http.StatusOK, status)
}

func (h *Handler) putRetention(c *gin.Context) {
	body, err := c.GetRawData()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: "failed to read request body"})
		return
	}

	settings, err := decodeRetentionSettings(body)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
		return
	}

	saved, err := h.config.SettingsStore.SaveSettings(c.Request.Context(), settings)
	if err != nil {
		if errors.Is(err, ErrValidation) {
			c.JSON(http.StatusBadRequest, gin.H{responseErrorKey: err.Error()})
			return
		}
		h.logError("failed to save retention settings", err)
		c.JSON(http.StatusInternalServerError, gin.H{responseErrorKey: "failed to save retention settings"})
		return
	}

	if h.config.OnSettingsChanged != nil {
		h.config.OnSettingsChanged(saved)
	}
	c.JSON(http.StatusOK, saved)
}
