package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
	"github.com/stretchr/testify/require"
)

type delayedRemoteCursorWriter struct {
	captureExecutorRunningWriter
	entered chan struct{}
	release chan struct{}
	reads   atomic.Int32
}

func (w *delayedRemoteCursorWriter) GetExecutorRunningBySessionID(context.Context, string) (*models.ExecutorRunning, error) {
	if w.reads.Add(1) == 1 {
		close(w.entered)
		<-w.release
	}
	return nil, errors.New("cursor persistence unavailable")
}

func TestSSHPassthroughCursorPersistenceReleasesGuardsAfterReadFailure(t *testing.T) {
	mgr, _, execution := newPassthroughRunnerManager(t)
	sshPassthroughTestController(t, execution)
	mgr.executorRegistry.Register(&MockExecutor{name: executor.NameSSH})
	target := agentctltypes.PassthroughTarget{ProcessID: "remote-process", Generation: 1}
	execution.PassthroughProcessID = target.ProcessID
	execution.setMetadataValue(metadataKeySSHPassthroughState, remotePassthroughRecord{PassthroughTarget: target, InstanceID: execution.ID})
	writer := &delayedRemoteCursorWriter{entered: make(chan struct{}), release: make(chan struct{})}
	mgr.runningWriter = writer
	releasePersistence := sync.OnceFunc(func() { close(writer.release) })
	t.Cleanup(releasePersistence)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := (&websocket.Upgrader{}).Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		_ = conn.WriteJSON(agentctltypes.PassthroughEventPage{Sequence: 1, Events: []agentctltypes.PassthroughEvent{{PassthroughTarget: target, InstanceID: execution.ID, SessionID: execution.SessionID, Sequence: 1, Kind: "started"}}})
	}))
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), nil)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	client := execution.agentctl
	consumed := make(chan uint64, 1)
	go func() {
		consumed <- mgr.consumeRemotePassthroughEvents(conn, execution, client, agentctltypes.PassthroughSnapshot{PassthroughTarget: target, InstanceID: execution.ID, SessionID: execution.SessionID}, 0)
	}()
	select {
	case <-writer.entered:
	case <-time.After(lockOrderDeadlockGuard):
		t.Fatal("event did not reach cursor persistence")
	}
	cleaned := make(chan error, 1)
	go func() { cleaned <- mgr.cleanupStaleExecution(context.Background(), execution) }()
	require.Eventually(t, func() bool { return goroutineParkedIn("cleanupStaleExecution") }, lockOrderDeadlockGuard, time.Millisecond)
	sourceAvailable := execution.agentctlSourceMu.TryLock()
	if sourceAvailable {
		execution.agentctlSourceMu.Unlock()
	}
	releasePersistence()
	require.True(t, sourceAvailable, "cursor persistence must not hold the client source guard")
	require.NoError(t, <-cleaned)
	select {
	case after := <-consumed:
		require.Equal(t, uint64(1), after)
	case <-time.After(lockOrderDeadlockGuard):
		t.Fatal("event consumer did not finish")
	}
	_, exists := mgr.executionStore.Get(execution.ID)
	require.False(t, exists)
	require.Nil(t, writer.running, "failed cursor writes must not recreate a removed execution")
	require.Zero(t, remotePassthroughRecordFor(execution).EventSequence, "failed saves must not advance the durable cursor projection")
}

func TestSSHPassthroughEventsRequireCurrentExecutionAndTarget(t *testing.T) {
	for _, mismatch := range []string{"none", "controller", "process", "generation", "execution"} {
		t.Run(mismatch, func(t *testing.T) {
			mgr, _, execution := newPassthroughRunnerManager(t)
			target := agentctltypes.PassthroughTarget{ProcessID: "current-process", Generation: 3}
			execution.PassthroughProcessID = target.ProcessID
			execution.Status = v1.AgentStatusReady
			execution.setMetadataValue(metadataKeySSHPassthroughState, remotePassthroughRecord{PassthroughTarget: target, InstanceID: "current-controller", EventSequence: 1})
			writer := &captureExecutorRunningWriter{}
			mgr.runningWriter = writer
			event := agentctltypes.PassthroughEvent{PassthroughTarget: target, InstanceID: "current-controller", SessionID: execution.SessionID, Sequence: 2, Kind: remotePassthroughSubmitted}
			switch mismatch {
			case "controller":
				event.InstanceID = "old-controller"
			case "process":
				event.ProcessID = "old-process"
			case "generation":
				event.Generation--
			case "execution":
				mgr.executionStore.Remove(execution.ID)
				require.NoError(t, mgr.executionStore.Add(&AgentExecution{ID: "replacement", SessionID: execution.SessionID, Status: v1.AgentStatusReady}))
			}
			mgr.handleRemotePassthroughEvent(execution, event)
			if mismatch == "none" {
				require.Equal(t, uint64(2), remotePassthroughRecordFor(execution).EventSequence)
				require.Equal(t, v1.AgentStatusRunning, execution.Status)
				require.NotNil(t, writer.running)
			} else {
				require.Equal(t, uint64(1), remotePassthroughRecordFor(execution).EventSequence)
				require.Equal(t, v1.AgentStatusReady, execution.Status)
				require.Nil(t, writer.running)
			}
		})
	}
}
