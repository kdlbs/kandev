// Package channels provides channel management for the office domain.
package channels

import "time"

// Channel represents a communication channel for an agent.
type Channel struct {
	ID             string    `json:"id" db:"id"`
	WorkspaceID    string    `json:"workspace_id" db:"workspace_id"`
	AgentProfileID string    `json:"agent_profile_id" db:"agent_profile_id"`
	Platform       string    `json:"platform" db:"platform"`
	Config         string    `json:"config" db:"config"`
	Status         string    `json:"status" db:"status"`
	TaskID         string    `json:"task_id" db:"task_id"`
	CreatedAt      time.Time `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time `json:"updated_at" db:"updated_at"`
}
