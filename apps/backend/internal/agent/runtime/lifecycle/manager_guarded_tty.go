package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agentctl/types/streams"
	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const guardedTTYAgentctlTimeout = 35 * time.Second

var ErrGuardedTTYExecutionUnavailable = errors.New("guarded TTY execution unavailable")

func guardedTTYProfileForAgent(
	profile *mcpprofile.Context,
	agentID string,
	passthrough bool,
) *mcpprofile.Context {
	if profile == nil {
		return nil
	}
	derived := mcpprofile.New(profile.Surface, profile.Capabilities, profile.Providers).
		WithoutCapability(mcpprofile.CapabilityGuardedTTYExec)
	if agentID == streams.GuardedTTYAgentID && !passthrough && derived.Surface == mcpprofile.SurfaceKanbanTask {
		derived = derived.WithCapability(mcpprofile.CapabilityGuardedTTYExec)
	}
	return &derived
}

// ExecuteGuardedTTY routes an attested request to exactly the live Codex ACP
// execution named by the trusted backend context. It never starts, resumes, or
// substitutes an execution, and it serializes against lifecycle replacement.
func (m *Manager) ExecuteGuardedTTY(
	ctx context.Context,
	request models.GuardedTTYDispatchRequest,
) (*streams.GuardedTTYExecReceipt, error) {
	if request.AttestationID == "" || request.Execution.ExecutionID == "" ||
		request.Execution.TaskID == "" || request.Execution.SessionID == "" || len(request.Argv) == 0 {
		return nil, ErrGuardedTTYExecutionUnavailable
	}
	execution, exists := m.executionStore.Get(request.Execution.ExecutionID)
	if !exists {
		return nil, ErrGuardedTTYExecutionUnavailable
	}
	execution.guardedTTYMu.Lock()
	defer execution.guardedTTYMu.Unlock()

	client, releaseClient := execution.AcquireAgentCtlClient()
	defer releaseClient()
	if !m.isExactGuardedTTYExecution(execution, request.Execution, client) {
		return nil, ErrGuardedTTYExecutionUnavailable
	}
	requestCtx, cancel := context.WithTimeout(ctx, guardedTTYAgentctlTimeout)
	defer cancel()
	receipt, err := client.GuardedTTYExec(requestCtx, streams.GuardedTTYAgentRequest{
		AttestationID: request.AttestationID,
		ExecutionID:   request.Execution.ExecutionID,
		TaskID:        request.Execution.TaskID,
		SessionID:     request.Execution.SessionID,
		Argv:          append([]string(nil), request.Argv...),
	})
	if err != nil || receipt == nil {
		return nil, ErrGuardedTTYExecutionUnavailable
	}
	if receipt.AttestationID != request.AttestationID ||
		receipt.ExecutionID != request.Execution.ExecutionID ||
		receipt.TaskID != request.Execution.TaskID || receipt.SessionID != request.Execution.SessionID ||
		receipt.ACPSessionID != execution.ACPSessionID {
		return nil, ErrGuardedTTYExecutionUnavailable
	}
	return receipt, nil
}

func (m *Manager) isExactGuardedTTYExecution(execution *AgentExecution, expected streams.MCPExecutionContext, client interface{ HasAgentStream() bool }) bool {
	if execution == nil || execution.ID != expected.ExecutionID || execution.TaskID != expected.TaskID ||
		execution.SessionID != expected.SessionID || execution.AgentID != streams.GuardedTTYAgentID ||
		execution.Status != v1.AgentStatusReady || client == nil ||
		!execution.IsAgentctlReady() || !execution.isSessionInitialized() || execution.ACPSessionID == "" ||
		!client.HasAgentStream() {
		return false
	}
	current, exists := m.executionStore.GetBySessionID(expected.SessionID)
	return exists && current == execution && current.ID == expected.ExecutionID
}
