package handlers

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/stretchr/testify/require"
)

func TestMessageOperationRetentionRunsOnlyOneBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		finish := make(chan struct{})
		h := &Handlers{}
		stop := h.SetSendOperationRetention(context.Background(), func(context.Context) {
			calls.Add(1)
			<-finish
		})
		defer stop()
		for range 32 {
			h.pruneSendOperationsAsync()
		}
		synctest.Wait()
		close(finish)
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load(), "concurrent claims must share one retention batch")
	})
}

func TestMessageOperationRetentionStopCancelsAndDrains(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int32
		finished := make(chan struct{})
		h := &Handlers{}
		stop := h.SetSendOperationRetention(context.Background(), func(ctx context.Context) {
			calls.Add(1)
			<-ctx.Done()
			close(finished)
		})
		h.pruneSendOperationsAsync()
		synctest.Wait()
		stop()
		select {
		case <-finished:
		default:
			t.Fatal("stop returned before the running batch drained")
		}
		for range 32 {
			h.pruneSendOperationsAsync()
		}
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load())
		stop()
	})
}

func TestMessageOperationRetentionUsesBackendLifetime(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var calls atomic.Int32
		finished := make(chan struct{})
		h := &Handlers{}
		stop := h.SetSendOperationRetention(ctx, func(ctx context.Context) {
			calls.Add(1)
			<-ctx.Done()
			close(finished)
		})
		defer stop()
		h.pruneSendOperationsAsync()
		synctest.Wait()
		cancel()
		synctest.Wait()
		select {
		case <-finished:
		default:
			t.Fatal("backend cancellation did not stop its retention batch")
		}
		h.pruneSendOperationsAsync()
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load())
	})
}
