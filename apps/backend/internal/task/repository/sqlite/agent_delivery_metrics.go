package sqlite

import (
	"context"
	"expvar"
)

var (
	agentDeliveryInboxLagEvents      = expvar.NewInt("agent_delivery_inbox_lag_events")
	agentDeliveryProjectionLagEvents = expvar.NewInt("agent_delivery_projection_lag_events")
)

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
