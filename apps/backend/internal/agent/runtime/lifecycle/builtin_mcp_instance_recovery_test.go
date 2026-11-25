package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/require"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

type builtinMCPInstanceRecoveryControl struct {
	info             *agentctl.InstanceInfo
	expectedToken    string
	replacementToken string
	createPort       int
	getStatusCode    int
	getRequests      int
	handshakes       int
	creates          int
}

func (c *builtinMCPInstanceRecoveryControl) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.URL.Path == "/health":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/v1/instances/"):
		c.getRequests++
		token := c.expectedToken
		if token == "" {
			token = "recovery-token"
		}
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if c.getStatusCode != 0 {
			http.Error(w, "instance info unavailable", c.getStatusCode)
			return
		}
		_ = json.NewEncoder(w).Encode(c.info)
	case r.Method == http.MethodPost && r.URL.Path == "/auth/handshake":
		c.handshakes++
		token := c.replacementToken
		if token == "" {
			token = "replacement-token"
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"token": token})
	case r.Method == http.MethodPost && r.URL.Path == "/api/v1/instances":
		c.creates++
		var request agentctl.CreateInstanceRequest
		_ = json.NewDecoder(r.Body).Decode(&request)
		id := request.ID
		if id == "" {
			id = "helper-1"
		}
		port := c.createPort
		if port == 0 {
			port = 41001
		}
		_ = json.NewEncoder(w).Encode(agentctl.CreateInstanceResponse{ID: id, Port: port, ScopedMCPRoutes: true})
	default:
		http.NotFound(w, r)
	}
}

