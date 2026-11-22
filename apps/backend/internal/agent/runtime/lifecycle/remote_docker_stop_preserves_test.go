package lifecycle

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/agent/docker"
)

type recordingDockerEngine struct {
	mu       sync.Mutex
	requests []string
}

func (e *recordingDockerEngine) record(r *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requests = append(e.requests, r.Method+" "+r.URL.Path)
}

func (e *recordingDockerEngine) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.requests...)
}

func newRecordingDockerEngine(t *testing.T) (*recordingDockerEngine, *httptest.Server) {
	t.Helper()
	engine := &recordingDockerEngine{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		engine.record(r)
		switch {
		case strings.HasSuffix(r.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.41", "Arch": "amd64"})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	return engine, server
}

func newRemoteDockerTestExecutor(t *testing.T, server *httptest.Server) *RemoteDockerExecutor {
	t.Helper()
	address := server.Listener.Addr().String()
	client, err := docker.NewRemoteClient(docker.RemoteTransport{
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", address)
		},
	}, newTestDockerLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })

	exec := NewRemoteDockerExecutor(newTestDockerLogger())
	exec.sessions["instance-1"] = &remoteDockerSession{
		dockerClient: client,
		containerMgr: NewContainerManager(client, "", newTestDockerLogger()),
	}
	return exec
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerExecutorPreservesRetainedWorkspace(t *testing.T) {
	for _, tc := range []struct {
		name        string
		reason      string
		agentFailed bool
	}{
		{"recoverable failure", StopReasonRecoverableAgentFailure, false},
		{"reused bootstrap failure", StopReasonAgentBootstrapFailed, false},
		{"unconfirmed agent stop on retained runtime", StopReasonRecoverableAgentFailure, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, server := newRecordingDockerEngine(t)
			exec := newRemoteDockerTestExecutor(t, server)
			instance := &ExecutorInstance{
				InstanceID:      "instance-1",
				TaskID:          "task-1",
				ContainerID:     "container-1",
				StopReason:      tc.reason,
				AgentStopFailed: tc.agentFailed,
			}
			require.NoError(t, exec.StopInstance(context.Background(), instance, false))
			require.Empty(t, engine.snapshot(), "a retained workspace must receive no destroy request")
			require.NotNil(t, instance.ContainerID, "the exact container identity remains the recovery target")
			exec.mu.Lock()
			_, retainedSession := exec.sessions["instance-1"]
			exec.mu.Unlock()
			require.False(t, retainedSession, "transport ownership is released even when compute is retained")
		})
	}
}

// @covers AC-EXECUTORS-REMOTE-DOCKER-001.12
func TestRemoteDockerExecutorRemovesFreshRollbackAndExplicitCleanup(t *testing.T) {
	for _, tc := range []struct {
		name   string
		reason string
	}{
		{"fresh launch rollback", StopReasonLaunchRollback},
		{"archive", StopReasonTaskArchived},
		{"delete", StopReasonTaskDeleted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			engine, server := newRecordingDockerEngine(t)
			exec := newRemoteDockerTestExecutor(t, server)
			instance := &ExecutorInstance{
				InstanceID:  "instance-1",
				TaskID:      "task-1",
				ContainerID: "container-1",
				StopReason:  tc.reason,
			}
			require.NoError(t, exec.StopInstance(context.Background(), instance, true))
			requests := engine.snapshot()
			require.True(t, containsDockerRequest(requests, "POST", "/kill"), "requests = %v", requests)
			require.True(t, containsDockerRequest(requests, "DELETE", "/containers/container-1"), "requests = %v", requests)
		})
	}
}

func containsDockerRequest(requests []string, method, suffix string) bool {
	for _, request := range requests {
		parts := strings.SplitN(request, " ", 2)
		if len(parts) == 2 && parts[0] == method && strings.HasSuffix(parts[1], suffix) {
			return true
		}
	}
	return false
}
