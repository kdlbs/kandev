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
	"github.com/kandev/kandev/internal/common/config"
)

type retainedContainerEngine struct {
	mu       sync.Mutex
	requests []string
}

func (e *retainedContainerEngine) record(request *http.Request) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.requests = append(e.requests, request.Method+" "+request.URL.Path)
}

func (e *retainedContainerEngine) snapshot() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.requests...)
}

func newRetainedContainerEngine(t *testing.T) (*retainedContainerEngine, *docker.Client) {
	t.Helper()
	engine := &retainedContainerEngine{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, request *http.Request) {
		engine.record(request)
		switch {
		case strings.HasSuffix(request.URL.Path, "/version"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]string{"ApiVersion": "1.41", "Arch": "amd64"})
		case strings.HasSuffix(request.URL.Path, "/containers/container-retained/json"):
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"Id": "container-retained",
				"State": map[string]interface{}{
					"Status": "running",
				},
				"NetworkSettings": map[string]interface{}{
					"Networks": map[string]interface{}{
						"bridge": map[string]interface{}{"IPAddress": "127.0.0.1"},
					},
				},
			})
		default:
			w.WriteHeader(http.StatusNoContent)
		}
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	address := server.Listener.Addr().String()
	client, err := docker.NewRemoteClient(docker.RemoteTransport{
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp", address)
		},
	}, newTestDockerLogger())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return engine, client
}

func newJournalFailureControl(t *testing.T) (*httptest.Server, *remoteDockerRecoveryEndpoints) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/api/v1/instances", func(w http.ResponseWriter, request *http.Request) {
		switch request.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"instances": []interface{}{}})
		case http.MethodPost:
			http.Error(w, `{"error":"failed to create instance: open delivery journal: lock_timeout"}`, http.StatusInternalServerError)
		default:
			http.NotFound(w, request)
		}
	})
	mux.HandleFunc("/api/v1/instances/", func(w http.ResponseWriter, request *http.Request) {
		http.NotFound(w, request)
	})
	server := httptest.NewServer(mux)
	t.Cleanup(server.Close)
	host, port := parseHTTPURL(t, server.URL)
	require.Equal(t, "127.0.0.1", host)
	endpoints := remoteDockerRecoveryEndpoints{AgentCtlPort: uint16(port)}
	return server, &endpoints
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
func TestDockerCreateInstanceBlocksFreshFallbackAfterJournalFailure(t *testing.T) {
	engine, dockerClient := newRetainedContainerEngine(t)
	_, endpoints := newJournalFailureControl(t)
	exec := NewDockerExecutor(config.DockerConfig{}, "", newTestDockerLogger())
	exec.docker = dockerClient
	exec.containerMgr = NewContainerManager(dockerClient, "", newTestDockerLogger())
	exec.initialized = true
	exec.endpoints = endpoints

	request := &ExecutorCreateRequest{
		InstanceID: "requested-exec",
		TaskID:     "task-1",
		SessionID:  "session-1",
		Metadata:   map[string]interface{}{MetadataKeyContainerID: "container-retained"},
		AuthToken:  "control-token",
	}
	_, err := exec.CreateInstance(context.Background(), request)
	require.Error(t, err)
	require.ErrorIs(t, err, errReconnectOwnerBlocked)
	require.Contains(t, err.Error(), "delivery journal")
	require.False(t, requestsContain(engine.snapshot(), "/containers/create"),
		"a retained container must not receive a fresh container create: %v", engine.snapshot())
}

// @covers AC-PLATFORM-DURABLE-AGENT-DELIVERY-002.1
func TestRemoteDockerCreateInstanceBlocksFreshFallbackAfterJournalFailure(t *testing.T) {
	engine, dockerClient := newRetainedContainerEngine(t)
	_, endpoints := newJournalFailureControl(t)
	exec := NewRemoteDockerExecutor(newTestDockerLogger())
	containerMgr := NewContainerManager(dockerClient, "", newTestDockerLogger())
	exec.connect = func(context.Context, *ExecutorCreateRequest) (*remoteDockerSession, error) {
		return &remoteDockerSession{dockerClient: dockerClient, containerMgr: containerMgr, endpoints: endpoints}, nil
	}
	exec.watchTransport = func(string, *remoteDockerSession) {}
	launched := false
	exec.launch = func(context.Context, *remoteDockerSession, *ExecutorCreateRequest) (*ExecutorInstance, error) {
		launched = true
		return &ExecutorInstance{InstanceID: "fresh"}, nil
	}

	request := &ExecutorCreateRequest{
		InstanceID: "requested-exec",
		TaskID:     "task-1",
		SessionID:  "session-1",
		Metadata:   map[string]interface{}{MetadataKeyContainerID: "container-retained"},
		AuthToken:  "control-token",
	}
	_, err := exec.CreateInstance(context.Background(), request)
	require.Error(t, err)
	require.ErrorIs(t, err, errReconnectOwnerBlocked)
	require.False(t, launched, "blocked reconnect must not launch a fresh remote container")
	require.False(t, requestsContain(engine.snapshot(), "/containers/create"),
		"a retained container must not receive a fresh container create: %v", engine.snapshot())
}

func requestsContain(requests []string, suffix string) bool {
	for _, request := range requests {
		if strings.HasSuffix(request, suffix) {
			return true
		}
	}
	return false
}
