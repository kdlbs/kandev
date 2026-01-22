package process

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
)

func newTestLogger(t *testing.T) *logger.Logger {
	log, err := logger.NewLogger(logger.LoggingConfig{
		Level:  "error",
		Format: "json",
	})
	if err != nil {
		t.Fatalf("failed to create logger: %v", err)
	}
	return log
}

func TestRingBufferTrimsOldest(t *testing.T) {
	buffer := newRingBuffer(10)
	buffer.append(ProcessOutputChunk{Stream: "stdout", Data: "hello", Timestamp: time.Now()}) // 5
	buffer.append(ProcessOutputChunk{Stream: "stdout", Data: "world", Timestamp: time.Now()}) // 5 (total 10)
	buffer.append(ProcessOutputChunk{Stream: "stderr", Data: "!!!", Timestamp: time.Now()})   // +3 -> trim

	snapshot := buffer.snapshot()
	if len(snapshot) == 0 {
		t.Fatal("expected buffered output")
	}
	combined := ""
	for _, chunk := range snapshot {
		combined += chunk.Data
	}
	if strings.Contains(combined, "hello") {
		t.Fatalf("expected oldest chunk to be trimmed, got %q", combined)
	}
	if !strings.Contains(combined, "world") {
		t.Fatalf("expected newer chunk to remain, got %q", combined)
	}
}

func TestProcessRunnerCapturesOutput(t *testing.T) {
	log := newTestLogger(t)
	runner := NewProcessRunner(nil, log, 2*1024*1024)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	info, err := runner.Start(ctx, StartProcessRequest{
		SessionID:  "session-1",
		Kind:       "dev",
		Command:    "printf 'hello'; sleep 2",
		WorkingDir: "",
	})
	if err != nil {
		t.Fatalf("failed to start process: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		proc, ok := runner.Get(info.ID, true)
		if !ok {
			time.Sleep(25 * time.Millisecond)
			continue
		}
		combined := ""
		for _, chunk := range proc.Output {
			combined += chunk.Data
		}
		if strings.Contains(combined, "hello") {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatal("process output not captured in time")
}
