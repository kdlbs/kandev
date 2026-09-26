package adminmetrics

import (
	"expvar"
	"testing"
)

func TestAdministrativeTurnMetricsRecordOutcomes(t *testing.T) {
	pending := expvar.Get("administrative_turn_completion_intents_pending").(*expvar.Int)
	completion := expvar.Get("administrative_turn_completion_reconciled_total").(*expvar.Map)
	deliveries := expvar.Get("administrative_turn_message_delivery_outcomes_total").(*expvar.Map)
	retries := expvar.Get("administrative_turn_message_delivery_retries_total").(*expvar.Int)
	duplicates := expvar.Get("administrative_turn_message_delivery_duplicates_total").(*expvar.Int)
	denials := expvar.Get("administrative_turn_stale_control_denials_total").(*expvar.Map)

	RecordCompletionPending(3)
	if pending.Value() != 3 {
		t.Fatalf("pending completion intents = %d, want 3", pending.Value())
	}
	assertMapDelta(t, completion, "outcome=settled;cause=quiet_grace", func() {
		RecordCompletionReconciled("settled", "quiet_grace")
	})
	assertMapDelta(t, deliveries, "outcome=recoverable", func() {
		RecordMessageDeliveryOutcome("recoverable", 1)
	})
	assertIntDelta(t, retries, RecordMessageDeliveryRetry)
	assertIntDelta(t, duplicates, RecordMessageDeliveryDuplicate)
	assertMapDelta(t, denials, "reason=active_turn", func() {
		RecordStaleControlDenial("active_turn")
	})
}

func assertIntDelta(t *testing.T, counter *expvar.Int, record func()) {
	t.Helper()
	before := counter.Value()
	record()
	if counter.Value() != before+1 {
		t.Fatalf("counter = %d, want %d", counter.Value(), before+1)
	}
}

func assertMapDelta(t *testing.T, metrics *expvar.Map, key string, record func()) {
	t.Helper()
	before := mapValue(metrics, key)
	record()
	if got := mapValue(metrics, key); got != before+1 {
		t.Fatalf("metric %q = %d, want %d", key, got, before+1)
	}
}

func mapValue(metrics *expvar.Map, key string) int64 {
	value := metrics.Get(key)
	if value == nil {
		return 0
	}
	counter, ok := value.(*expvar.Int)
	if !ok {
		return 0
	}
	return counter.Value()
}
