package lifecycle

import (
	"context"
	"fmt"
	"sort"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/usage"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
)

// SetProviderUsageLimitsEnabled is installed before the manager starts. OFF
// keeps the original configure and prompt paths without credential reads.
func (m *Manager) SetProviderUsageLimitsEnabled(enabled bool) {
	m.providerUsageLimits = enabled
	m.sessionManager.providerUsageLimits = enabled
}

func (m *Manager) configureAgentUsage(ctx context.Context, client *agentctl.Client, execution *AgentExecution, agentArgs []string, env map[string]string, delivery *agentctltypes.ProjectMCPDelivery) error {
	if !m.providerUsageLimits {
		return client.ConfigureAgentWithProjectDelivery(ctx, execution.AgentCommand, agentArgs, env, execution.ContinueCommand, execution.ContinueArgs, delivery, execution.LaunchWrapper)
	}
	policy := &usage.ExecutionUsagePolicy{ProfileID: execution.AgentProfileID, AgentID: execution.AgentID, ExecutorType: execution.ExecutorType}
	usage.ApplySelectedUsage(ctx, policy)
	return client.ConfigureAgentWithUsageAndProjectDelivery(ctx, execution.AgentCommand, agentArgs, env, execution.ContinueCommand, execution.ContinueArgs, policy, delivery, execution.LaunchWrapper)
}

// ReadProfileExecutionUsage observes existing native actors only. Session
// access is checked before borrowing a client; reads never prepare/start one.
func (m *Manager) ReadProfileExecutionUsage(ctx context.Context, profileID string) ([]usage.ProfileExecutionUsage, error) {
	return m.readExecutionUsage(ctx, func(execution *AgentExecution) bool { return execution.AgentProfileID == profileID })
}

// Office keeps its stable identity while the running concrete profile changes.
// This observes those original actors, never a profile's host-default account.
func (m *Manager) ReadOfficeProfileExecutionUsage(ctx context.Context, profileID string) ([]usage.ProfileExecutionUsage, error) {
	return m.readExecutionUsage(ctx, func(execution *AgentExecution) bool {
		if execution.OfficeAgentProfileID != "" {
			return execution.OfficeAgentProfileID == profileID
		}
		return execution.AgentProfileID == profileID
	})
}

func (m *Manager) readExecutionUsage(ctx context.Context, match func(*AgentExecution) bool) ([]usage.ProfileExecutionUsage, error) {
	result := make([]usage.ProfileExecutionUsage, 0)
	if !m.providerUsageLimits {
		return result, nil
	}
	for _, execution := range m.ListExecutions() {
		if !match(execution) {
			continue
		}
		if m.sessionAccessCheck != nil && execution.SessionID != "" {
			if err := m.sessionAccessCheck(ctx, execution.SessionID); err != nil {
				continue
			}
		}
		model := ""
		if state := execution.GetModelState(); state != nil {
			model = state.CurrentModelID
		}
		report := &usage.ExecutionUsageReport{Reason: "execution_unavailable", Model: model}
		client, release := execution.AcquireAgentCtlClient()
		if client != nil {
			if observed, err := client.ReadExecutionUsage(ctx, model); err == nil {
				report = observed
			}
		}
		release()
		result = append(result, usage.ProfileExecutionUsage{ExecutionID: execution.ID, SessionID: execution.SessionID, Report: report})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ExecutionID < result[j].ExecutionID })
	return result, nil
}

func (sm *SessionManager) checkExecutionUsage(ctx context.Context, execution *AgentExecution) error {
	if !sm.providerUsageLimits {
		return nil
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	if client == nil {
		return fmt.Errorf("execution usage owner unavailable")
	}
	model := ""
	if state := execution.GetModelState(); state != nil {
		model = state.CurrentModelID
	}
	return client.CheckExecutionUsage(ctx, model)
}
