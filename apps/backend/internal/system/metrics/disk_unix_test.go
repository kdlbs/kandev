//go:build !windows

package metrics

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"
)

func TestDiskPercentReturnsWhenContextCancelsWhileStatfsBlocks(t *testing.T) {
	original := statfs
	block := make(chan struct{})
	started := make(chan struct{})
	statfs = func(_ string, _ *syscall.Statfs_t) error {
		close(started)
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
}
