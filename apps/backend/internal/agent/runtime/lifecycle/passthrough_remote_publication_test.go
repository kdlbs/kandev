package lifecycle

import (
	"context"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/events"
	eventbus "github.com/kandev/kandev/internal/events/bus"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

func TestSSHPassthroughPublicationAllowsRuntimeCleanup(t *testing.T) {
	for _, kind := range []string{"idle", "submitted", "status"} {
		t.Run(kind, func(t *testing.T) {
			mgr, _, execution := newPassthroughRunnerManager(t)
			sshPassthroughTestController(t, execution)
			mgr.executorRegistry.Register(&MockExecutor{name: executor.NameSSH})
			execution.Status = v1.AgentStatusReady
			subject := events.AgentRunning
			if kind == "idle" {
				execution.Status, subject = v1.AgentStatusRunning, events.AgentReady
			}
			if kind == "status" {
				subject = events.BuildProcessStatusSubject(execution.SessionID)
			}
			target := agentctltypes.PassthroughTarget{ProcessID: "process", Generation: 1}
			execution.PassthroughProcessID = target.ProcessID
			execution.setMetadataValue(metadataKeySSHPassthroughState, remotePassthroughRecord{PassthroughTarget: target, InstanceID: execution.ID})
			memoryBus := eventbus.NewMemoryEventBus(mgr.logger)
			t.Cleanup(memoryBus.Close)
			mgr.eventBus, mgr.eventPublisher = memoryBus, NewEventPublisher(memoryBus, mgr.logger)
			type outcome struct {
				cursor uint64
				err    error
			}
			received := make(chan outcome, 1)
			_, err := memoryBus.Subscribe(subject, func(context.Context, *eventbus.Event) error {
				cursor := remotePassthroughRecordFor(execution).EventSequence
				err := mgr.BindResumeAttempt(t.Context(), execution.SessionID, "subscriber-resume")
				if err == nil {
					err = mgr.cleanupStaleExecution(t.Context(), execution)
				}
				received <- outcome{cursor, err}
				return err
			})
			require.NoError(t, err)
			done := make(chan struct{})
			go func() {
				defer close(done)
				mgr.handleRemotePassthroughEvent(execution, agentctltypes.PassthroughEvent{PassthroughTarget: target, InstanceID: execution.ID, SessionID: execution.SessionID, Sequence: 1, Kind: kind, Status: agentctltypes.ProcessStatusRunning})
			}()
			select {
			case result := <-received:
				require.NoError(t, result.err)
				require.Equal(t, uint64(1), result.cursor, "cursor must be saved before a subscriber can retire the runtime")
			case <-time.After(lockOrderDeadlockGuard):
				t.Fatal("event publication held a runtime guard needed by cleanup")
			}
			select {
			case <-done:
			case <-time.After(lockOrderDeadlockGuard):
				t.Fatal("event projection did not finish after subscriber cleanup")
			}
			_, exists := mgr.executionStore.Get(execution.ID)
			require.False(t, exists)
		})
	}
}
