package buffer

import (
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestRingBuffer_PushAndSnapshot(t *testing.T) {
	b := New(3)
	for i := 0; i < 3; i++ {
		b.Push(Entry{Message: string(rune('a' + i))})
	}
	snap := b.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(snap))
	}
	for i, want := range []string{"a", "b", "c"} {
		if snap[i].Message != want {
			t.Errorf("entry %d: got %q, want %q", i, snap[i].Message, want)
		}
	}
}

func TestRingBuffer_FIFOEvictionAtCapacity(t *testing.T) {
	b := New(3)
	for i := 0; i < 5; i++ {
		b.Push(Entry{Message: string(rune('a' + i))})
	}
	snap := b.Snapshot()
	if len(snap) != 3 {
		t.Fatalf("expected 3 entries after eviction, got %d", len(snap))
	}
	for i, want := range []string{"c", "d", "e"} {
		if snap[i].Message != want {
			t.Errorf("entry %d: got %q, want %q", i, snap[i].Message, want)
		}
	}
}

func TestRingBuffer_DefaultCapacity(t *testing.T) {
	b := New(0)
	if b.capacity != DefaultCapacity {
		t.Fatalf("non-positive capacity should default to %d, got %d", DefaultCapacity, b.capacity)
	}
}

func TestRingBuffer_SnapshotIsolation(t *testing.T) {
	b := New(5)
	b.Push(Entry{Message: "first"})
	snap := b.Snapshot()
	snap[0].Message = "mutated"
	if got := b.Snapshot()[0].Message; got != "first" {
		t.Fatalf("snapshot mutation leaked: got %q", got)
	}
}

func TestRingBuffer_ConcurrentWrites(t *testing.T) {
	const writers = 8
	const perWriter = 200
	b := New(writers * perWriter)
	var wg sync.WaitGroup
	wg.Add(writers)
	for w := 0; w < writers; w++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perWriter; i++ {
				b.Push(Entry{Timestamp: time.Now(), Message: "x"})
			}
		}()
	}
	wg.Wait()
	if got := b.Len(); got != writers*perWriter {
		t.Fatalf("expected %d entries, got %d", writers*perWriter, got)
	}
}

func TestRingBuffer_Clear(t *testing.T) {
	b := New(3)
	b.Push(Entry{Message: "a"})
	b.Push(Entry{Message: "b"})
	b.Clear()
	if got := b.Len(); got != 0 {
		t.Fatalf("expected 0 entries after clear, got %d", got)
	}
}

func TestDefault_ReturnsSameInstance(t *testing.T) {
	first := Default()
	second := Default()
	if first != second {
		t.Fatal("Default() should return the same singleton")
	}
}

func TestCore_CapturesEntriesWithFields(t *testing.T) {
	buf := New(10)
	core := NewCore(buf, zapcore.DebugLevel)
	logger := zap.New(core).With(zap.String("service", "test"))

	logger.Info("hello", zap.Int("count", 42))
	logger.Warn("careful")

	snap := buf.Snapshot()
	if len(snap) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(snap))
	}
	if snap[0].Message != "hello" || snap[0].Level != "info" {
		t.Errorf("entry 0: got message=%q level=%q", snap[0].Message, snap[0].Level)
	}
	if got, _ := snap[0].Fields["service"].(string); got != "test" {
		t.Errorf("entry 0 service field: got %v", snap[0].Fields["service"])
	}
	if got, _ := snap[0].Fields["count"].(int64); got != 42 {
		t.Errorf("entry 0 count field: got %v", snap[0].Fields["count"])
	}
	if snap[1].Level != "warn" {
		t.Errorf("entry 1 level: got %q", snap[1].Level)
	}
}

func TestRingBuffer_SnapshotDeepCopiesFields(t *testing.T) {
	b := New(3)
	b.Push(Entry{Message: "m", Fields: map[string]any{"k": "v1"}})

	snap1 := b.Snapshot()
	snap1[0].Fields["k"] = "mutated"
	snap1[0].Fields["new"] = "added"

	snap2 := b.Snapshot()
	if got := snap2[0].Fields["k"]; got != "v1" {
		t.Errorf("buffered field mutated: got %v", got)
	}
	if _, ok := snap2[0].Fields["new"]; ok {
		t.Errorf("buffered field map had extra key added")
	}
}

func TestCore_RespectsLevelEnabler(t *testing.T) {
	buf := New(10)
	core := NewCore(buf, zapcore.WarnLevel)
	logger := zap.New(core)

	logger.Debug("nope")
	logger.Info("nope")
	logger.Warn("yes")
	logger.Error("yes")

	if got := buf.Len(); got != 2 {
		t.Fatalf("expected only warn+error to be captured, got %d entries", got)
	}
}
