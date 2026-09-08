package backendapp

import (
	"testing"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
)

func TestLifecycleMetricProviderIncludesKubernetesExecution(t *testing.T) {
	execution := executionWithAgentCtl(t, &lifecycle.AgentExecution{
		ID: "exec-k8s", TaskID: "task-1", SessionID: "session-1",
		RuntimeName: agentruntime.RuntimeKubernetes,
	})
	provider := lifecycleMetricProvider{manager: metricExecutionListStub{
		executions: []*lifecycle.AgentExecution{execution},
	}}

	sources := provider.MetricExecutions()
	if len(sources) != 1 {
		t.Fatalf("execution sources = %d, want 1", len(sources))
	}
	if sources[0].ExecutorType != string(agentruntime.RuntimeKubernetes) {
		t.Fatalf("executor type = %q, want k8s", sources[0].ExecutorType)
	}
}
