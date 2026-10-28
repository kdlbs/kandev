package lifecycle

import (
	"context"
	"errors"
	"slices"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/agents"
	"github.com/kandev/kandev/internal/agent/mcpconfig"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/common/sessionmcp"
	"github.com/kandev/kandev/internal/secrets"
)

const sessionMCPKeyReadTimeout = 5 * time.Second

// mcpCarrier is how an agent receives the Authorization header of its
// Kandev MCP entry.
type mcpCarrier int

const (
	carrierHeaders  mcpCarrier = iota // header inside the generated private config
	carrierCodexEnv                   // env_http_headers plus a process environment variable
	carrierDeferred
)

// classifyMCPCarrier decides the carrier for an agent's passthrough strategy.
// An agent that is not verified to forward configured MCP headers, or whose
// Kandev entry would be written into a workspace file, gets no carrier and
// the launch is refused: an unverified agent never receives an
// unauthenticated Kandev entry.
func classifyMCPCarrier(agent agents.Agent, strategy mcpconfig.PassthroughMCPStrategy) (mcpCarrier, string) {
	if agent != nil && agents.IsPassthroughOnly(agent) {
		return carrierDeferred, CauseCustomTUIDeferred
	}
	switch strategy.(type) {
	case mcpconfig.ClaudeStrategy, *mcpconfig.ClaudeStrategy,
		mcpconfig.OpenCodeStrategy, *mcpconfig.OpenCodeStrategy:
		return carrierHeaders, ""
	case mcpconfig.CodexStrategy, *mcpconfig.CodexStrategy:
		return carrierCodexEnv, ""
	case mcpconfig.CursorStrategy, *mcpconfig.CursorStrategy,
		mcpconfig.PiStrategy, *mcpconfig.PiStrategy:
		return carrierDeferred, CauseWorkspaceFileCarrierDeferred
	default:
		return carrierDeferred, CauseAgentNotVerified
	}
}

// requireSessionMCPCarrier refuses an agent that has no way to carry the
// session token. It must run before any path that writes the Kandev entry,
// including the Cursor project-file path that bypasses withSessionMCPAuth.
func (m *Manager) requireSessionMCPCarrier(execution *AgentExecution, agent agents.Agent, strategy mcpconfig.PassthroughMCPStrategy) error {
	if _, ok := m.sessionKeyStore(); !ok {
		return nil
	}
	if carrier, cause := classifyMCPCarrier(agent, strategy); carrier == carrierDeferred {
		return &LaunchRefusal{Code: CodeMCPTokenCarrierUnavailable, Cause: cause}
	}
	return nil
}

// sessionMCPAuthorization derives the Authorization header value for the
// execution's instance. It returns "" when no signing key store is wired
// (test doubles); the production runtime always has one.
func (m *Manager) sessionMCPAuthorization(ctx context.Context, execution *AgentExecution) (string, error) {
	keys, ok := m.sessionKeyStore()
	if !ok {
		return "", nil
	}
	readCtx, cancel := context.WithTimeout(ctx, sessionMCPKeyReadTimeout)
	defer cancel()
	key, err := keys.SessionMCPKey(readCtx)
	if err != nil {
		if errors.Is(err, secrets.ErrNotFound) || errors.Is(err, secrets.ErrSessionKeyUnusable) {
			m.logger.Error(CodeMCPTokenKeyUnavailable, zap.Error(err))
		}
		return "", &LaunchRefusal{Code: CodeMCPTokenKeyUnavailable, Err: err}
	}
	nonce := ""
	if v, ok := execution.metadataValue(MetadataKeyMCPTokenNonce); ok {
		nonce, _ = v.(string)
	}
	if nonce == "" {
		return "", &LaunchRefusal{Code: CodeMCPTokenKeyUnavailable, Err: errors.New("instance token nonce missing")}
	}
	return "Bearer " + sessionmcp.Token(key, execution.ID, nonce), nil
}

// withSessionMCPAuth returns a copy of servers whose Kandev entry carries the
// session token, for handing to the strategy only. The caller keeps the
// original list, which never holds the token.
func (m *Manager) withSessionMCPAuth(
	ctx context.Context,
	execution *AgentExecution,
	agent agents.Agent,
	strategy mcpconfig.PassthroughMCPStrategy,
	servers []types.McpServer,
) ([]types.McpServer, error) {
	if _, ok := m.sessionKeyStore(); !ok {
		return servers, nil
	}
	if err := m.requireSessionMCPCarrier(execution, agent, strategy); err != nil {
		return nil, err
	}
	auth, err := m.sessionMCPAuthorization(ctx, execution)
	if err != nil {
		return nil, err
	}
	out := make([]types.McpServer, len(servers))
	copy(out, servers)
	for i := range out {
		if out[i].Name != kandevMCPServerName {
			continue
		}
		headers := make(map[string]string, len(out[i].Headers)+1)
		for k, v := range out[i].Headers {
			headers[k] = v
		}
		headers["Authorization"] = auth
		out[i].Headers = headers
	}
	return out, nil
}

// withCodexSessionTokenEnv sets the Codex token variable in the environment of
// a process this backend starts itself. It is set only on the start request,
// after the profile environment, so it never reaches persisted metadata. A
// remote passthrough is started by its own agentctl, which sets the variable
// from the token it already holds.
func (m *Manager) withCodexSessionTokenEnv(ctx context.Context, execution *AgentExecution, request process.InteractiveStartRequest) (process.InteractiveStartRequest, error) {
	if isSSHPassthrough(execution) || !slices.Contains(request.Command, sessionmcp.CodexTokenArg) {
		return request, nil
	}
	auth, err := m.sessionMCPAuthorization(ctx, execution)
	if err != nil || auth == "" {
		return request, err
	}
	env := make(map[string]string, len(request.Env)+1)
	for k, v := range request.Env {
		env[k] = v
	}
	env[sessionmcp.CodexTokenEnv] = auth
	request.Env = env
	return request, nil
}
