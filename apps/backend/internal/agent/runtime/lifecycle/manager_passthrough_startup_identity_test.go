package lifecycle

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

type promotedPassthroughTestAgent struct{ *testAgent }

func (a *promotedPassthroughTestAgent) BuildCommand(agents.CommandOptions) agents.Command {
	return a.Cfg.PassthroughCmd
}

func TestPromotedPassthroughResumePublishesAdmittedIdentity(t *testing.T) {
	mgr, runner, execution := newPassthroughResetReadyTest(t,
		`printf 'READY> '; while IFS= read -r input; do printf 'Processed: %s\nREADY> ' "$input"; done`)
	agent, ok := mgr.registry.Get(execution.AgentID)
	require.True(t, ok)
	require.NoError(t, mgr.registry.Unregister(execution.AgentID))
	require.NoError(t, mgr.registry.Register(&promotedPassthroughTestAgent{agent.(*testAgent)}))
	settled := make(chan struct{}, 4)
	runner.SetTurnCompleteCallback(func(sessionID, processID string) {
		mgr.handlePassthroughTurnComplete(sessionID, processID)
		settled <- struct{}{}
	})
	ready := make(chan AgentEventPayload, 4)
	mgr.eventBus.(*MockEventBus).OnPublish = func(subject string, event *eventbus.Event) {
		if subject == events.AgentReady {
			ready <- event.Data.(AgentEventPayload)
		}
	}
	execution.AgentCommand = ""
	execution.IsPassthrough = false
	execution.Status = v1.AgentStatusStarting
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctx = WithResumeAttemptID(ctx, "resume-promoted")
	promoted, err := mgr.Launch(ctx, &LaunchRequest{
		TaskID: execution.TaskID, SessionID: execution.SessionID,
		AgentProfileID: execution.AgentProfileID, PreviousExecutionID: "previous",
		IsPassthrough: true, ResumePassthrough: true, TaskScope: TaskLaunchScopeTask,
	})
	require.NoError(t, err)
	require.Same(t, execution, promoted)
	require.NoError(t, mgr.StartAgentProcess(ctx, execution.ID))
	assertReady := func() {
		t.Helper()
		select {
		case payload := <-ready:
			require.Equal(t, "resume-promoted", payload.AttemptID)
			require.Equal(t, execution.SessionID, payload.SessionID)
		case <-ctx.Done():
			t.Fatal("passthrough did not publish readiness")
		}
		select {
		case <-settled:
		case <-ctx.Done():
			t.Fatal("passthrough readiness callback did not settle")
		}
	}
	assertReady()
	require.NoError(t, mgr.MarkPassthroughRunning(execution.SessionID))
	require.NoError(t, mgr.WritePassthroughStdin(ctx, execution.SessionID, "wrap-next\r"))
	assertReady()
	chunks, ok := runner.GetBuffer(execution.PassthroughProcessID)
	require.True(t, ok)
	var output strings.Builder
	for _, chunk := range chunks {
		output.WriteString(chunk.Data)
	}
	require.Contains(t, output.String(), "Processed: wrap-next")
}

func TestCancelledPassthroughStartupDoesNotReplaceAttempt(t *testing.T) {
	mgr, _, execution := newPassthroughResetReadyTest(t, "printf 'READY> '; read -r input")
	execution.IsPassthrough = true
	execution.beginStartupAttemptWithID("existing-owner")
	generation := execution.startupAttemptSnapshot()
	ctx, cancel := context.WithCancel(WithResumeAttemptID(context.Background(), "cancelled-owner"))
	cancel()

	require.ErrorIs(t, mgr.StartAgentProcess(ctx, execution.ID), context.Canceled)
	require.Equal(t, generation, execution.startupAttemptSnapshot())
	require.Equal(t, "existing-owner", execution.currentStartupAttemptID())
	require.Empty(t, execution.PassthroughProcessID)
}

type passthroughPublicationContextKey struct{}

type passthroughContextEventBus struct {
	*MockEventBus
	publishedContext context.Context
}

func (b *passthroughContextEventBus) Publish(ctx context.Context, subject string, event *eventbus.Event) error {
	if subject == events.AgentRunning {
		b.publishedContext = ctx
	}
	return b.MockEventBus.Publish(ctx, subject, event)
}

func TestPassthroughRunningPreservesSynchronousPublicationContext(t *testing.T) {
	mgr, _, execution := newPassthroughResetReadyTest(t, "read -r input")
	execution.PassthroughProcessID = "process-ready"
	bus := &passthroughContextEventBus{MockEventBus: &MockEventBus{}}
	mgr.eventBus = bus
	mgr.eventPublisher = NewEventPublisher(bus, mgr.logger)
	ctx := context.WithValue(context.Background(), passthroughPublicationContextKey{}, "owner")

	require.NoError(t, mgr.MarkPassthroughRunningWithContext(ctx, execution.SessionID))
	require.NotNil(t, bus.publishedContext)
	require.Equal(t, "owner", bus.publishedContext.Value(passthroughPublicationContextKey{}))
}
