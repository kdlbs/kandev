package backendapp

import (
	"context"
	"errors"
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

type lifecycleMetricClient struct {
	execution *lifecycle.AgentExecution
}

func (c lifecycleMetricClient) SystemMetrics(
	ctx context.Context,
	metricIDs []string,
	diskPath string,
) (*metrics.SourceSnapshot, error) {
	client, releaseClient := c.execution.AcquireAgentCtlClient()
	defer releaseClient()
	if client == nil {
		return nil, errors.New("agentctl client is unavailable")
	}
	return client.SystemMetrics(ctx, metricIDs, diskPath)
}

func (p lifecycleMetricProvider) MetricExecutions() []metrics.ExecutionSource {
	if p.manager == nil {
		return nil
	}
	executions := p.manager.ListExecutions()
	sources := make([]metrics.ExecutionSource, 0, len(executions))
	for _, execution := range executions {
		if execution == nil || !shouldCollectExecutionMetrics(execution.RuntimeName) {
			continue
		}
		client, releaseClient := execution.AcquireAgentCtlClient()
		hasClient := client != nil
		releaseClient()
		if !hasClient {
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
			Client:       lifecycleMetricClient{execution: execution},
		})
	}
	return sources
}

func shouldCollectExecutionMetrics(runtime agentruntime.Runtime) bool {
	return runtime.IsContainerized() || runtime == agentruntime.RuntimeSSH
}
