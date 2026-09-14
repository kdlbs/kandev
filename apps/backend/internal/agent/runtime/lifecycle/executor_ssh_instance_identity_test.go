package lifecycle

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSSHAgentctlIdentitySurvivesRepeatedRecovery(t *testing.T) {
	const remoteInstanceID = "original-remote-instance"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Instance-ID") != remoteInstanceID || r.Header.Get("Authorization") != "Bearer token" {
			http.Error(w, "instance no longer exists at this port", http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"agent_status":"running"}`))
	}))
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(serverURL.Port())
	require.NoError(t, err)

	for _, retainProbedClient := range []bool{true, false} {
		t.Run(strconv.FormatBool(retainProbedClient), func(t *testing.T) {
			executor := &SSHExecutor{logger: newNopLogger(t)}
			previousID := remoteInstanceID
			// Start with a legacy runtime row that has no explicit controller identity.
			metadata := map[string]interface{}{
				MetadataKeySSHRemoteAgentctlPort: "43123",
				MetadataKeySSHRemoteTaskDir:      "/remote/task",
			}
			for generation := 1; generation <= 3; generation++ {
				req := &ExecutorCreateRequest{
					InstanceID:          "recovered-instance-" + strconv.Itoa(generation),
					PreviousExecutionID: previousID,
					SessionID:           "session-1",
					AuthToken:           "token",
					Metadata:            metadata,
				}
				client, reusing := executor.newResumedAgentctlClient(context.Background(), port, req)
				require.True(t, reusing, "recovery %d must reach the original remote controller", generation)
				state := &sshSessionState{
					target:    &SSHTarget{Host: "ssh.example", Port: 22, User: "agent"},
					forwarder: &SSHPortForwarder{localPort: port},
					remoteDir: "/remote/session",
					authToken: req.AuthToken,
				}
				if retainProbedClient {
					state.agentctlClient = client
				}
				instance := executor.buildResumedInstance(req, state)
				require.Equal(t, req.InstanceID, instance.InstanceID)
				require.NoError(t, instance.Client.Health(context.Background()))
				metadata = FilterPersistentMetadata(instance.Metadata)
				previousID = instance.InstanceID
			}
		})
	}
}

func TestSSHNewControllerRecordsItsOwnIdentity(t *testing.T) {
	executor := &SSHExecutor{logger: newNopLogger(t)}
	instance := executor.buildInstance(
		&ExecutorCreateRequest{InstanceID: "new-instance", Metadata: map[string]interface{}{
			"ssh_remote_agentctl_instance_id": "stale-instance",
		}},
		&SSHTarget{Host: "ssh.example", Port: 22, User: "agent"},
		&SSHPortForwarder{localPort: 43124},
		"/remote/task", "/remote/session", 43123, 99, "/home/agent/.kandev", "token",
	)
	require.Equal(t, "new-instance", instance.Metadata["ssh_remote_agentctl_instance_id"])
}

func TestSSHControllerIdentityMetadataLifecycle(t *testing.T) {
	const key = "ssh_remote_agentctl_instance_id"
	require.True(t, ShouldPersistMetadataKey(key), "identity must survive same-session restart")
	require.True(t, IsSessionScopedMetadataKey(key), "identity must not leak to sibling sessions")
	metadata := map[string]interface{}{key: "stale-instance", MetadataKeySSHRemoteTaskDir: "/remote/task"}
	clearSSHResumeRuntimeMetadata(metadata)
	require.NotContains(t, metadata, key, "replacement controllers must not inherit stale identity")
	require.Equal(t, "/remote/task", metadata[MetadataKeySSHRemoteTaskDir])
}
