//go:build windows

package metrics

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestDiskPercentReturnsWhenContextCancelsWhileQueryBlocks(t *testing.T) {
	original := getDiskFreeSpaceEx
	block := make(chan struct{})
	started := make(chan struct{})
	getDiskFreeSpaceEx = func(_ *uint16, _ *uint64, _ *uint64, _ *uint64) error {
		close(started)
		<-block
		return errors.New("unblocked")
	}
	t.Cleanup(func() {
		close(block)
		getDiskFreeSpaceEx = original
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := diskPercent(ctx, `C:\`)
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("GetDiskFreeSpaceEx was not called")
	}
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("diskPercent error = %v, want context.Canceled", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("diskPercent did not return after context cancellation")
	}
}

func TestDiskPercentLive(t *testing.T) {
	v, err := diskPercent(context.Background(), `C:\`)
	if err != nil {
		t.Fatalf("diskPercent: %v", err)
	}
	if v < 0 || v > 100 {
		t.Fatalf("disk percent=%v, want 0..100", v)
	}
}

func TestDiskPercentFromBytes(t *testing.T) {
	if _, err := diskPercentFromBytes(0, 0); err == nil {
		t.Fatal("expected error when total is zero")
	}
	v, err := diskPercentFromBytes(100, 25)
	if err != nil {
		t.Fatalf("diskPercentFromBytes: %v", err)
	}
	if v != 75 {
		t.Fatalf("got %v, want 75", v)
	}
}
