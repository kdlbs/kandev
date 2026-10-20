package lifecycle

import (
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/stretchr/testify/require"
)

func TestSSHPassthroughCursorRevalidatesAfterRuntimeLease(t *testing.T) {
	for _, changed := range []string{"none", "execution", "generation", "controller", "cursor"} {
		t.Run(changed, func(t *testing.T) {
			mgr, _, execution := newPassthroughRunnerManager(t)
			execution.ExecutorType, execution.RuntimeName = executorTypeSSH, executor.NameSSH
			target := agentctltypes.PassthroughTarget{ProcessID: "current-process", Generation: 1}
			execution.PassthroughProcessID = target.ProcessID
			record := remotePassthroughRecord{PassthroughTarget: target, InstanceID: "controller", EventSequence: 1}
			execution.setMetadataValue(metadataKeySSHPassthroughState, record)
			writer := &captureExecutorRunningWriter{}
			mgr.runningWriter = writer
			execution.remoteInstanceLifecycleMu.Lock()
			release := sync.OnceFunc(execution.remoteInstanceLifecycleMu.Unlock)
			t.Cleanup(release)
			type outcome struct {
				accepted bool
				err      error
			}
			done := make(chan outcome, 1)
			go func() {
				accepted, err := mgr.advanceRemotePassthroughCursor(t.Context(), execution, target, "controller", 2)
				done <- outcome{accepted, err}
			}()
			require.Eventually(t, func() bool { return goroutineParkedIn("advanceRemotePassthroughCursor") }, lockOrderDeadlockGuard, time.Millisecond)
			switch changed {
			case "execution":
				mgr.executionStore.Remove(execution.ID)
				require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "replacement", SessionID: execution.SessionID}))
			case "generation":
				record.Generation++
			case "controller":
				record.InstanceID = "replacement-controller"
			case "cursor":
				record.EventSequence = 3
			}
			execution.setMetadataValue(metadataKeySSHPassthroughState, record)
			release()
			var result outcome
			select {
			case result = <-done:
			case <-time.After(lockOrderDeadlockGuard):
				t.Fatal("cursor save did not release its runtime lease")
			}
			require.NoError(t, result.err)
			if changed == "none" {
				require.True(t, result.accepted)
				require.NotNil(t, writer.running)
				require.Equal(t, uint64(2), remotePassthroughRecordFor(execution).EventSequence)
			} else {
				require.False(t, result.accepted)
				require.Nil(t, writer.running)
				require.Equal(t, record, remotePassthroughRecordFor(execution))
			}
		})
	}
}
