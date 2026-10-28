package lifecycle

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/kandev/kandev/internal/agentruntime"
)

// kubernetesReconnectBackend mimics a Kubernetes reconnect: the returned
// instance carries the recorded Pod identity of the request and no reuse flag.
type kubernetesReconnectBackend struct {
	*createBackend
	podUID string
}

func (b *kubernetesReconnectBackend) CreateInstance(ctx context.Context, req *ExecutorCreateRequest) (*ExecutorInstance, error) {
	inst, err := b.createBackend.CreateInstance(ctx, req)
	if err != nil {
		return nil, err
	}
	inst.RuntimeName = agentruntime.RuntimeKubernetes
	inst.Metadata = map[string]interface{}{MetadataKeyKubernetesPodUID: b.podUID}
	return inst, nil
}

func TestCreateInstanceWithSessionMCPRefusesKubernetesReconnectWithoutStopping(t *testing.T) {
	b := &kubernetesReconnectBackend{createBackend: newCreateBackend(t, nil), podUID: "pod-uid-1"}
	req := &ExecutorCreateRequest{InstanceID: "exec-1", Metadata: map[string]interface{}{MetadataKeyKubernetesPodUID: "pod-uid-1"}}
	b.setHandler(servesFingerprint([]string{"session_mcp_token"}, func(*http.Request) string { return "retained-token" }))
	m := newLaunchManager(&keyedStore{key: launchKey()})
	_, err := m.createInstanceWithSessionMCP(context.Background(), b, req)
	var refusal *LaunchRefusal
	if !errors.As(err, &refusal) || refusal.Cause != CauseRetainedInstanceDeferred {
		t.Fatalf("err = %v, want cause %s", err, CauseRetainedInstanceDeferred)
	}
	if len(b.stopped) != 0 {
		t.Fatalf("reconnected instance was stopped: %q", b.stopped)
	}
}

func TestCreateInstanceWithSessionMCPKubernetesFreshPodIsNotRetained(t *testing.T) {
	b := &kubernetesReconnectBackend{createBackend: newCreateBackend(t, nil), podUID: "pod-uid-2"}
	req := &ExecutorCreateRequest{InstanceID: "exec-1", Metadata: map[string]interface{}{MetadataKeyKubernetesPodUID: "pod-uid-1"}}
	b.setHandler(servesFingerprint([]string{"session_mcp_token"}, func(r *http.Request) string { return "" }))
	m := newLaunchManager(&keyedStore{key: launchKey()})
	_, err := m.createInstanceWithSessionMCP(context.Background(), b, req)
	var refusal *LaunchRefusal
	if errors.As(err, &refusal) && refusal.Cause == CauseRetainedInstanceDeferred {
		t.Fatalf("a recreated Pod was treated as retained: %v", err)
	}
}
