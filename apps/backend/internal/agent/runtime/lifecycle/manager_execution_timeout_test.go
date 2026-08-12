package lifecycle

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/kandev/kandev/internal/common/constants"
)

func TestCoalescedExecutionAllowsSetupPastOldDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		mgr := &Manager{stopCh: make(chan struct{})}
		result := make(chan error, 1)
		go func() {
			_, err := mgr.doCoalescedExecution(context.Background(), "session-setup", func(ctx context.Context) (interface{}, error) {
				select {
				case <-time.After(90 * time.Second):
					return struct{}{}, nil
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			})
			result <- err
		}()

		if err := <-result; err != nil {
			t.Fatalf("setup crossing old deadline failed: %v", err)
		}
		if constants.AgentLaunchTimeout <= 90*time.Second {
			t.Fatalf("AgentLaunchTimeout = %s, want more than 90s", constants.AgentLaunchTimeout)
		}
	})
}
