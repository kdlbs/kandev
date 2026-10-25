package streams

import (
	"context"

	mcpprofile "github.com/kandev/kandev/internal/mcp/profile"
)

type mcpExecutionContextKey struct{}

// MCPExecutionContext identifies the backend-owned execution behind one
// in-session MCP stream. Agent-controlled request payloads cannot override it.
type MCPExecutionContext struct {
	ExecutionID               string
	TaskID                    string
	SessionID                 string
	ManagedToolPolicy         *mcpprofile.ManagedToolPolicy
	ManagedToolPolicyRequired bool

	// CoordinatorToolPolicy is the tool list bound to a coordinator
	// conversation. CoordinatorToolPolicyRequired is true when the execution
	// carries a binding at all, so an unparsable one refuses instead of
	// falling back to the phase-1 tools.
	CoordinatorToolPolicy         *mcpprofile.CoordinatorToolPolicy
	CoordinatorToolPolicyRequired bool
}

// WithMCPExecutionContext attaches trusted execution identity to a dispatch.
func WithMCPExecutionContext(ctx context.Context, execution MCPExecutionContext) context.Context {
	if execution.ManagedToolPolicy != nil {
		policy := *execution.ManagedToolPolicy
		policy.AgentToolNames = append([]string(nil), execution.ManagedToolPolicy.AgentToolNames...)
		execution.ManagedToolPolicy = &policy
	}
	if execution.CoordinatorToolPolicy != nil {
		policy := *execution.CoordinatorToolPolicy
		policy.ToolNames = append([]string(nil), execution.CoordinatorToolPolicy.ToolNames...)
		execution.CoordinatorToolPolicy = &policy
	}
	return context.WithValue(ctx, mcpExecutionContextKey{}, execution)
}

// MCPExecutionContextFromContext returns trusted execution identity.
func MCPExecutionContextFromContext(ctx context.Context) (MCPExecutionContext, bool) {
	execution, ok := ctx.Value(mcpExecutionContextKey{}).(MCPExecutionContext)
	return execution, ok && execution.TaskID != "" && execution.SessionID != ""
}
