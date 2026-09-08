// Package persistence exposes the authenticated required-store diagnostics
// endpoint used by the system pages and operator tooling.
package persistence

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/kandev/kandev/internal/persistence/requiredstores"
)

// StoreDiagnostic is the public, sanitized view of one catalog entry.
type StoreDiagnostic struct {
	ID            string     `json:"id"`
	OwnerPackage  string     `json:"owner_package"`
	State         string     `json:"state"`
	LastCheckedAt *time.Time `json:"last_checked_at,omitempty"`
	Error         string     `json:"error,omitempty"`
}

// Response is the complete required-persistence diagnostic payload.
type Response struct {
	Driver string            `json:"driver"`
	State  string            `json:"state"`
	Stores []StoreDiagnostic `json:"stores"`
}

// Handler serves diagnostics for one required-store tracker.
type Handler struct {
	tracker *requiredstores.Tracker
	health  *requiredstores.Health
	driver  string
}

// NewHandler creates a diagnostics handler. The driver is the stable driver
// name reported by the configured database pool.
func NewHandler(tracker *requiredstores.Tracker, health *requiredstores.Health, driver string) *Handler {
	return &Handler{tracker: tracker, health: health, driver: driver}
}

// Handle writes one stable row for every catalog descriptor.
func (h *Handler) Handle(c *gin.Context) {
	if h == nil || h.tracker == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "persistence diagnostics are unavailable"})
		return
	}
	snapshot := h.tracker.Snapshot()
	stores := make([]StoreDiagnostic, len(snapshot))
	for index, status := range snapshot {
		stores[index] = diagnosticFor(status)
	}
	state := h.tracker.AggregateState()
	if h.health != nil {
		state = h.health.State()
	}
	c.JSON(http.StatusOK, Response{Driver: h.driver, State: string(state), Stores: stores})
}

// RegisterRoutes mounts the diagnostics endpoint under a system route group.
func RegisterRoutes(group *gin.RouterGroup, handler *Handler) {
	if group == nil || handler == nil {
		return
	}
	group.GET("/diagnostics/persistence", handler.Handle)
}

func diagnosticFor(status requiredstores.Status) StoreDiagnostic {
	diagnostic := StoreDiagnostic{
		ID: status.ID, OwnerPackage: status.OwnerPackage, State: string(status.State),
		Error: requiredstores.PublicError(status),
	}
	if !status.LastCheckedAt.IsZero() {
		checkedAt := status.LastCheckedAt
		diagnostic.LastCheckedAt = &checkedAt
	}
	return diagnostic
}
