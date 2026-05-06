package agents

import (
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/routing"
)

// CreateAgentRequest is the request body for creating an agent instance.
type CreateAgentRequest struct {
	Name                  string `json:"name"`
	AgentProfileID        string `json:"agent_profile_id"`
	Role                  string `json:"role"`
	Icon                  string `json:"icon"`
	ReportsTo             string `json:"reports_to"`
	Permissions           string `json:"permissions"`
	Reason                string `json:"reason"`
	BudgetMonthlyCents    int    `json:"budget_monthly_cents"`
	MaxConcurrentSessions int    `json:"max_concurrent_sessions"`
	DesiredSkills         string `json:"desired_skills"`
	ExecutorPreference    string `json:"executor_preference"`
}

// UpdateAgentRequest is the request body for updating an agent instance.
type UpdateAgentRequest struct {
	Name                  *string `json:"name,omitempty"`
	AgentProfileID        *string `json:"agent_profile_id,omitempty"`
	Role                  *string `json:"role,omitempty"`
	Icon                  *string `json:"icon,omitempty"`
	Status                *string `json:"status,omitempty"`
	ReportsTo             *string `json:"reports_to,omitempty"`
	Permissions           *string `json:"permissions,omitempty"`
	BudgetMonthlyCents    *int    `json:"budget_monthly_cents,omitempty"`
	MaxConcurrentSessions *int    `json:"max_concurrent_sessions,omitempty"`
	DesiredSkills         *string `json:"desired_skills,omitempty"`
	SkillIDs              *string `json:"skill_ids,omitempty"`
	ExecutorPreference    *string `json:"executor_preference,omitempty"`
	PauseReason           *string `json:"pause_reason,omitempty"`
	// Routing carries an optional provider-routing override blob. When
	// non-nil it replaces the agent's stored override entirely; a zero
	// AgentOverrides clears the overrides.
	Routing *routing.AgentOverrides `json:"routing,omitempty"`
}

// UpdateAgentStatusRequest is the request body for changing agent status.
type UpdateAgentStatusRequest struct {
	Status      string `json:"status"`
	PauseReason string `json:"pause_reason"`
}

// AgentResponse wraps a single agent instance.
type AgentResponse struct {
	Agent *models.AgentInstance `json:"agent"`
}

// AgentListResponse wraps a list of agent instances.
type AgentListResponse struct {
	Agents []*models.AgentInstance `json:"agents"`
}

// UpsertMemoryRequest is the request body for creating/updating agent memory.
type UpsertMemoryRequest struct {
	Entries []MemoryEntry `json:"entries"`
}

// MemoryEntry represents a single memory entry for upsert.
type MemoryEntry struct {
	Layer    string `json:"layer"`
	Key      string `json:"key"`
	Content  string `json:"content"`
	Metadata string `json:"metadata"`
}

// MemoryListResponse wraps a list of memory entries.
type MemoryListResponse struct {
	Memory []*models.AgentMemory `json:"memory"`
}

// MemorySummaryResponse wraps a summary of memory entries.
type MemorySummaryResponse struct {
	Count int `json:"count"`
}

// InstructionFileResponse wraps a single instruction file.
type InstructionFileResponse struct {
	File *models.InstructionFile `json:"file"`
}

// InstructionListResponse wraps a list of instruction files.
type InstructionListResponse struct {
	Files []*models.InstructionFile `json:"files"`
}

// UpsertInstructionRequest is the request body for creating/updating an instruction file.
type UpsertInstructionRequest struct {
	Content string `json:"content"`
}
