package recorder

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type blockedScanReader struct{ calls atomic.Int32 }

func (r *blockedScanReader) Rebind(query string) string { return query }
func (r *blockedScanReader) SelectContext(ctx context.Context, _ any, _ string, _ ...any) error {
	r.calls.Add(1)
	<-ctx.Done()
	return ctx.Err()
}

func TestScannerOwnsOneDailyReadAndCancelsItOnStop(t *testing.T) {
	reader := &blockedScanReader{}
	s := NewScanner(&Capture{now: time.Now}, reader, nil)
	s.Start(context.Background())
	s.Start(context.Background())
	waitFor(t, "daily read", func() bool { return reader.calls.Load() > 0 })
	stopped := make(chan struct{})
	go func() { s.Stop(); close(stopped) }()
	select {
	case <-stopped:
	case <-time.After(5 * time.Second):
		t.Fatal("stop did not cancel and join daily read")
	}
	if reader.calls.Load() != 1 {
		t.Fatalf("daily reads = %d", reader.calls.Load())
	}
	s.Stop()
	s.Start(context.Background())
	s.wg.Wait()
	if reader.calls.Load() != 1 {
		t.Fatal("stopped scanner read again")
	}
}

func TestScannerStoppedBeforeStartDoesNotRead(t *testing.T) {
	reader := &blockedScanReader{}
	s := NewScanner(&Capture{now: time.Now}, reader, nil)
	s.Stop()
	s.Start(context.Background())
	s.Stop()
	if reader.calls.Load() != 0 {
		t.Fatal("stopped scanner started a read")
	}
}
