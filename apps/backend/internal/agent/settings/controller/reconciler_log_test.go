package controller

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

func TestProfileReconcilerLogReconcileError(t *testing.T) {
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
			wantMsg:   "reconcile: list profiles failed",
		},
		{
			name:      "context canceled logs debug",
			err:       context.Canceled,
			wantLevel: zapcore.DebugLevel,
			wantMsg:   "reconcile: list profiles failed (context canceled during shutdown)",
		},
		{
			name:      "wrapped context canceled logs debug",
			err:       fmt.Errorf("list: %w", context.Canceled),
			wantLevel: zapcore.DebugLevel,
			wantMsg:   "reconcile: list profiles failed (context canceled during shutdown)",
		},
		{
			name:      "deadline exceeded is not treated as shutdown",
			err:       context.DeadlineExceeded,
			wantLevel: zapcore.WarnLevel,
			wantMsg:   "reconcile: list profiles failed",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			log, err := logger.NewFromZap(zap.New(core))
			if err != nil {
				t.Fatalf("NewFromZap: %v", err)
			}
			r := &ProfileReconciler{log: log}

			r.logReconcileError("reconcile: list profiles failed", tt.err)

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

func TestProfileReconcilerLogReconcileErrorForwardsFields(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatalf("NewFromZap: %v", err)
	}
	r := &ProfileReconciler{log: log}

	r.logReconcileError("orphan cleanup: delete failed", errors.New("boom"),
		zap.String("profile_id", "p-1"))

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	fields := entries[0].ContextMap()
	if fields["profile_id"] != "p-1" {
		t.Fatalf("profile_id field = %v, want p-1", fields["profile_id"])
	}
	if fields["error"] != "boom" {
		t.Fatalf("error field = %v, want boom", fields["error"])
	}
}
