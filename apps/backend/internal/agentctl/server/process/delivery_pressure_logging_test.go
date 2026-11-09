package process

import (
	"context"
	"path/filepath"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/agentctl/journal"
	"github.com/kandev/kandev/internal/agentctl/server/config"
	"github.com/kandev/kandev/internal/common/logger"
)

func pressureTestManagerWithObserver(t *testing.T) (*Manager, *journal.Journal, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	m := NewManager(&config.InstanceConfig{
		WorkDir:            t.TempDir(),
		SessionID:          "session",
		DurableJournalPath: filepath.Join(t.TempDir(), "original"),
	}, log)
	if err := m.deliveryJournal.Close(); err != nil {
		t.Fatal(err)
	}
	j, err := journal.Open(journal.Config{Path: filepath.Join(t.TempDir(), "pressure-log"), MaxEventBytes: 2048, MaxStreamBytes: 4096, MaxJournalBytes: 16384, ReserveBytes: 512})
	if err != nil {
		t.Fatal(err)
	}
	m.deliveryJournal = j
	m.status.Store(StatusRunning)
	t.Cleanup(func() {
		m.CloseAdmission()
		if err := m.closeDeliveryJournal(); err != nil {
			t.Error(err)
		}
	})
	return m, j, logs
}

func pressureTransitionLevels(logs *observer.ObservedLogs) []zapcore.Level {
	var levels []zapcore.Level
	for _, entry := range logs.FilterMessage("durable delivery pressure changed").All() {
		levels = append(levels, entry.Level)
	}
	return levels
}

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-002.5
func TestDeliveryPressureHealthyTransitionSeverity(t *testing.T) {
	m, j, logs := pressureTestManagerWithObserver(t)
	ctx := context.Background()

	if err := m.refreshDeliveryPressure(ctx, j, ""); err != nil {
		t.Fatal(err)
	}
	last := fillPressureJournal(t, j)
	if err := m.refreshDeliveryPressure(ctx, j, ""); err != nil {
		t.Fatal(err)
	}
	if err := j.Acknowledge(ctx, "session", last); err != nil {
		t.Fatal(err)
	}
	if err := m.refreshDeliveryPressure(ctx, j, ""); err != nil {
		t.Fatal(err)
	}

	levels := pressureTransitionLevels(logs)
	if len(levels) != 3 {
		t.Fatalf("pressure transition levels = %v, want initial healthy, guarded, recovered healthy", levels)
	}
	if levels[0] != zapcore.InfoLevel {
		t.Fatalf("initial healthy transition level = %v, want info", levels[0])
	}
	if levels[1] != zapcore.WarnLevel {
		t.Fatalf("guarded transition level = %v, want warn", levels[1])
	}
	if levels[2] != zapcore.InfoLevel {
		t.Fatalf("recovered healthy transition level = %v, want info", levels[2])
	}
	for _, entry := range logs.FilterMessage("durable delivery pressure changed").All() {
		if entry.ContextMap()["state"] == "healthy" && entry.Level != zapcore.InfoLevel {
			t.Fatalf("healthy state logged at %v: %+v", entry.Level, entry.ContextMap())
		}
	}
}
