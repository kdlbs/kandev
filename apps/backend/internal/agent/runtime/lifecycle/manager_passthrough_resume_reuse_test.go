package lifecycle

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestResumePassthroughSessionKeepsLiveLocalProcess(t *testing.T) {
	mgr, runner, execution := newPassthroughResetReadyTest(t, `printf 'READY> '; while IFS= read -r input; do printf 'Processed: %s\nREADY> ' "$input"; done`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, mgr.ResumePassthroughSession(ctx, execution.SessionID))
	firstID := execution.PassthroughProcessID
	t.Cleanup(func() { _ = runner.Stop(context.Background(), firstID) })
	require.NoError(t, runner.WaitForFirstIdle(ctx, firstID))
	require.NoError(t, mgr.ResumePassthroughSession(ctx, execution.SessionID))
	require.Equal(t, firstID, execution.PassthroughProcessID, "a reconnect must reuse the current live process")
	require.True(t, runner.IsProcessRunning(firstID))
}

func TestPromotedPassthroughResumeAdoptsTerminalReadiness(t *testing.T) {
	mgr, runner, execution := newPassthroughResetReadyTest(t, `printf 'READY> '; while IFS= read -r input; do printf 'Processed: %s\nREADY> ' "$input"; done`)
	agent, ok := mgr.registry.Get(execution.AgentID)
	require.True(t, ok)
	require.NoError(t, mgr.registry.Unregister(execution.AgentID))
	require.NoError(t, mgr.registry.Register(&promotedPassthroughTestAgent{agent.(*testAgent)}))
	runner.SetTurnCompleteCallback(mgr.handlePassthroughTurnComplete)
	execution.AgentCommand = ""
	execution.IsPassthrough = false
	execution.Status = v1.AgentStatusStarting
	execution.beginStartupAttemptWithID("terminal-owner")
	generation := execution.startupAttemptSnapshot()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, mgr.ResumePassthroughSession(ctx, execution.SessionID))
	firstID := execution.PassthroughProcessID
	t.Cleanup(func() { _ = runner.Stop(context.Background(), firstID) })
	require.NoError(t, runner.WaitForFirstIdle(ctx, firstID))
	ready := make(chan AgentEventPayload, 4)
	mgr.eventBus.(*MockEventBus).OnPublish = func(subject string, event *eventbus.Event) {
		if subject == events.AgentReady || subject == events.AgentBootReady {
			ready <- event.Data.(AgentEventPayload)
		}
	}
	ctx = WithResumeAttemptID(ctx, "resume-adopted")
	_, err := mgr.Launch(ctx, &LaunchRequest{
		TaskID: execution.TaskID, SessionID: execution.SessionID, AgentProfileID: execution.AgentProfileID,
		PreviousExecutionID: "previous", IsPassthrough: true, ResumePassthrough: true, StartAgent: true, TaskScope: TaskLaunchScopeTask,
	})
	require.NoError(t, err)
	require.NoError(t, mgr.StartAgentProcess(ctx, execution.ID))
	require.Equal(t, firstID, execution.PassthroughProcessID)
	require.Equal(t, generation, execution.startupAttemptSnapshot())
	require.Empty(t, ready, "adoption must not synthesize a completion event")
	require.True(t, mgr.IsLocalPassthroughReadyForResume(execution.SessionID, execution.ID, "resume-adopted"))
	require.False(t, mgr.IsLocalPassthroughReadyForResume(execution.SessionID, execution.ID, "terminal-owner"))
	require.False(t, mgr.IsLocalPassthroughReadyForResume(execution.SessionID, "other-execution", "resume-adopted"))
	require.NoError(t, mgr.MarkPassthroughRunning(execution.SessionID))
	require.True(t, runner.HasObservedFirstIdle(firstID))
	require.False(t, mgr.IsLocalPassthroughReadyForResume(execution.SessionID, execution.ID, "resume-adopted"), "historical idle must not settle a busy turn")
	require.NoError(t, mgr.WritePassthroughStdin(ctx, execution.SessionID, "next\r"))
	select {
	case payload := <-ready:
		require.Equal(t, "resume-adopted", payload.AttemptID)
	case <-ctx.Done():
		t.Fatal("follow-up did not publish the adopted resume identity")
	}
}

