package sqlite

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/stretchr/testify/require"
)

// Fixed-size isolated samples establish which portion of a mixed call is
// transaction work. Setup uses an unobserved context and completes first.
func TestWriterWorkloadIsolated(t *testing.T) {
	for _, operation := range []string{"idle", "receive", "project", "settle"} {
		t.Run(operation, func(t *testing.T) {
			w := newWriterWorkload(t, 1, 16384, 16)
			const rounds = 32
			plain := context.Background()
			events := make([][]*models.AgentDeliveryEvent, rounds)
			effects := make([][]*models.AgentDeliveryEffect, rounds)
			terminals := make([]*models.AgentDeliveryEvent, rounds)
			for i := range events {
				events[i], effects[i] = w.events(0, i)
				if operation == "project" {
					require.NoError(t, w.receive(plain, events[i]))
				}
				if operation == "settle" {
					var err error
					terminals[i], err = w.prepareTerminal(plain, 0, i)
					require.NoError(t, err)
				}
			}
			o := newWriterWorkloadObservation()
			ctx := context.WithValue(plain, writerWorkloadObservationKey{}, o)
			before := w.repo.db.Stats()
			started := time.Now()
			for i := range events {
				switch operation {
				case "receive":
					require.NoError(t, w.receive(ctx, events[i]))
				case "project":
					require.NoError(t, w.project(ctx, events[i], effects[i]))
				case "settle":
					require.NoError(t, w.settle(ctx, terminals[i]))
				case "idle":
					require.NoError(t, w.sidebar(ctx))
				}
				probeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				err := writerWorkloadMeasure(probeCtx, "health", func() error { return w.health.Check(probeCtx) })
				cancel()
				require.NoError(t, err)
			}
			after := w.repo.db.Stats()
			o.report(t, map[string]any{"isolated": operation, "rounds": rounds, "payload_bytes": w.bytes, "batch": w.batch,
				"elapsed_seconds": time.Since(started).Seconds(), "writer_wait_count": after.WaitCount - before.WaitCount})
		})
	}
}
