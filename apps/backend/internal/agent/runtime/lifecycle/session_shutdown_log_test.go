package lifecycle

import (
	"context"
	"fmt"
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestIsBenignPromptTeardown(t *testing.T) {
	tests := []struct {
		name string
		msg  string
		want bool
	}{
		{name: "empty", msg: "", want: false},
		{name: "peer disconnected", msg: "-32603 ... peer disconnected before response", want: true},
		{name: "connection closed", msg: "prompt failed: connection closed", want: true},
		{name: "notification queue overflow", msg: "notification queue overflow", want: true},
		{name: "context canceled", msg: fmt.Sprintf("send prompt: %s", context.Canceled.Error()), want: true},
		{name: "cancel escalated sentinel", msg: "cancel escalated after grace", want: true},
		{name: "prompt abandoned after cancel", msg: "prompt abandoned after cancel", want: true},
		{name: "genuine agent error", msg: "agent reported: tool execution failed", want: false},
		{name: "deadline exceeded is not benign as string", msg: context.DeadlineExceeded.Error(), want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isBenignPromptTeardown(tt.msg); got != tt.want {
				t.Fatalf("isBenignPromptTeardown(%q) = %v, want %v", tt.msg, got, tt.want)
			}
		})
	}
}

func TestStopChClosed(t *testing.T) {
	t.Run("nil stopCh is not shutting down", func(t *testing.T) {
		sm := &SessionManager{}
		if sm.stopChClosed() {
			t.Fatal("nil stopCh should report not shutting down")
		}
	})
	t.Run("open stopCh is not shutting down", func(t *testing.T) {
		sm := &SessionManager{stopCh: make(chan struct{})}
		if sm.stopChClosed() {
			t.Fatal("open stopCh should report not shutting down")
		}
	})
	t.Run("closed stopCh is shutting down", func(t *testing.T) {
		ch := make(chan struct{})
		close(ch)
		sm := &SessionManager{stopCh: ch}
		if !sm.stopChClosed() {
			t.Fatal("closed stopCh should report shutting down")
		}
	})
}

// observedLogger returns a Logger backed by an observer core capturing entries
// at WARN and above, so tests can assert the exact level a site logged at.
func observedLogger(t *testing.T) (*logger.Logger, *observer.ObservedLogs) {
	t.Helper()
	core, logs := observer.New(zapcore.WarnLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	return log, logs
}

func TestPromptCompletionLogLevel(t *testing.T) {
	tests := []struct {
		name       string
		errText    string
		stopClosed bool
		wantLevel  zapcore.Level
		wantMsg    string
	}{
		{
			name:      "transport death logs warn",
			errText:   "-32603 ... peer disconnected before response",
			wantLevel: zapcore.WarnLevel,
			wantMsg:   "prompt aborted during shutdown",
		},
		{
			name:       "shutdown in progress logs warn",
			errText:    "some agent error",
			stopClosed: true,
			wantLevel:  zapcore.WarnLevel,
			wantMsg:    "prompt aborted during shutdown",
		},
		{
			name:      "genuine agent error on active session logs error",
			errText:   "agent reported: tool execution failed",
			wantLevel: zapcore.ErrorLevel,
			wantMsg:   "prompt completed with error",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			log, logs := observedLogger(t)
			// Mirror the classification branch in waitForPromptDone.
			sm := &SessionManager{logger: log}
			if tt.stopClosed {
				ch := make(chan struct{})
				close(ch)
				sm.stopCh = ch
			}
			if isBenignPromptTeardown(tt.errText) || sm.stopChClosed() {
				sm.logger.Warn("prompt aborted during shutdown", zap.String("error", tt.errText))
			} else {
				sm.logger.Error("prompt completed with error", zap.String("error", tt.errText))
			}
			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("expected 1 log entry, got %d", len(entries))
			}
			if entries[0].Level != tt.wantLevel {
				t.Fatalf("level = %v, want %v", entries[0].Level, tt.wantLevel)
			}
			if entries[0].Message != tt.wantMsg {
				t.Fatalf("message = %q, want %q", entries[0].Message, tt.wantMsg)
			}
		})
	}
}
