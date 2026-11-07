package lifecycle

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/executor"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/usage"
	"github.com/kandev/kandev/internal/agentctl/server/utility"
)

const inferenceOwnerSession = "session"

func (m *Manager) SetInferenceConsumer(consumer agentctl.InferenceConsumer) {
	m.inferenceConsumer = consumer
	if m.executorRegistry == nil {
		return
	}
	backend, err := m.executorRegistry.GetBackend(executor.NameStandalone)
	if err != nil {
		return
	}
	if standalone, ok := backend.(*StandaloneExecutor); ok {
		standalone.inferenceStopConsumer = consumer
	}

}

func (m *Manager) executeOwnedInference(ctx context.Context, execution *AgentExecution, client *agentctl.Client, req *utility.PromptRequest, profileID string) (*utility.PromptResponse, error) {
	if m.providerUsageLimits {
		copy := *req
		copy.UsagePolicy = &usage.ExecutionUsagePolicy{ProfileID: profileID, AgentID: req.AgentID, ExecutorType: execution.ExecutorType}
		req = &copy
	}
	if m.inferenceConsumer == nil {
		return client.InferencePromptOwned(ctx, req)
	}
	identity := agentctl.InferenceIdentityFromContext(ctx)
	if !inferenceIdentityMatchesExecution(identity, execution) {
		return nil, &agentctl.InferenceCapacityError{Reason: "execution_identity_changed", Cause: fmt.Errorf("inference execution relationships changed")}
	}
	if identity.TaskID == "" {
		identity.TaskID = execution.TaskID
	}
	if identity.SessionID == "" {
		identity.SessionID = execution.SessionID
	}
	if identity.LogicalProfileID == "" {
		identity.LogicalProfileID = profileID
	}
	if profileID != "" {
		identity.ExecutionProfileID = profileID
	}
	identity.ParentExecutionID = execution.ID
	identity.OwnerKind = inferenceOwnerSession
	identity.ExecutorType = execution.ExecutorType
	return m.inferenceConsumer.Execute(ctx, client, req, identity)
}

// InferenceClients borrows recovered clients; observation never creates an execution.
func (m *Manager) InferenceClients(_ context.Context) ([]agentctl.InferenceClientRef, error) {
	if m == nil {
		return nil, fmt.Errorf("inference runtime unavailable")
	}
	var refs []agentctl.InferenceClientRef
	var unavailable bool
	for _, execution := range m.ListExecutions() {
		client, release := execution.AcquireAgentCtlClient()
		if client == nil {
			release()
			unavailable = true
			continue
		}
		refs = append(refs, agentctl.InferenceClientRef{Client: client, Release: release, Identity: agentctl.InferenceIdentity{TaskID: execution.TaskID, SessionID: execution.SessionID, ParentExecutionID: execution.ID, OwnerKind: inferenceOwnerSession, ExecutorType: execution.ExecutorType}})
	}
	if unavailable {
		return refs, fmt.Errorf("inference execution owner client unavailable")
	}
	return refs, nil
}

func inferenceIdentityMatchesExecution(identity agentctl.InferenceIdentity, execution *AgentExecution) bool {
	taskMatches := identity.TaskID == "" || execution.TaskID == "" || identity.TaskID == execution.TaskID
	sessionMatches := identity.SessionID == "" || identity.SessionID == execution.SessionID
	return taskMatches && sessionMatches
}
