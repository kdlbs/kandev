// Package types provides shared types for agentctl packages.
// This breaks import cycles between adapter and acp packages.
package types

import "context"

// PermissionRequest represents a permission request from the agent
type PermissionRequest struct {
	SessionID  string             `json:"session_id"`
	ToolCallID string             `json:"tool_call_id"`
	Title      string             `json:"title"`
	Options    []PermissionOption `json:"options"`
}

// PermissionOption represents a permission choice
type PermissionOption struct {
	OptionID string `json:"option_id"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // allow_once, allow_always, reject_once, reject_always
}

// PermissionResponse is the user's response to a permission request
type PermissionResponse struct {
	OptionID  string `json:"option_id,omitempty"`
	Cancelled bool   `json:"cancelled,omitempty"`
}

// PermissionHandler is called when the agent requests permission for an action
type PermissionHandler func(ctx context.Context, req *PermissionRequest) (*PermissionResponse, error)

