//go:build !windows

package metrics

import (
	"context"
	"errors"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

func TestDiskPercentReturnsWhenContextCancelsWhileStatfsBlocks(t *testing.T) {
	original := statfs
	block := make(chan struct{})
	started := make(chan struct{})
	var calls atomic.Int32
	statfs = func(_ string, _ *syscall.Statfs_t) error {
		if calls.Add(1) == 1 {
			close(started)
		}
		<-block
		return errors.New("unblocked")
	}
	t.Cleanup(func() {
		close(block)
		statfs = original
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := diskPercent(ctx, "/slow")
		done <- err
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("statfs was not called")
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
	for range 20 {
		requestContext, requestCancel := context.WithCancel(context.Background())
		requestCancel()
		if _, err := DiskUsage(requestContext, "/still-slow"); !errors.Is(err, context.Canceled) {
			t.Fatalf("repeated DiskUsage error = %v, want context.Canceled", err)
		}
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("blocked statfs calls = %d, want one admitted call", got)
	}
}

func TestDiskUsageReturnsCapacityFields(t *testing.T) {
	original := statfs
	statfs = func(_ string, stat *syscall.Statfs_t) error {
		*stat = syscall.Statfs_t{Blocks: 1000, Bsize: 1, Bavail: 250}
		return nil
	}
	t.Cleanup(func() { statfs = original })

	capacity, err := DiskUsage(context.Background(), "/data")
	if err != nil {
		t.Fatalf("DiskUsage: %v", err)
	}
	if capacity.TotalBytes != 1000 || capacity.AvailableBytes != 250 || capacity.UsedBytes != 750 || capacity.UsedPercent != 75 {
		t.Fatalf("capacity = %#v, want total=1000 available=250 used=750 percent=75", capacity)
	}
}
