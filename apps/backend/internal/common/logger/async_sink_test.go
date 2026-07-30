package logger

import (
	"context"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zapcore"
)

type blockingWriter struct {
	once    sync.Once
	started chan struct{}
	release chan struct{}
}

func (w *blockingWriter) Write(data []byte) (int, error) {
	w.once.Do(func() { close(w.started) })
	<-w.release
	return len(data), nil
}

func TestAsyncSinkNeverWaitsForBlockedWriterAndReservesWarnings(t *testing.T) {
	writer := &blockingWriter{started: make(chan struct{}), release: make(chan struct{})}
	sink := newAsyncSink(writer, asyncSinkConfig{
		Name: "test", MaxEntries: 3, MaxBytes: 30,
		ReservedEntries: 1, ReservedBytes: 10, MaxEntryBytes: 20,
	})
	t.Cleanup(func() {
		close(writer.release)
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = sink.Close(ctx)
	})

	if !sink.Enqueue(zapcore.DebugLevel, []byte("first")) {
		t.Fatal("first debug entry was rejected")
	}
	select {
	case <-writer.started:
	case <-time.After(time.Second):
		t.Fatal("writer did not start")
	}
	if !sink.Enqueue(zapcore.InfoLevel, []byte("second")) {
		t.Fatal("second low-priority entry was rejected")
	}

	started := time.Now()
	if sink.Enqueue(zapcore.InfoLevel, []byte("third")) {
		t.Fatal("low-priority entry consumed reserved capacity")
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("enqueue waited for blocked writer: %s", elapsed)
	}
	if !sink.Enqueue(zapcore.WarnLevel, []byte("warning")) {
		t.Fatal("warning did not use reserved capacity")
	}
	if sink.Enqueue(zapcore.ErrorLevel, []byte("overflow")) {
		t.Fatal("entry beyond total capacity was accepted")
	}

	stats := sink.Stats()
	if got := stats.Lost["info"]["reserved_capacity"]; got != 1 {
		t.Fatalf("reserved-capacity info loss = %d", got)
	}
	if got := stats.Lost["error"]["capacity"]; got != 1 {
		t.Fatalf("capacity error loss = %d", got)
	}
}

func TestAsyncSinkDropsOversizedEntry(t *testing.T) {
	sink := newAsyncSink(discardWriter{}, asyncSinkConfig{
		Name: "test", MaxEntries: 3, MaxBytes: 30, MaxEntryBytes: 4,
	})
	if sink.Enqueue(zapcore.ErrorLevel, []byte("12345")) {
		t.Fatal("oversized entry was accepted")
	}
	if got := sink.Stats().Lost["error"]["entry_too_large"]; got != 1 {
		t.Fatalf("oversized loss = %d", got)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_ = sink.Close(ctx)
}

func TestAsyncSinkCloseHonorsDeadlineAndAccountsForPendingEntry(t *testing.T) {
	writer := &blockingWriter{started: make(chan struct{}), release: make(chan struct{})}
	sink := newAsyncSink(writer, asyncSinkConfig{
		Name: "test", MaxEntries: 3, MaxBytes: 30, MaxEntryBytes: 20,
	})
	if !sink.Enqueue(zapcore.WarnLevel, []byte("warning")) {
		t.Fatal("warning was rejected")
	}
	<-writer.started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	started := time.Now()
	if err := sink.Close(ctx); err == nil {
		t.Fatal("Close succeeded despite blocked writer")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("Close exceeded bounded deadline: %s", elapsed)
	}
	if got := sink.Stats().Lost["warn"]["shutdown_timeout"]; got != 1 {
		t.Fatalf("shutdown-timeout loss = %d", got)
	}
	close(writer.release)
}

type discardWriter struct{}

func (discardWriter) Write(data []byte) (int, error) { return len(data), nil }
