package lifecycle

import (
	"context"
	"encoding/json"
	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

type lifecycleStopProofConsumer struct{ calls int }

func (c *lifecycleStopProofConsumer) Execute(context.Context, *agentctl.Client, *utility.PromptRequest, agentctl.InferenceIdentity) (*utility.PromptResponse, error) {
	return nil, nil
}
func (c *lifecycleStopProofConsumer) ObserveInferenceStop(context.Context, *utility.AttemptSnapshot) error {
	c.calls++
	return nil
}

type inferenceStopBackend struct {
	MockExecutor
	control *agentctl.ControlClient
}

func (b *inferenceStopBackend) StopInstance(ctx context.Context, instance *ExecutorInstance, _ bool) error {
	return b.control.DeleteInstance(ctx, instance.InstanceID)
}
func TestInferenceStopLifecycleActualBackendDeleteForwardsOwner(t *testing.T) {
	manager := newTestManager(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"inference_attempts": utility.AttemptSnapshot{InstanceID: "instance", Known: true, Attempts: []utility.AttemptStatus{}}})
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	control := agentctl.NewControlClient(u.Hostname(), port, newTestLogger())
	defer control.Close()
	registry := NewExecutorRegistry(newTestLogger())
	registry.Register(&inferenceStopBackend{MockExecutor: MockExecutor{name: executor.NameStandalone}, control: control})
	manager.executorRegistry = registry
	consumer := &lifecycleStopProofConsumer{}
	manager.SetInferenceConsumer(consumer)
	if err := manager.stopAgentViaBackend(context.Background(), "instance", &AgentExecution{ID: "instance", RuntimeName: executor.NameStandalone}, StopReasonTaskDeleted, true, false); err != nil {
		t.Fatal(err)
	}
	if consumer.calls != 1 {
		t.Fatalf("actual lifecycle backend dropped owner: calls=%d", consumer.calls)
	}
}

func TestInferenceStopStandaloneBackgroundRetryRetainsInstalledOwner(t *testing.T) {
	manager := newTestManager(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"inference_attempts": utility.AttemptSnapshot{InstanceID: "instance", Known: true, Attempts: []utility.AttemptStatus{}}})
	}))
	defer server.Close()
	u, _ := url.Parse(server.URL)
	port, _ := strconv.Atoi(u.Port())
	control := agentctl.NewControlClient(u.Hostname(), port, newTestLogger())
	defer control.Close()
	backend := &StandaloneExecutor{ctl: control, logger: newTestLogger()}
	registry := NewExecutorRegistry(newTestLogger())
	registry.Register(backend)
	manager.executorRegistry = registry
	consumer := &lifecycleStopProofConsumer{}
	manager.SetInferenceConsumer(consumer)
	if err := backend.stopWithRetryClient(context.Background(), control, "instance"); err != nil {
		t.Fatal(err)
	}
	if consumer.calls != 1 {
		t.Fatalf("startup background stop lost installed owner: calls=%d", consumer.calls)
	}
}
