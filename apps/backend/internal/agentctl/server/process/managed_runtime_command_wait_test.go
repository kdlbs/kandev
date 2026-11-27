package process

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agentctl/server/config"
)

func TestManagedRuntimeCommandOutputWaitHonorsCancellation(t *testing.T) {
	finished := make(chan struct{})
	close(finished)
	proc := &PipedProcess{Done: finished}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	manager := NewManager(&config.InstanceConfig{WorkDir: t.TempDir()}, newTestLogger(t))
	t.Cleanup(func() { _ = manager.StopForTeardown(context.Background()) })
	done := make(chan error, 1)
	go func() {
		_, err := manager.waitManagedRuntimeCommand(ctx, proc, make(chan []byte), make(chan []byte))
		done <- err
	}()
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("output wait = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("a pipe inherited by a descendant blocked cancellation")
	}
}
