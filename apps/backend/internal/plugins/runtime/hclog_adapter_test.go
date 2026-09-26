package runtime

import (
	"sync/atomic"
	"testing"

	hclog "github.com/hashicorp/go-hclog"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func newObservedLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log, logs
}

func TestHCLogAdapterDowngradesExpectedExitOnStop(t *testing.T) {
	tests := []struct {
		name      string
		stopping  bool
		msg       string
		wantLevel zapcore.Level
	}{
		{
			name:      "expected exit during deliberate kill logs debug",
			stopping:  true,
			msg:       "plugin process exited",
			wantLevel: zapcore.DebugLevel,
		},
		{
			name:      "unexpected exit while running logs error",
			stopping:  false,
			msg:       "plugin process exited",
			wantLevel: zapcore.ErrorLevel,
		},
		{
			name:      "unrelated error during stop still logs error",
			stopping:  true,
			msg:       "handshake failed",
			wantLevel: zapcore.ErrorLevel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logs := newObservedLogger(t)
			stopping := &atomic.Bool{}
			stopping.Store(tt.stopping)
			a := newHCLogAdapter(log, stopping)

			a.Error(tt.msg, "error", "signal: terminated")

			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("expected 1 log entry, got %d", len(entries))
			}
			if entries[0].Level != tt.wantLevel {
				t.Fatalf("level = %v, want %v", entries[0].Level, tt.wantLevel)
			}
			if entries[0].Message != tt.msg {
				t.Fatalf("message = %q, want %q", entries[0].Message, tt.msg)
			}
		})
	}
}

func TestHCLogAdapterForwardsArgsAsFields(t *testing.T) {
	log, logs := newObservedLogger(t)
	a := newHCLogAdapter(log, &atomic.Bool{})

	a.Warn("plugin restarting", "plugin", "github-status", "attempt", 2)

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["plugin"] != "github-status" {
		t.Fatalf("plugin field = %v, want github-status", fields["plugin"])
	}
	if fields["attempt"] != int64(2) {
		t.Fatalf("attempt field = %v, want 2", fields["attempt"])
	}
}

func TestHCLogAdapterLevelRouting(t *testing.T) {
	log, logs := newObservedLogger(t)
	a := newHCLogAdapter(log, &atomic.Bool{})

	a.Log(hclog.Warn, "via Log", "k", "v")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Level != zapcore.WarnLevel {
		t.Fatalf("level = %v, want warn", entries[0].Level)
	}
}
