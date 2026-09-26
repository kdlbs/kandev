package github

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

func TestPollerLogCleanupError(t *testing.T) {
	tests := []struct {
		name      string
		err       error
		wantLevel zapcore.Level
		wantMsg   string
	}{
		{
			name:      "generic error logs warn",
			err:       errors.New("db is down"),
			wantLevel: zapcore.WarnLevel,
			wantMsg:   "failed to sweep orphaned review tasks",
		},
		{
			name:      "context canceled logs debug",
			err:       context.Canceled,
			wantLevel: zapcore.DebugLevel,
			wantMsg:   "failed to sweep orphaned review tasks (context canceled during shutdown)",
		},
		{
			name:      "wrapped context canceled logs debug",
			err:       fmt.Errorf("sweep: %w", context.Canceled),
			wantLevel: zapcore.DebugLevel,
			wantMsg:   "failed to sweep orphaned review tasks (context canceled during shutdown)",
		},
		{
			name:      "deadline exceeded is not treated as shutdown",
			err:       context.DeadlineExceeded,
			wantLevel: zapcore.WarnLevel,
			wantMsg:   "failed to sweep orphaned review tasks",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("NewFromZap: %v", err)
			}
			p := &Poller{logger: log}

			p.logCleanupError("failed to sweep orphaned review tasks", tt.err)

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

func TestPollerLogCleanupErrorForwardsFields(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	p := &Poller{logger: log}

	p.logCleanupError("failed to cleanup merged review tasks", errors.New("boom"),
		zap.String("watch_id", "rw-1"))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["watch_id"] != "rw-1" {
		t.Fatalf("watch_id field = %v, want rw-1", fields["watch_id"])
	}
	if fields["error"] != "boom" {
		t.Fatalf("error field = %v, want boom", fields["error"])
	}
}
