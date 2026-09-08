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
	AutoApprove           *bool   `json:"auto_approve,omitempty"`
	AllowIndexing         *bool   `json:"allow_indexing,omitempty"`
	CLIPassthrough        *bool   `json:"cli_passthrough,omitempty"`
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

// agentResponseBody wraps a models.AgentInstance for API responses,
// shadowing the shared struct's "model" key so it can be omitted for
// Office identities (role != ""). Embedding rather than hand-mirroring
// the 43 JSON-tagged fields on the shared struct keeps this DTO in
// sync automatically as that struct grows; a *string shadow field
// (not string) is required because the embedded struct's Model field
// has no `omitempty` — a plain string would drop the "model" key
// entirely for role=="" rows too, which AC-13 requires unchanged.
type agentResponseBody struct {
	*models.AgentInstance
	Model *string `json:"model,omitempty"`
}

// newAgentResponseBody wraps agent for an API response. Model is set
// (and so emitted) only for role=="" rows (execution profiles); nil —
// omitted from JSON — for Office identities. A nil agent stays nil so
// callers serialise "agent": null exactly as before, never a
// zero-valued {} DTO.
func newAgentResponseBody(agent *models.AgentInstance) *agentResponseBody {
	if agent == nil {
		return nil
	}
	body := &agentResponseBody{AgentInstance: agent}
	if agent.Role == "" {
		m := agent.Model
		body.Model = &m
	}
	return body
}

// newAgentResponseBodies wraps a slice of agents for a list response.
func newAgentResponseBodies(agents []*models.AgentInstance) []*agentResponseBody {
	out := make([]*agentResponseBody, len(agents))
	for i, a := range agents {
		out[i] = newAgentResponseBody(a)
	}
	return out
}

// AgentResponse wraps a single agent instance.
type AgentResponse struct {
	Agent *agentResponseBody `json:"agent"`
}

// AgentListResponse wraps a list of agent instances.
type AgentListResponse struct {
	Agents []*agentResponseBody `json:"agents"`
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
