package lifecycle

import (
	"errors"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

// disconnectTestManager builds a Manager wired with just enough to run
// handleStreamDisconnect for the promptGeneration==0 path: an execution store,
// an event publisher over a tracking bus, and a logger backed by an observer
// core capturing DEBUG and above so the test can read back the exact level.
func disconnectTestManager(t *testing.T) (*Manager, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	mgr := &Manager{
		logger:         log,
		executionStore: NewExecutionStore(),
		eventPublisher: NewEventPublisher(&MockEventBusWithTracking{}, log),
	}
	return mgr, logs
}

// disconnectLogEntry returns the single entry whose message names the stream
// disconnect, ignoring the incidental "updated execution status" debug lines
// that UpdateStatus emits on the same logger.
func disconnectLogEntry(t *testing.T, logs *observer.ObservedLogs) observer.LoggedEntry {
	t.Helper()
	var found []observer.LoggedEntry
	for _, e := range logs.All() {
		if e.Message == "agent updates stream disconnected" ||
			e.Message == "agent updates stream disconnected during shutdown" {
			found = append(found, e)
		}
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 disconnect log entry, got %d (all: %v)", len(found), logs.All())
	}
	return found[0]
}

func TestHandleStreamDisconnectLogLevel(t *testing.T) {
	tests := []struct {
		name         string
		shuttingDown bool
		wantLevel    zapcore.Level
		wantMsg      string
	}{
		{
			name:         "active manager logs warn",
			shuttingDown: false,
			wantLevel:    zapcore.WarnLevel,
			wantMsg:      "agent updates stream disconnected",
		},
		{
			name:         "shutting down logs debug",
			shuttingDown: true,
			wantLevel:    zapcore.DebugLevel,
			wantMsg:      "agent updates stream disconnected during shutdown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, logs := disconnectTestManager(t)
			if tt.shuttingDown {
				mgr.shuttingDown.Store(true)
			}
			execution := &AgentExecution{ID: "exec-1", SessionID: "session-1"}
			if err := mgr.executionStore.Add(execution); err != nil {
				t.Fatalf("add execution: %v", err)
			}

			// promptGeneration==0 exercises the simple disconnect path.
			mgr.handleStreamDisconnect(execution, errors.New("use of closed network connection"), 0)

			entry := disconnectLogEntry(t, logs)
			if entry.Level != tt.wantLevel {
				t.Fatalf("level = %v, want %v", entry.Level, tt.wantLevel)
			}
			if entry.Message != tt.wantMsg {
				t.Fatalf("message = %q, want %q", entry.Message, tt.wantMsg)
			}
		})
	}
}
