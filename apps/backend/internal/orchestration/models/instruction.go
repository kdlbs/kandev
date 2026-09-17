package models

type InstructionFile struct {
	ID             string `json:"id" db:"id"`
	AgentProfileID string `json:"agent_profile_id" db:"agent_profile_id"`
	Filename       string `json:"filename" db:"filename"`
	Content        string `json:"content" db:"content"`
	IsEntry        bool   `json:"is_entry" db:"is_entry"`
	CreatedAt      string `json:"created_at" db:"created_at"`
	UpdatedAt      string `json:"updated_at" db:"updated_at"`
}
