package logs

import (
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger/buffer"
)

func TestTailFromBuffer_NilBufferReturnsEmpty(t *testing.T) {
	svc := &Service{}
	if got := svc.tailFromBuffer(5); len(got) != 0 {
		t.Fatalf("len = %d, want 0", len(got))
	}
}

func TestTailFromBuffer_ReturnsLastNLines(t *testing.T) {
	rb := buffer.New(10)
	for i := 1; i <= 5; i++ {
		rb.Push(buffer.Entry{
			Timestamp: time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC),
			Level:     "info",
			Message:   "line",
		})
	}
	svc := &Service{memBuffer: rb}
	lines := svc.tailFromBuffer(2)
	if len(lines) != 2 {
		t.Fatalf("len(lines) = %d, want 2", len(lines))
	}
	if lines[0] != "2026-01-01T00:00:04.000Z INFO line" {
		t.Errorf("lines[0] = %q", lines[0])
	}
	if lines[1] != "2026-01-01T00:00:05.000Z INFO line" {
		t.Errorf("lines[1] = %q", lines[1])
	}
}

func TestFormatBufferEntry_IncludesCallerFieldsAndStack(t *testing.T) {
	ts := time.Date(2026, 5, 1, 12, 30, 0, 0, time.UTC)
	line := formatBufferEntry(buffer.Entry{
		Timestamp: ts,
		Level:     "warn",
		Caller:    "main.go:42",
		Message:   "boom",
		Fields:    map[string]any{"count": 3, "name": "alpha"},
		Stack:     "trace...",
	})
	if line != "2026-05-01T12:30:00.000Z WARN main.go:42 boom count=3 name=alpha stack=..." {
		t.Errorf("formatBufferEntry = %q", line)
	}
}

func TestFormatFieldValue_StringErrorAndJSON(t *testing.T) {
	if got := formatFieldValue("plain"); got != "plain" {
		t.Errorf("string = %q", got)
	}
	errVal := errors.New("boom")
	if got := formatFieldValue(errVal); got != "boom" {
		t.Errorf("error = %q", got)
	}
	if got := formatFieldValue(map[string]int{"a": 1}); got != `{"a":1}` {
		t.Errorf("json = %q", got)
	}
}
