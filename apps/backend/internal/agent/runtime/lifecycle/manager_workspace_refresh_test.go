package lifecycle

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

func TestFinishExecutionWorkspaceActivityRefreshesBeforePausingRuntime(t *testing.T) {
	events := make(chan string, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/workspace/poll-mode":
			var body struct {
				Mode string `json:"mode"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			events <- r.URL.Path + ":" + body.Mode
		case "/api/v1/workspace/refresh":
			events <- r.URL.Path
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	mgr := newTestManagerForAggregator(t)
	port := srv.Listener.Addr().(*net.TCPAddr).Port
	client := agentctl.NewClient("127.0.0.1", port, newTestLogger())
	t.Cleanup(func() { client.Close() })
	execution := &AgentExecution{
		ID: "exec-s1", SessionID: "s1", TaskID: "task-1", WorkspacePath: "/tmp/ws1", agentctl: client,
	}
	if err := mgr.executionStore.Add(execution); err != nil {
		t.Fatalf("add execution: %v", err)
	}

	mgr.setRuntimeInterest("s1", true)
	select {
	case got := <-events:
		if got != "/api/v1/workspace/poll-mode:slow" {
			t.Fatalf("initial runtime event = %q, want slow poll mode", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for initial runtime poll mode")
	}

	mgr.finishExecutionWorkspaceActivity(execution, "turn_complete")
	first := eventsWithTimeout(t, events)
	second := eventsWithTimeout(t, events)
	if first != "/api/v1/workspace/refresh" {
		t.Fatalf("first completion event = %q, want final refresh", first)
	}
	if second != "/api/v1/workspace/poll-mode:paused" {
		t.Fatalf("second completion event = %q, want paused poll mode", second)
	}
}

func eventsWithTimeout(t *testing.T, events <-chan string) string {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	select {
	case event := <-events:
		return event
	case <-timer.C:
		return ""
	}
}
