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

func TestStripANSI(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no ANSI codes",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
		{
			name:     "simple color code",
			input:    "\x1b[31mred text\x1b[0m",
			expected: "red text",
		},
		{
			name:     "bold text",
			input:    "\x1b[1mbold\x1b[0m",
			expected: "bold",
		},
		{
			name:     "multiple colors",
			input:    "\x1b[31mred\x1b[32mgreen\x1b[34mblue\x1b[0m",
			expected: "redgreenblue",
		},
		{
			name:     "256 color code",
			input:    "\x1b[38;5;196mcolored\x1b[0m",
			expected: "colored",
		},
		{
			name:     "RGB color code",
			input:    "\x1b[38;2;255;0;0mred\x1b[0m",
			expected: "red",
		},
		{
			name:     "cursor movement",
			input:    "\x1b[2Amove up\x1b[3Bmove down",
			expected: "move upmove down",
		},
		{
			name:     "clear line",
			input:    "text\x1b[2Kcleared",
			expected: "textcleared",
		},
		{
			name:     "real world npm output",
			input:    "\x1b[32m✓\x1b[39m \x1b[90mCompiled successfully\x1b[39m",
			expected: "✓ Compiled successfully",
		},
		{
			name:     "mixed with newlines",
			input:    "\x1b[31mError:\x1b[0m\nSomething failed\n\x1b[33mWarning:\x1b[0m check logs",
			expected: "Error:\nSomething failed\nWarning: check logs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stripANSI(tt.input)
			if result != tt.expected {
				t.Errorf("stripANSI(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
