package steptelemetry

import (
	"expvar"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func observerLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log, logs
}

// expvarMapValue reads the current int64 value stored at key in an
// expvar.Map, or 0 if the key hasn't been set yet (Add creates its backing
// *expvar.Int lazily on first use).
func expvarMapValue(t *testing.T, m *expvar.Map, key string) int64 {
	t.Helper()
	v := m.Get(key)
	if v == nil {
		return 0
	}
	i, ok := v.(*expvar.Int)
	if !ok {
		t.Fatalf("expvar value at key %q is a %T, want *expvar.Int", key, v)
	}
	return i.Value()
}

func TestRecordLedgerRowBumpsCounterAndLogs(t *testing.T) {
	log, logs := observerLogger(t)
	key := metricLabel("trigger", string(TriggerManualMove))
	before := expvarMapValue(t, stepTransitionsTotal, key)

	RecordLedgerRow(log, TriggerManualMove)

	after := expvarMapValue(t, stepTransitionsTotal, key)
	if after != before+1 {
		t.Fatalf("telemetry_step_transitions_inserted_total[%s] = %d, want %d", key, after, before+1)
	}

	entries := logs.FilterMessage(metricStepTransitionInserted).All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].ContextMap()["trigger"] != string(TriggerManualMove) {
		t.Fatalf("trigger field = %v, want %q", entries[0].ContextMap()["trigger"], TriggerManualMove)
	}
}

// TestRecordLedgerRowCounterIsKeyedByTrigger pins that the expvar counter is
// keyed per-trigger, not one shared bucket. TestRecordLedgerRowBumpsCounter-
// AndLogs alone would still pass if incStepTransition collapsed every
// trigger into stepTransitionsTotal.Add("", 1) — this asserts recording one
// trigger leaves a different trigger's own bucket untouched.
func TestRecordLedgerRowCounterIsKeyedByTrigger(t *testing.T) {
	log, _ := observerLogger(t)
	moveKey := metricLabel("trigger", string(TriggerManualMove))
	bulkKey := metricLabel("trigger", string(TriggerBulkMove))
	beforeMove := expvarMapValue(t, stepTransitionsTotal, moveKey)
	beforeBulk := expvarMapValue(t, stepTransitionsTotal, bulkKey)

	RecordLedgerRow(log, TriggerManualMove)

	if got := expvarMapValue(t, stepTransitionsTotal, moveKey); got != beforeMove+1 {
		t.Fatalf("telemetry_step_transitions_inserted_total[%s] = %d, want %d", moveKey, got, beforeMove+1)
	}
	if got := expvarMapValue(t, stepTransitionsTotal, bulkKey); got != beforeBulk {
		t.Fatalf("telemetry_step_transitions_inserted_total[%s] = %d, want unchanged %d (recording %s must not bump %s's bucket)",
			bulkKey, got, beforeBulk, TriggerManualMove, TriggerBulkMove)
	}
}

func TestRecordLedgerRowNoopWhenLoggerNil(t *testing.T) {
	// Must not panic.
	RecordLedgerRow(nil, TriggerBulkMove)
}

func TestRecordTurnStampBumpsCounterAndLogsBothBranches(t *testing.T) {
	log, logs := observerLogger(t)
	presentKey := metricLabel("stamp", "present")
	absentKey := metricLabel("stamp", "absent")
	beforePresent := expvarMapValue(t, turnStampsTotal, presentKey)
	beforeAbsent := expvarMapValue(t, turnStampsTotal, absentKey)

	RecordTurnStamp(log, true)
	if got := expvarMapValue(t, turnStampsTotal, presentKey); got != beforePresent+1 {
		t.Fatalf("telemetry_turn_stamps_total[%s] = %d after the present branch, want %d", presentKey, got, beforePresent+1)
	}
	if got := expvarMapValue(t, turnStampsTotal, absentKey); got != beforeAbsent {
		t.Fatalf("telemetry_turn_stamps_total[%s] = %d after the present branch, want unchanged %d", absentKey, got, beforeAbsent)
	}

	RecordTurnStamp(log, false)
	if got := expvarMapValue(t, turnStampsTotal, absentKey); got != beforeAbsent+1 {
		t.Fatalf("telemetry_turn_stamps_total[%s] = %d after the absent branch, want %d", absentKey, got, beforeAbsent+1)
	}
	if got := expvarMapValue(t, turnStampsTotal, presentKey); got != beforePresent+1 {
		t.Fatalf("telemetry_turn_stamps_total[%s] = %d after the absent branch, want unchanged %d", presentKey, got, beforePresent+1)
	}

	entries := logs.FilterMessage(metricTurnStamped).All()
	if len(entries) != 2 {
		t.Fatalf("log entries = %d, want 2", len(entries))
	}
	if entries[0].ContextMap()["present"] != true {
		t.Fatalf("first entry present = %v, want true", entries[0].ContextMap()["present"])
	}
	if entries[1].ContextMap()["present"] != false {
		t.Fatalf("second entry present = %v, want false", entries[1].ContextMap()["present"])
	}
}

func TestRecordTurnStampNoopWhenLoggerNil(t *testing.T) {
	// Must not panic.
	RecordTurnStamp(nil, true)
}

func TestMetricLabelRejectsOddPairs(t *testing.T) {
	if got := metricLabel("k1"); got != "" {
		t.Fatalf("metricLabel with odd pairs = %q, want empty", got)
	}
}
