package lifecycle

import (
	"context"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// executionBuiltinMCPCredential reads the built-in MCP caller credential from
// the execution's own agentctl instance. It is read on demand rather than
// cached, so a replaced or adopted instance always yields its own value.
//
// An execution without an agentctl client has no instance that could enforce
// a credential, so it yields "". A failed read from an attached instance is an
// error: the caller must not write header-less entries for an instance that
// may enforce one.
func executionBuiltinMCPCredential(ctx context.Context, execution *AgentExecution) (string, error) {
	if execution == nil {
		return "", nil
	}
	client, release := execution.AcquireAgentCtlClient()
	defer release()
	return clientBuiltinMCPCredential(ctx, client)
}

// clientBuiltinMCPCredential reads the credential from one specific agentctl
// instance. Restart paths pass the staged replacement client, whose credential
// differs from the instance still published on the execution.
func clientBuiltinMCPCredential(ctx context.Context, client *agentctl.Client) (string, error) {
	if client == nil {
		return "", nil
	}
	return client.BuiltinMCPCredential(ctx)
}
