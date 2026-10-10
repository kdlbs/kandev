package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTurnCaptureAdmissionWaitCancellationPreservesFence(t *testing.T) {
	for _, reason := range []string{"caller", "shutdown", "timeout"} {
		t.Run(reason, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				execution := &AgentExecution{}
				beginTurnChangeCaptureLocked(execution, 7)
				stop := make(chan struct{})
				manager := &SessionManager{stopCh: stop}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				result := make(chan error, 1)
				go func() { result <- manager.waitForTurnChangeCapture(ctx, execution) }()
				synctest.Wait()
				want := context.Canceled
				switch reason {
				case "caller":
					cancel()
				case "shutdown":
					close(stop)
				case "timeout":
					want = context.DeadlineExceeded
					time.Sleep(turnChangeTerminalCaptureTimeout)
				}
				require.ErrorIs(t, <-result, want)
				require.Equal(t, uint64(7), execution.turnChangeCaptureGeneration)
				select {
				case <-execution.turnChangeCaptureDone:
					t.Fatal("cancelled admission released capture ownership")
				default:
				}
			})
		})
	}
}
