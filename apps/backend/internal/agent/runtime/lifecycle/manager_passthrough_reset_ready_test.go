package lifecycle

import (
	"context"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func newPassthroughResetReadyTest(t *testing.T, command string) (*Manager, *process.InteractiveRunner, *AgentExecution) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell prompt output")
	}
	mgr, runner, execution := newPassthroughRunnerManager(t)
	const agentID = "reset-ready-test-agent"
	require.NoError(t, mgr.registry.Register(&testAgent{
		id: agentID, enabled: true,
		StandardPassthrough: agents.StandardPassthrough{Cfg: agents.PassthroughConfig{
			Supported: true, PassthroughCmd: agents.NewCommand("sh", "-c", command),
			PromptPattern: "READY> $", IdleTimeout: time.Minute, SubmitSequence: "\r",
		}},
	}))
	mgr.profileResolver = &mockPassthroughProfileResolver{agentName: agentID, cliPassthrough: true}
	execution.AgentID = agentID
	execution.Status = v1.AgentStatusReady
	t.Cleanup(func() {
		_ = runner.Stop(context.Background(), execution.PassthroughProcessID)
	})
	return mgr, runner, execution
}

func TestRestartPassthroughProcessSettlesStartupBeforeNextTurn(t *testing.T) {
	mgr, runner, execution := newPassthroughResetReadyTest(t, "printf 'READY> '; read -r input")
	var startupSettled atomic.Bool
	runner.SetTurnCompleteCallback(func(sessionID, processID string) {
		mgr.handlePassthroughTurnComplete(sessionID, processID)
		startupSettled.Store(true)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	require.NoError(t, mgr.restartPassthroughProcess(ctx, execution))
	require.True(t, startupSettled.Load(), "reset must settle boot readiness before admitting a prompt")
	require.NoError(t, mgr.MarkPassthroughRunning(execution.SessionID))
	mgr.handlePassthroughTurnComplete(execution.SessionID, execution.PassthroughProcessID)
	require.Equal(t, v1.AgentStatusReady, execution.Status)
	bus := mgr.eventBus.(*MockEventBus)
	var readyCount int
	for _, event := range bus.PublishedEvents {
		if event.Type == events.AgentReady {
			readyCount++
		}
	}
	require.Equal(t, 1, readyCount, "the next turn must retain its completion event")
}

func TestRestartPassthroughProcessCancelsStartupWait(t *testing.T) {
	mgr, _, execution := newPassthroughResetReadyTest(t, "read -r input")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	bus := mgr.eventBus.(*MockEventBus)
	bus.OnPublish = func(subject string, _ *eventbus.Event) {
		if subject == events.AgentContextReset {
			cancel()
		}
	}

	err := mgr.restartPassthroughProcess(ctx, execution)

	require.ErrorIs(t, err, context.Canceled)
}
