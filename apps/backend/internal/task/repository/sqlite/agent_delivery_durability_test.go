package sqlite

import (
	"context"
	"expvar"
	"testing"
)

func agentDeliveryDurabilityMetric(t *testing.T, key string) int64 {
	t.Helper()
	counter, ok := expvar.Get("agent_delivery_durability_barrier_total").(*expvar.Map)
	if !ok {
		t.Fatal("agent_delivery_durability_barrier_total is not published")
	}
	value, ok := counter.Get(key).(*expvar.Int)
	if !ok {
		return 0
	}
	return value.Value()
}

// @covers AC-PLATFORM-DELIVERY-WRITE-COST-001.3
func TestAwaitAgentDeliveryDurability(t *testing.T) {
	repo := newRepoForSessionTests(t)
	ctx := context.Background()
	for _, name := range []string{
		"agent_delivery_durability_barrier_total",
		"agent_delivery_durability_barrier_seconds",
	} {
		if expvar.Get(name) == nil {
			t.Fatalf("expvar %q is not published", name)
		}
	}

	successesBefore := agentDeliveryDurabilityMetric(t, "success")
	if err := repo.AwaitAgentDeliveryDurability(ctx); err != nil {
		t.Fatalf("await durability: %v", err)
	}
	if got := agentDeliveryDurabilityMetric(t, "success"); got <= successesBefore {
		t.Fatalf("success metric = %d, want greater than %d", got, successesBefore)
	}

	errorsBefore := agentDeliveryDurabilityMetric(t, "error")
	if _, err := repo.db.Exec(`DROP TABLE kandev_meta`); err != nil {
		t.Fatalf("drop kandev_meta: %v", err)
	}
	if err := repo.AwaitAgentDeliveryDurability(ctx); err == nil {
		t.Fatal("durability barrier succeeded without kandev_meta")
	}
	if got := agentDeliveryDurabilityMetric(t, "error"); got <= errorsBefore {
		t.Fatalf("error metric = %d, want greater than %d", got, errorsBefore)
	}
}
