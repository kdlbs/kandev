package dto

import "time"

// AgentProfileUsageDTO is a provider reading, not recorded cost or an allowance.
// AccountID is an opaque public grouping ID; credential locations stay private.
type AgentProfileUsageDTO struct {
	ProfileID    string                       `json:"profile_id"`
	ModelID      string                       `json:"model_id"`
	State        string                       `json:"state"`
	Stale        bool                         `json:"stale,omitempty"`
	Reason       string                       `json:"reason,omitempty"`
	Status       int                          `json:"status,omitempty"`
	Source       string                       `json:"source"`
	BindingScope string                       `json:"binding_scope,omitempty"`
	Provider     string                       `json:"provider,omitempty"`
	AccountID    string                       `json:"account_id,omitempty"`
	ModelClass   string                       `json:"model_class,omitempty"`
	Plan         string                       `json:"plan,omitempty"`
	FetchedAt    *time.Time                   `json:"fetched_at,omitempty"`
	Windows      []AgentProfileUsageWindowDTO `json:"windows"`
}

type AgentProfileUsageWindowDTO struct {
	Label          string     `json:"label"`
	UtilizationPct float64    `json:"utilization_pct"`
	ResetAt        *time.Time `json:"reset_at,omitempty"`
	Scope          string     `json:"scope,omitempty"`
	LimitReached   bool       `json:"limit_reached,omitempty"`
}

type ListAgentProfileUsageResponse struct {
	Profiles []AgentProfileUsageDTO `json:"profiles"`
}
