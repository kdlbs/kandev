package backendapp

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestExecutionObservationRefreshWakeAndShutdown(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		wakes := make(chan struct{}, 1)
		done := make(chan struct{})
		var calls atomic.Int64
		go func() {
			defer close(done)
			runExecutionObserver(ctx, func(context.Context) { calls.Add(1) }, wakes)
		}()
		synctest.Wait()
		require.EqualValues(t, 1, calls.Load(), "initial refresh must run without a timer or client")
		wakes <- struct{}{}
		synctest.Wait()
		require.EqualValues(t, 2, calls.Load(), "capture wake must refresh immediately")
		time.Sleep(10 * time.Second)
		synctest.Wait()
		require.EqualValues(t, 3, calls.Load(), "evidence must refresh even without client activity")
		cancel()
		<-done
		time.Sleep(20 * time.Second)
		synctest.Wait()
		require.EqualValues(t, 3, calls.Load(), "shutdown must cancel the timer and join the loop")
	})
}
