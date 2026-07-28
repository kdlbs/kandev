package backendapp

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	orchexecutor "github.com/kandev/kandev/internal/orchestrator/executor"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type fakePluginStarter struct {
	started chan string
	err     error
	// blockUntil, when non-nil, holds StartTask open until closed, so a test
	// can prove the adapter returned before the (slow) launch finished.
	blockUntil chan struct{}
}

func (f *fakePluginStarter) StartTask(ctx context.Context, taskID, _, _, _, _, _, _ string, _, _ bool, _ []v1.MessageAttachment) (*orchexecutor.TaskExecution, error) {
	if f.blockUntil != nil {
		select {
		case <-f.blockUntil:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	select {
	case f.started <- taskID:
	default:
	}
	return &orchexecutor.TaskExecution{}, f.err
}

func newStarterAdapter(t *testing.T, orch pluginStarterService) pluginsTaskStarterAdapter {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	require.NoError(t, err)
	return pluginsTaskStarterAdapter{orch: orch, log: log}
}

func TestPluginsStarter_LaunchesAsynchronously(t *testing.T) {
	orch := &fakePluginStarter{started: make(chan string, 1)}
	a := newStarterAdapter(t, orch)

	require.NoError(t, a.StartTask(context.Background(), "task-1"))
	select {
	case id := <-orch.started:
		require.Equal(t, "task-1", id)
	case <-time.After(2 * time.Second):
		t.Fatal("orchestrator StartTask was never invoked by the fire-and-forget goroutine")
	}
}

// TestPluginsStarter_ReturnsBeforeLaunchCompletes proves StartTask does not
// block the caller on the (potentially long) orchestrator launch path.
func TestPluginsStarter_ReturnsBeforeLaunchCompletes(t *testing.T) {
	block := make(chan struct{})
	// Register the release before the goroutine can block on it (and before any
	// t.Fatal path), so an early failure can't strand the launch goroutine
	// until AgentLaunchTimeout. sync.Once makes the explicit release below and
	// this cleanup safe to both run.
	var releaseOnce sync.Once
	release := func() { releaseOnce.Do(func() { close(block) }) }
	t.Cleanup(release)

	orch := &fakePluginStarter{started: make(chan string, 1), blockUntil: block}
	a := newStarterAdapter(t, orch)

	require.NoError(t, a.StartTask(context.Background(), "task-1"), "must return immediately even while the launch is in flight")
	release() // release the launch so the goroutine can finish
	select {
	case <-orch.started:
	case <-time.After(2 * time.Second):
		t.Fatal("launch goroutine never completed after unblock")
	}
}

// TestPluginsStarter_DetachesFromCallerContext proves a cancelled caller
// context does not abort the launch — the task already exists, so the launch
// runs on a detached context.
func TestPluginsStarter_DetachesFromCallerContext(t *testing.T) {
	orch := &fakePluginStarter{started: make(chan string, 1)}
	a := newStarterAdapter(t, orch)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel BEFORE the call
	require.NoError(t, a.StartTask(ctx, "task-1"))
	select {
	case id := <-orch.started:
		require.Equal(t, "task-1", id, "launch proceeds despite the cancelled caller context")
	case <-time.After(2 * time.Second):
		t.Fatal("launch was aborted by the cancelled caller context")
	}
}

func TestPluginsStarter_LaunchErrorIsSwallowed(t *testing.T) {
	orch := &fakePluginStarter{started: make(chan string, 1), err: errors.New("no executor")}
	a := newStarterAdapter(t, orch)

	require.NoError(t, a.StartTask(context.Background(), "task-1"), "a launch error is best-effort, never surfaced")
	select {
	case <-orch.started:
	case <-time.After(2 * time.Second):
		t.Fatal("orchestrator StartTask was never invoked")
	}
}
