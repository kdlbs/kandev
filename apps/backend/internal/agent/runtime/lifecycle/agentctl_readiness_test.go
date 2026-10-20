package lifecycle

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/stretchr/testify/require"
)

func TestCleanupStaleExecutionDoesNotWaitForReadinessRetry(t *testing.T) {
	for _, stalled := range []bool{false, true} {
		name := "unhealthy"
		if stalled {
			name = "unresponsive"
		}
		t.Run(name, func(t *testing.T) {
			mgr := newTestManager(t)
			defer mgr.closeStopCh()
			probed := make(chan struct{})
			var once sync.Once
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					once.Do(func() { close(probed) })
					if stalled {
						<-r.Context().Done()
						return
					}
				}
				w.WriteHeader(http.StatusServiceUnavailable)
			}))
			defer server.Close()
			client := agentctl.NewClient("127.0.0.1", server.Listener.Addr().(*net.TCPAddr).Port, newTestLogger())
			execution := &AgentExecution{ID: "readiness", SessionID: "readiness-session", agentctl: client}
			require.NoError(t, mgr.executionStore.Add(execution))
			readyDone := make(chan struct{})
			go func() {
				mgr.waitForAgentctlReady(execution)
				close(readyDone)
			}()
			defer func() {
				mgr.closeStopCh()
				<-readyDone
			}()
			select {
			case <-probed:
			case <-time.After(3 * time.Second):
				t.Fatal("readiness did not probe the runtime")
			}
			cleaned := make(chan error, 1)
			go func() {
				cleaned <- mgr.CleanupStaleExecutionBySessionID(context.Background(), execution.SessionID)
			}()
			select {
			case err := <-cleaned:
				require.NoError(t, err)
			case <-time.After(2 * time.Second):
				mgr.closeStopCh()
				<-cleaned
				t.Fatal("cleanup waited for the readiness retry timeout")
			}
			select {
			case <-readyDone:
			case <-time.After(2 * time.Second):
				t.Fatal("readiness continued after the client was detached")
			}
			require.False(t, execution.IsAgentctlReady())
		})
	}
}

func TestAgentctlReadinessPinsOnlyHealthProbe(t *testing.T) {
	probed := make(chan struct{})
	respond := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(probed)
		select {
		case <-respond:
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
		}
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := agentctl.NewClient("127.0.0.1", server.Listener.Addr().(*net.TCPAddr).Port, newTestLogger())
	defer client.Close()
	execution := &AgentExecution{agentctl: client}
	done := make(chan error, 1)
	go func() { done <- execution.waitForAgentctlReady(ctx) }()
	select {
	case <-probed:
	case <-ctx.Done():
		t.Fatal("health probe did not start")
	}
	if execution.agentctlLifecycleMu.TryLock() {
		execution.agentctlLifecycleMu.Unlock()
		t.Fatal("client replacement was allowed during the health request")
	}
	close(respond)
	require.NoError(t, <-done)
	require.True(t, execution.agentctlLifecycleMu.TryLock())
	execution.agentctlLifecycleMu.Unlock()
}
