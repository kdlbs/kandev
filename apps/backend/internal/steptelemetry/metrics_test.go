package steptelemetry

import (
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

func TestRecordLedgerRowBumpsCounterAndLogs(t *testing.T) {
	log, logs := observerLogger(t)
	before := stepTransitionsTotal.String()

	RecordLedgerRow(log, TriggerManualMove)

	after := stepTransitionsTotal.String()
	if after == before {
		t.Fatal("expvar counter telemetry_step_transitions_total did not change")
	}

	entries := logs.FilterMessage(metricStepTransitionWritten).All()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if entries[0].ContextMap()["trigger"] != string(TriggerManualMove) {
		t.Fatalf("trigger field = %v, want %q", entries[0].ContextMap()["trigger"], TriggerManualMove)
	}
}

func TestRecordLedgerRowNoopWhenLoggerNil(t *testing.T) {
	// Must not panic.
	RecordLedgerRow(nil, TriggerBulkMove)
}

func TestRecordTurnStampBumpsCounterAndLogsBothBranches(t *testing.T) {
	log, logs := observerLogger(t)

	RecordTurnStamp(log, true)
	RecordTurnStamp(log, false)

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
