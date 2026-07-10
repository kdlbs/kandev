package backendapp

import (
	"reflect"
	"testing"
	"unsafe"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type metricExecutionListStub struct {
	executions []*lifecycle.AgentExecution
}

func (s metricExecutionListStub) ListExecutions() []*lifecycle.AgentExecution {
	return s.executions
}

func TestLifecycleMetricProviderMetricExecutions(t *testing.T) {
	docker := executionWithAgentCtl(t, &lifecycle.AgentExecution{
		ID:          "exec-docker",
		TaskID:      "task-1",
		SessionID:   "session-1",
		RuntimeName: agentruntime.RuntimeDocker,
	})
	ssh := executionWithAgentCtl(t, &lifecycle.AgentExecution{
		ID:          "exec-ssh",
		SessionID:   "session-2",
		RuntimeName: agentruntime.RuntimeSSH,
	})
	standalone := executionWithAgentCtl(t, &lifecycle.AgentExecution{
		ID:          "exec-standalone",
		TaskID:      "task-2",
		SessionID:   "session-3",
		RuntimeName: agentruntime.RuntimeStandalone,
	})

	provider := lifecycleMetricProvider{manager: metricExecutionListStub{executions: []*lifecycle.AgentExecution{
		nil,
		docker,
		ssh,
		standalone,
		{
			ID:          "exec-no-client",
			TaskID:      "task-3",
			SessionID:   "session-4",
			RuntimeName: agentruntime.RuntimeDocker,
		},
	}}}

	sources := provider.MetricExecutions()
	if len(sources) != 2 {
		t.Fatalf("expected 2 execution sources, got %d", len(sources))
	}

	if sources[0].ID != "exec-docker" {
		t.Fatalf("expected docker source first, got %q", sources[0].ID)
	}
	if sources[0].Label != "Task task-1 execution" {
		t.Fatalf("expected task label, got %q", sources[0].Label)
	}
	if sources[0].ExecutorType != string(agentruntime.RuntimeDocker) {
		t.Fatalf("expected docker executor type, got %q", sources[0].ExecutorType)
	}
	if sources[0].Client == nil {
		t.Fatal("expected docker source client")
	}

	if sources[1].ID != "exec-ssh" {
		t.Fatalf("expected ssh source second, got %q", sources[1].ID)
	}
	if sources[1].Label != "Execution session-2" {
		t.Fatalf("expected session label, got %q", sources[1].Label)
	}
}

func TestLifecycleMetricProviderMetricExecutionsNilManager(t *testing.T) {
	sources := (lifecycleMetricProvider{}).MetricExecutions()
	if sources != nil {
		t.Fatalf("expected nil sources, got %#v", sources)
	}
}

func TestShouldCollectExecutionMetrics(t *testing.T) {
	tests := []struct {
		runtime agentruntime.Runtime
		want    bool
	}{
		{runtime: agentruntime.RuntimeDocker, want: true},
		{runtime: agentruntime.RuntimeRemoteDocker, want: true},
		{runtime: agentruntime.RuntimeSprites, want: true},
		{runtime: agentruntime.RuntimeSSH, want: true},
		{runtime: agentruntime.RuntimeStandalone, want: false},
		{runtime: agentruntime.Runtime("local_worktree"), want: false},
		{runtime: "", want: false},
	}

	for _, tt := range tests {
		if got := shouldCollectExecutionMetrics(tt.runtime); got != tt.want {
			t.Fatalf("shouldCollectExecutionMetrics(%q) = %v, want %v", tt.runtime, got, tt.want)
		}
	}
}

func executionWithAgentCtl(t *testing.T, execution *lifecycle.AgentExecution) *lifecycle.AgentExecution {
	t.Helper()
	log, err := logger.NewFromZap(zap.NewNop())
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	setAgentCtlClient(t, execution, agentctl.NewClient("127.0.0.1", 1, log))
	return execution
}

func setAgentCtlClient(t *testing.T, execution *lifecycle.AgentExecution, client *agentctl.Client) {
	t.Helper()
	field := reflect.ValueOf(execution).Elem().FieldByName("agentctl")
	reflect.NewAt(field.Type(), unsafe.Pointer(field.UnsafeAddr())).Elem().Set(reflect.ValueOf(client))
}
