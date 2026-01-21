package models

import "time"

// AgentProfileMcpConfig stores MCP configuration for a specific agent profile.
type AgentProfileMcpConfig struct {
	ProfileID string                 `json:"profile_id"`
	Enabled   bool                   `json:"enabled"`
	Servers   map[string]interface{} `json:"servers"`
	Meta      map[string]interface{} `json:"meta,omitempty"`
	CreatedAt time.Time              `json:"created_at"`
	UpdatedAt time.Time              `json:"updated_at"`
}
