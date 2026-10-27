package lifecycle

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/nomad"
	"github.com/kandev/kandev/internal/task/models"
)

func TestNomadRecoveryReopensPersistedReverseAPIListener(t *testing.T) {
	server := newFakeSSHServer(t, nil)
	client := server.dial(t)
	const rawURL = "http://127.0.0.1:38429/api/v1"
	launch := &ExecutorCreateRequest{Env: map[string]string{envKeyKandevAPIURL: rawURL}, Metadata: map[string]interface{}{}}
	first, _, err := openSSHRuntimeAPITunnelForRequest(client, launch)
	if err != nil || first == nil {
		t.Fatalf("launch tunnel: %v", err)
	}
	port := first.RemotePort()
	_ = first.Close()

	// Only the persisted inventory metadata survives a backend restart.
	record := &models.ExecutorRunning{AgentExecutionID: "exec-1", Metadata: map[string]interface{}{
		MetadataKeySSHRuntimeAPILocalURL:   launch.Metadata[MetadataKeySSHRuntimeAPILocalURL],
		MetadataKeySSHRuntimeAPIRemotePort: launch.Metadata[MetadataKeySSHRuntimeAPIRemotePort],
	}}
	recovery := nomadRecoveryRequest(record, nomad.EnvironmentState{EnvironmentID: "env-1"})
	reopened, _, err := openSSHRuntimeAPITunnelForRequest(client, recovery)
	if err != nil || reopened == nil {
		t.Fatalf("recovery tunnel: %v", err)
	}
	defer func() { _ = reopened.Close() }()
	if reopened.RemotePort() != port || recovery.Env[envKeyKandevAPIURL] != "http://127.0.0.1:"+port+"/api/v1" {
		t.Fatalf("recovery listener = %s url=%s, want original port %s", reopened.RemotePort(), recovery.Env[envKeyKandevAPIURL], port)
	}
}
