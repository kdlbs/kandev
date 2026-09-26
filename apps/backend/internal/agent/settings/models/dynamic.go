package models

import "time"

// DynamicAgentProfile stores the optimistic version for one dynamic profile's
// routing document. The parent agent_profiles row remains the profile's
// identity and display configuration.
type DynamicAgentProfile struct {
	ProfileID string    `json:"profile_id"`
	Version   int64     `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// DynamicAgentRoute is one ordered concrete candidate in a dynamic profile.
// RulesJSON is normalized by the dynamic-profile controller before persistence.
type DynamicAgentRoute struct {
	DynamicProfileID   string `json:"dynamic_profile_id"`
	Position           int    `json:"position"`
	ExecutionProfileID string `json:"execution_profile_id"`
	Enabled            bool   `json:"enabled"`
	RulesJSON          string `json:"rules_json"`
}

// DynamicProfileReference describes a dynamic profile that still points at a
// concrete profile. Soft-deleted parents are included so delete confirmation
// can explain stale configuration instead of silently losing the dependency.
type DynamicProfileReference struct {
	ProfileID string     `json:"profile_id"`
	Name      string     `json:"name"`
	DeletedAt *time.Time `json:"deleted_at,omitempty"`
}
