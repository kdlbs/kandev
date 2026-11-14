package dashboard

import (
	"context"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
	"github.com/kandev/kandev/internal/system/sessioncapacity"
)

// controlCapacityCacheTTL collapses the parallel workspace dashboard reads the
// integrated control makes into one admission observation.
const controlCapacityCacheTTL = 2 * time.Second

// ControlCapacitySource is the admission owner's read-only observation. The
// optional seam keeps the control dashboard free of launch or admission work.
type ControlCapacitySource interface {
	SessionCapacityObservation(context.Context) sessioncapacity.Observation
}

type controlCapacityCache struct {
	source     ControlCapacitySource
	ttl        time.Duration
	now        func() time.Time
	mu         sync.Mutex
	observedAt time.Time
	value      *sqlite.ControlWaiting
}

func newControlCapacityCache(source ControlCapacitySource) *controlCapacityCache {
	return &controlCapacityCache{source: source, ttl: controlCapacityCacheTTL, now: time.Now}
}

// waiting returns one observation per TTL. The mutex is held across the
// observation so concurrent dashboard reads join the same read instead of
// contending on the admission owner's lock.
func (c *controlCapacityCache) waiting(ctx context.Context) *sqlite.ControlWaiting {
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if c.value != nil && now.Sub(c.observedAt) < c.ttl {
		return c.value
	}
	c.value = projectControlCapacityWaiting(ctx, c.source)
	c.observedAt = now
	return c.value
}

// SetControlCapacitySource wires the installation-wide shared-capacity
// observation used by the control dashboard aggregate.
func (s *DashboardService) SetControlCapacitySource(source ControlCapacitySource) {
	if s == nil {
		return
	}
	if source == nil {
		s.controlCapacity = nil
		return
	}
	s.controlCapacity = newControlCapacityCache(source)
}

func (s *DashboardService) controlCapacityWaiting(ctx context.Context) *sqlite.ControlWaiting {
	if s == nil || s.controlCapacity == nil {
		return nil
	}
	return s.controlCapacity.waiting(ctx)
}

// projectControlCapacityWaiting projects the waiter observation into the
// aggregate's two figures. Grade counts a grade-limit refusal, including
// both-limit refusals; Total counts every refusal.
func projectControlCapacityWaiting(ctx context.Context, source ControlCapacitySource) *sqlite.ControlWaiting {
	waiting := source.SessionCapacityObservation(ctx).Waiting
	if !waiting.Known {
		return &sqlite.ControlWaiting{}
	}
	total, grade := 0, 0
	for reason, count := range waiting.Counts {
		total += count
		if reason == sessioncapacity.WaitingReasonGradeLimit || reason == sessioncapacity.WaitingReasonBothLimits {
			grade += count
		}
	}
	return &sqlite.ControlWaiting{Known: true, Grade: grade, Total: total}
}
