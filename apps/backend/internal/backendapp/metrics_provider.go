package backendapp

import (
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/system/metrics"
)

type lifecycleMetricProvider struct {
	manager metricExecutionLister
}

type metricExecutionLister interface {
	ListExecutions() []*lifecycle.AgentExecution
}

func (p lifecycleMetricProvider) MetricExecutions() []metrics.ExecutionSource {
	if p.manager == nil {
		return nil
	}
	executions := p.manager.ListExecutions()
	sources := make([]metrics.ExecutionSource, 0, len(executions))
	for _, execution := range executions {
		if execution == nil || execution.GetAgentCtlClient() == nil || !shouldCollectExecutionMetrics(execution.RuntimeName) {
			continue
		}
		label := fmt.Sprintf("Execution %s", execution.SessionID)
		if execution.TaskID != "" {
			label = fmt.Sprintf("Task %s execution", execution.TaskID)
		}
		sources = append(sources, metrics.ExecutionSource{
			ID:           execution.ID,
			Label:        label,
			ExecutorType: execution.RuntimeName.String(),
			SessionID:    execution.SessionID,
			TaskID:       execution.TaskID,
			Client:       execution.GetAgentCtlClient(),
		})
	}
	return sources
}

func shouldCollectExecutionMetrics(runtime agentruntime.Runtime) bool {
	switch runtime {
	case agentruntime.RuntimeDocker, agentruntime.RuntimeRemoteDocker, agentruntime.RuntimeSprites, agentruntime.RuntimeSSH:
		return true
	default:
		return false
	}
}