func TestPassthroughFreshStartKeepsPendingProcess(t *testing.T) {
	mgr, runner, execution := newPassthroughResetReadyTest(t, `read -r input; printf 'READY> '; read -r input`)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, mgr.ResumePassthroughSession(ctx, execution.SessionID))
	firstID := execution.PassthroughProcessID
	t.Cleanup(func() { _ = runner.Stop(context.Background(), firstID) })
	execution.beginStartupAttemptWithID("pending-owner")
	require.False(t, runner.HasObservedFirstIdle(firstID))
	require.False(t, mgr.IsLocalPassthroughReadyForResume(execution.SessionID, execution.ID, "pending-owner"))
	profile, err := mgr.profileResolver.ResolveProfile(ctx, execution.AgentProfileID)
	require.NoError(t, err)
	require.NoError(t, mgr.startPassthroughSession(ctx, execution, profile))
	require.Equal(t, firstID, execution.PassthroughProcessID)
	require.NoError(t, runner.Stop(ctx, firstID))
	require.NoError(t, mgr.ResumePassthroughSession(ctx, execution.SessionID))
	require.NotEqual(t, firstID, execution.PassthroughProcessID, "a stopped process must be replaced")
	require.ErrorIs(t, mgr.resumePassthroughSession(ctx, execution.SessionID, firstID), errPassthroughProcessReplaced)
}

func TestPassthroughResumeReadinessDefersContendedProcessLocks(t *testing.T) {
	mgr, _, execution := newPassthroughResetReadyTest(t, "read -r input")
	for _, lock := range []*sync.Mutex{&execution.remoteInstanceLifecycleMu, &execution.passthroughLifecycleMu} {
		lock.Lock()
		done := make(chan bool, 1)
		go func() { done <- mgr.IsLocalPassthroughReadyForResume(execution.SessionID, execution.ID, "attempt") }()
		select {
		case ready := <-done:
			lock.Unlock()
			require.False(t, ready)
		case <-time.After(time.Second):
			lock.Unlock()
			<-done
			t.Fatal("readiness blocked behind process ownership while its caller holds admission")
		}
	}
}

func TestPassthroughAdoptionPreservesGenerationZeroProjection(t *testing.T) {
	mgr, runner, execution := newPassthroughResetReadyTest(t, "printf 'READY> '; read -r input")
	execution.SessionSettingsPolicy = SessionSettingsPolicyStrict
	execution.SessionSettingsProjectionPolicy = SessionSettingsPolicyProviderRestored
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	require.NoError(t, mgr.ResumePassthroughSession(ctx, execution.SessionID))
	require.NoError(t, runner.WaitForFirstIdle(ctx, execution.PassthroughProcessID))
	require.Zero(t, execution.startupAttemptSnapshot())
	require.NoError(t, mgr.bindPassthroughStartupOrBegin(WithResumeAttemptID(ctx, "adopt-zero"), execution, true))
	require.Zero(t, execution.startupAttemptSnapshot(), "adoption must retain the generation owned by the live process")
	require.Equal(t, SessionSettingsPolicyProviderRestored, execution.sessionSettingsProjectionPolicy())
	require.Equal(t, "adopt-zero", execution.currentStartupAttemptID())
}

func TestPassthroughAdoptionRejectsReplacedExecution(t *testing.T) {
	mgr, _, previous := newPassthroughResetReadyTest(t, "read -r input")
	mgr.executionStore.Remove(previous.ID)
	successor := &AgentExecution{ID: "successor", SessionID: previous.SessionID}
	successor.beginStartupAttemptWithID("successor-owner")
	require.NoError(t, mgr.executionStore.Add(successor))
	err := mgr.bindResumeAttemptToExecution(previous, "stale-owner")
	require.ErrorIs(t, err, models.ErrExecutionRotated)
	require.Equal(t, "successor-owner", successor.currentStartupAttemptID())
}

func TestPassthroughStartupRejectsRotatedExecutionAtEachBoundary(t *testing.T) {
	for _, boundary := range []string{"binding", "dispatch"} {
		t.Run(boundary, func(t *testing.T) {
			mgr, runner, previous := newPassthroughResetReadyTest(t, "printf 'READY> '; read -r input")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			require.NoError(t, mgr.ResumePassthroughSession(ctx, previous.SessionID))
			require.NoError(t, runner.WaitForFirstIdle(ctx, previous.PassthroughProcessID))
			ctx = WithResumeAttemptID(ctx, "previous-owner")
			require.NoError(t, mgr.bindPassthroughStartupOrBegin(ctx, previous, true))
			generation := previous.startupAttemptSnapshot()
			previous.isResumedSession = true
			successor := &AgentExecution{ID: "successor", SessionID: previous.SessionID, PassthroughProcessID: previous.PassthroughProcessID}
			successor.beginStartupAttemptWithID("successor-owner")
			mgr.executionStore.Remove(previous.ID)
			require.NoError(t, mgr.executionStore.Add(successor))
			var err error
			if boundary == "binding" {
				err = mgr.bindPassthroughStartupOrBegin(ctx, previous, true)
			} else {
				err = mgr.startPassthroughExecution(ctx, previous, nil)
			}
			require.ErrorIs(t, err, models.ErrExecutionRotated)
			require.Equal(t, generation, previous.startupAttemptSnapshot())
			require.Equal(t, "successor-owner", successor.currentStartupAttemptID())
			require.Equal(t, previous.PassthroughProcessID, successor.PassthroughProcessID)
		})
	}
}
