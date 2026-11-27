package sqlite

import (
	"context"
	"expvar"
	"time"
)

const (
	agentDeliveryDurabilityBarrierSuccess = "success"
	agentDeliveryDurabilityBarrierError   = "error"
)

var (
	agentDeliveryInboxLagEvents      = expvar.NewInt("agent_delivery_inbox_lag_events")
	agentDeliveryProjectionLagEvents = expvar.NewInt("agent_delivery_projection_lag_events")

	agentDeliveryDurabilityBarrierTotal   = expvar.NewMap("agent_delivery_durability_barrier_total")
	agentDeliveryDurabilityBarrierSeconds = expvar.NewMap("agent_delivery_durability_barrier_seconds")
)

// observeAgentDeliveryDurabilityBarrier publishes one bounded barrier result
// and accumulates the duration buckets through one second.
func observeAgentDeliveryDurabilityBarrier(err error, elapsed time.Duration) {
	result := agentDeliveryDurabilityBarrierSuccess
	if err != nil {
		result = agentDeliveryDurabilityBarrierError
	}
	agentDeliveryDurabilityBarrierTotal.Add(result, 1)
	seconds := elapsed.Seconds()
	for _, bucket := range []struct {
		limit float64
		key   string
	}{
		{0.005, "le_5ms"}, {0.01, "le_10ms"}, {0.025, "le_25ms"}, {0.05, "le_50ms"},
		{0.1, "le_100ms"}, {0.25, "le_250ms"}, {0.5, "le_500ms"}, {1, "le_1"},
	} {
		if seconds <= bucket.limit {
			agentDeliveryDurabilityBarrierSeconds.Add(bucket.key, 1)
		}
	}
	if seconds > 1 {
		agentDeliveryDurabilityBarrierSeconds.Add("gt_1", 1)
	}
}

func (r *Repository) refreshAgentDeliveryLag(ctx context.Context) {
	if r == nil || r.ro == nil {
		return
	}
	var inboxLag, projectionLag int64
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT
			COALESCE(SUM(CASE WHEN remote_high_water > received_sequence
				THEN remote_high_water - received_sequence ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN received_sequence > projected_sequence
				THEN received_sequence - projected_sequence ELSE 0 END), 0)
		FROM agent_delivery_cursors`)).Scan(&inboxLag, &projectionLag)
	if err != nil {
		// A missing or unavailable metric query is not a healthy zero. Keep
		// the value negative so operators can distinguish unavailable data.
		agentDeliveryInboxLagEvents.Set(-1)
		agentDeliveryProjectionLagEvents.Set(-1)
		return
	}
	agentDeliveryInboxLagEvents.Set(inboxLag)
	agentDeliveryProjectionLagEvents.Set(projectionLag)
}
