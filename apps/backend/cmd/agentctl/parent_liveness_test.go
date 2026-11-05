package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/kandev/kandev/internal/common/logger"
)

// @covers AC-PLATFORM-DIAGNOSTIC-SIGNAL-004.2
func TestParentLivenessEOFDoesNotClaimProcessDeath(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	log, err := logger.NewFromZap(zap.New(core))
	if err != nil {
		t.Fatal(err)
	}
	readEnd, writeEnd, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("KANDEV_PARENT_PIPE_FD", fmt.Sprintf("%d", readEnd.Fd()))
	t.Cleanup(func() {
		_ = readEnd.Close()
		_ = writeEnd.Close()
	})
	parentClosed := monitorParentLiveness(log)
	if parentClosed == nil {
		t.Fatal("parent liveness monitor was not armed")
	}
	cleaned := make(chan struct{})
	go waitForShutdown(log, parentClosed, nil, func(context.Context) {
		close(cleaned)
	})
	if err := writeEnd.Close(); err != nil {
		t.Fatal(err)
	}

	select {
	case <-cleaned:
	case <-time.After(3 * time.Second):
		t.Fatal("parent-liveness closure did not run cleanup")
	}

	var closureEntry *observer.LoggedEntry
	for _, entry := range logs.All() {
		if strings.Contains(entry.Message, "parent") {
			closureEntry = &entry
		}
	}
	if closureEntry == nil {
		t.Fatal("parent-liveness closure was not logged")
	}
	if !strings.Contains(closureEntry.Message, "pipe closed") {
		t.Fatalf("closure message = %q, want the observed pipe closure", closureEntry.Message)
	}
	if strings.Contains(strings.ToLower(closureEntry.Message), "died") {
		t.Fatalf("closure message = %q, must not claim the parent process died", closureEntry.Message)
	}
	if closureEntry.Level != zapcore.WarnLevel {
		t.Fatalf("closure level = %v, want warn", closureEntry.Level)
	}
}
