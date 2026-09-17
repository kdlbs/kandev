package agents

import (
	"fmt"
	"github.com/kandev/kandev/internal/agent/runtimeauth"
)

type AgentAuth = runtimeauth.AgentAuth
type AgentClaims = runtimeauth.AgentClaims

const DefaultTokenDuration = runtimeauth.DefaultTokenDuration

var (
	ErrTokenMalformed = runtimeauth.ErrTokenMalformed
	ErrTokenExpired   = runtimeauth.ErrTokenExpired
	ErrTokenSignature = runtimeauth.ErrTokenSignature
)

func NewAgentAuth(key string) *AgentAuth { return runtimeauth.NewAgentAuth(key) }

// SetAuth sets the JWT auth on the service so it can validate tokens.
func (s *AgentService) SetAuth(auth *AgentAuth) {
	s.auth = auth
}

// ValidateAgentJWT validates an agent JWT and returns the claims.
// Returns an error if no AgentAuth has been configured.
func (s *AgentService) ValidateAgentJWT(token string) (*AgentClaims, error) {
	if s.auth == nil {
		return nil, fmt.Errorf("agent auth not configured")
	}
	return s.auth.ValidateAgentJWT(token)
}

// MintRuntimeJWT creates a per-run token for runtime syscall routes.
func (s *AgentService) MintRuntimeJWT(agentInstanceID, taskID, workspaceID, runID, sessionID, capabilities string) (string, error) {
	if s.auth == nil {
		return "", fmt.Errorf("agent auth not configured")
	}
	return s.auth.MintRuntimeJWT(agentInstanceID, taskID, workspaceID, runID, sessionID, capabilities)
}
