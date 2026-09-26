package client

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

const (
	AvailabilityStatusAvailable      = "available"
	AvailabilityStatusUnavailable    = "unavailable"
	AvailabilityReasonAgentctlExited = "agentctl_exited"
)

// AvailabilitySnapshot is the sanitized, install-wide agent runtime state
// exposed to authenticated clients. It intentionally contains no process or
// launcher details.
type AvailabilitySnapshot struct {
	Status     string     `json:"status"`
	Reason     string     `json:"reason,omitempty"`
	OccurredAt *time.Time `json:"occurred_at,omitempty"`
}

// Availability owns the monotonic runtime availability state. A runtime is
// unpublished until its health check and authentication handshake succeed;
// after that point, an unexpected launcher exit is a terminal unavailable
// state for the lifetime of the backend process.
type Availability struct {
	mu        sync.RWMutex
	publishMu sync.Mutex
	eventBus  bus.EventBus
	logger    *logger.Logger
	snapshot  AvailabilitySnapshot
	published bool
}

// NewAvailability creates an unpublished runtime availability tracker.
func NewAvailability(eventBus bus.EventBus, log *logger.Logger) *Availability {
	return &Availability{eventBus: eventBus, logger: log}
}

// Snapshot returns a copy of the current published state.
func (a *Availability) Snapshot() (AvailabilitySnapshot, bool) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.published {
		return AvailabilitySnapshot{}, false
	}

	snapshot := a.snapshot
	if a.snapshot.OccurredAt != nil {
		occurredAt := *a.snapshot.OccurredAt
		snapshot.OccurredAt = &occurredAt
	}
	return snapshot, true
}

// MarkAvailable publishes the first healthy-and-authenticated state.
func (a *Availability) MarkAvailable() {
	a.publishMu.Lock()
	defer a.publishMu.Unlock()

	a.mu.Lock()
	if a.published {
		a.mu.Unlock()
		return
	}
	a.snapshot = AvailabilitySnapshot{Status: AvailabilityStatusAvailable}
	a.published = true
	snapshot := a.snapshot
	a.mu.Unlock()

	a.publish(snapshot)
}

// MarkUnavailable records an unexpected agentctl exit. The transition is
// intentionally fail-closed and cannot be reversed until the backend restarts.
func (a *Availability) MarkUnavailable() {
	a.publishMu.Lock()
	defer a.publishMu.Unlock()

	a.mu.Lock()
	if !a.published || a.snapshot.Status == AvailabilityStatusUnavailable {
		a.mu.Unlock()
		return
	}
	now := time.Now().UTC()
	a.snapshot = AvailabilitySnapshot{
		Status:     AvailabilityStatusUnavailable,
		Reason:     AvailabilityReasonAgentctlExited,
		OccurredAt: &now,
	}
	snapshot := a.snapshot
	a.mu.Unlock()

	a.publish(snapshot)
}

func (a *Availability) publish(snapshot AvailabilitySnapshot) {
	if a.eventBus == nil {
		return
	}
	if err := a.eventBus.Publish(
		context.Background(),
		events.AgentRuntimeAvailabilityChanged,
		bus.NewEvent(events.AgentRuntimeAvailabilityChanged, "agentctl", snapshot),
	); err != nil && a.logger != nil {
		a.logger.Warn("failed to publish agent runtime availability", zap.Error(err))
	}
}