func TestBuiltinMCPInstanceRecovery(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		t.Run(map[bool]string{false: "retained legacy helper", true: "retained scoped helper"}[scoped], func(t *testing.T) {
			control := &builtinMCPInstanceRecoveryControl{info: &agentctl.InstanceInfo{
				ID: "helper-1", Port: 41001, TaskID: "task-1", SessionID: "session-1",
				WorkspacePath: kubernetesWorkspacePath, ScopedMCPRoutes: scoped,
			}}
			controlServer := httptest.NewServer(control)
			t.Cleanup(controlServer.Close)
			instanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/health" {
					w.WriteHeader(http.StatusOK)
					return
				}
				http.NotFound(w, r)
			}))
			t.Cleanup(instanceServer.Close)
			controlPort := testHTTPPort(t, controlServer)
			instancePort := testHTTPPort(t, instanceServer)
			forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
				uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
				41001:                                    instancePort,
			}}
			executor := NewKubernetesExecutor(nil, newTestLogger())
			executor.healthRetryDelay = 0
			runtime := &kubernetesRuntimeClient{streams: kubeexecutor.NewStreamOperations(&recordingKubernetesExec{}, forwards)}
			req := &ExecutorCreateRequest{
				InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1",
				AuthToken: "recovery-token", BootstrapNonce: "unused-nonce",
				Metadata: map[string]interface{}{},
			}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kandev-agents", Name: "agent-pod"}}
			recorded := kubernetesRecordedState{remotePort: 41001, agentctlInstanceID: "helper-1"}

			client, forward, token, port, err := executor.reconnectKubernetesAgentctl(context.Background(), runtime, req, pod, recorded)

			require.NoError(t, err)
			t.Cleanup(func() {
				client.Close()
				require.NoError(t, forward.Close())
			})
			require.Equal(t, "recovery-token", token)
			require.Equal(t, 41001, port)
			require.Equal(t, scoped, client.BuiltinMCPScopedRoutes())
			wantURL := "http://127.0.0.1:41001/mcp"
			if scoped {
				wantURL = "http://127.0.0.1:41001/i/helper-1/mcp"
			}
			gotURL, err := canonicalBuiltinMCPURL("127.0.0.1", port, client)
			require.NoError(t, err)
			require.Equal(t, wantURL, gotURL)
			require.Zero(t, control.handshakes)
			require.Zero(t, control.creates)
			require.Equal(t, []uint16{uint16(kubeexecutor.DefaultAgentctlPort), 41001}, forwards.remotePorts())
		})
	}

	t.Run("missing authenticated instance follows the restart handshake", func(t *testing.T) {
		control := &builtinMCPInstanceRecoveryControl{getStatusCode: http.StatusNotFound}
		controlServer := httptest.NewServer(control)
		t.Cleanup(controlServer.Close)
		instanceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.WriteHeader(http.StatusOK)
				return
			}
			http.NotFound(w, r)
		}))
		t.Cleanup(instanceServer.Close)
		controlPort, instancePort := testHTTPPort(t, controlServer), testHTTPPort(t, instanceServer)
		forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
			uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
			41001:                                    instancePort,
		}}
		executor := NewKubernetesExecutor(nil, newTestLogger())
		executor.healthRetryDelay = 0
		runtime := &kubernetesRuntimeClient{streams: kubeexecutor.NewStreamOperations(&recordingKubernetesExec{}, forwards)}
		req := &ExecutorCreateRequest{
			InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1",
			AuthToken: "recovery-token", BootstrapNonce: "restart-nonce", Metadata: map[string]interface{}{},
		}
		pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kandev-agents", Name: "agent-pod"}}
		recorded := kubernetesRecordedState{remotePort: 41001, agentctlInstanceID: "helper-1"}

		client, forward, token, port, err := executor.reconnectKubernetesAgentctl(context.Background(), runtime, req, pod, recorded)

		require.NoError(t, err)
		t.Cleanup(func() {
			client.Close()
			require.NoError(t, forward.Close())
		})
		require.Equal(t, 1, control.getRequests)
		require.Equal(t, 1, control.handshakes)
		require.Equal(t, 1, control.creates)
		require.Equal(t, "replacement-token", token)
		require.Equal(t, 41001, port)
	})

	for _, test := range []struct {
		name       string
		statusCode int
	}{
		{name: "server error", statusCode: http.StatusInternalServerError},
		{name: "malformed instance info", statusCode: http.StatusOK},
	} {
		t.Run(test.name+" does not recreate", func(t *testing.T) {
			control := &builtinMCPInstanceRecoveryControl{getStatusCode: test.statusCode}
			server := httptest.NewServer(control)
			t.Cleanup(server.Close)
			controlPort := testHTTPPort(t, server)
			forwards := &recordingKubernetesForwarder{localPorts: map[uint16]uint16{
				uint16(kubeexecutor.DefaultAgentctlPort): controlPort,
			}}
			executor := NewKubernetesExecutor(nil, newTestLogger())
			executor.healthRetryDelay = 0
			runtime := &kubernetesRuntimeClient{streams: kubeexecutor.NewStreamOperations(&recordingKubernetesExec{}, forwards)}
			req := &ExecutorCreateRequest{
				InstanceID: "execution-1", TaskID: "task-1", SessionID: "session-1",
				AuthToken: "recovery-token", BootstrapNonce: "must-not-be-used", Metadata: map[string]interface{}{},
			}
			pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "kandev-agents", Name: "agent-pod"}}
			recorded := kubernetesRecordedState{remotePort: 41001, agentctlInstanceID: "helper-1"}

			_, _, _, _, err := executor.reconnectKubernetesAgentctl(context.Background(), runtime, req, pod, recorded)

			require.Error(t, err)
			require.True(t, errors.Is(err, errKubernetesInstanceModeRead), "failure must be classified as an instance-mode read failure")
			require.Zero(t, control.handshakes, "recovery must not obtain a new token after an unreadable InstanceInfo")
			require.Zero(t, control.creates, "recovery must not recreate the retained helper after an unreadable InstanceInfo")
			require.Equal(t, []uint16{uint16(kubeexecutor.DefaultAgentctlPort)}, forwards.remotePorts())
		})
	}
}

func testHTTPPort(t *testing.T, server *httptest.Server) uint16 {
	t.Helper()
	_, rawPort, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	require.NoError(t, err)
	port, err := strconv.Atoi(rawPort)
	require.NoError(t, err)
	return uint16(port)
}
