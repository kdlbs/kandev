package client

import (
	"context"
	"net/http"
)

// AgentSessionAssociation identifies the native session currently held by
// this authenticated agentctl instance.
type AgentSessionAssociation struct {
	InstanceID        string `json:"instance_id"`
	SessionID         string `json:"session_id"`
	IncarnationID     string `json:"incarnation_id"`
	HarnessGeneration uint64 `json:"harness_generation"`
	NativeSessionID   string `json:"native_session_id"`
	AgentStatus       string `json:"agent_status"`
}

// GetAgentSessionAssociation reads the live peer's owner and native-session
// identity without issuing an ACP request.
func (c *Client) GetAgentSessionAssociation(ctx context.Context) (*AgentSessionAssociation, error) {
	var result AgentSessionAssociation
	if err := c.doDeliveryRequest(ctx, http.MethodGet, "/api/v1/agent/session", nil, &result); err != nil {
		return nil, err
	}
	return &result, nil
}
