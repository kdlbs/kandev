package models

import "time"

// AgentMcpConfig stores MCP configuration for a specific agent.
type AgentMcpConfig struct {
	AgentID   string                 `json:"agent_id"`
	Enabled   bool                   `json:"enabled"`
	Servers   map[string]interface{} `json:"servers"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}
