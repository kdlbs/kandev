package hostutility

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/registry"
	agentctlclient "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"net/http"
	"net/http/httptest"
	"testing"
)

type hostStopProofConsumer struct{ calls int }

func (c *hostStopProofConsumer) Execute(context.Context, *agentctlclient.Client, *utility.PromptRequest, agentctlclient.InferenceIdentity) (*utility.PromptResponse, error) {
	return nil, nil
}
func (c *hostStopProofConsumer) ObserveInferenceStop(context.Context, *utility.AttemptSnapshot) error {
	c.calls++
	return nil
}
func TestInferenceStopHostActualDeleteForwardsOwner(t *testing.T) {
	log := newTestLogger(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"inference_attempts": utility.AttemptSnapshot{InstanceID: "instance", Known: true, Attempts: []utility.AttemptStatus{}}})
	}))
	defer server.Close()
	host, port := serverHostPort(t, server)
	control := agentctlclient.NewControlClient(host, port, log)
	defer control.Close()
	manager := NewManager(registry.NewRegistry(log), host, port, control, log)
	consumer := &hostStopProofConsumer{}
	manager.SetInferenceConsumer(consumer)
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	ctx, release := hostUtilityDeleteContext(parent)
	defer release()
	manager.deleteInstance(ctx, &instance{instanceID: "instance"})
	if consumer.calls != 1 {
		t.Fatalf("actual host delete dropped owner: calls=%d", consumer.calls)
	}
}
