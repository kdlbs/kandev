package models

import (
	"encoding/json"
	"errors"
)

var ErrCapabilityGeneration = errors.New("capability directory changed; restart pagination")

// Capability is a projection of a native catalog, never its configuration.
// Effect describes authority requirements; discovery grants no execution rights.
type Capability struct {
	ID             string          `json:"id"`
	Kind           string          `json:"kind"`
	Name           string          `json:"name"`
	InputSchema    json.RawMessage `json:"input_schema"`
	SchemaPartial  bool            `json:"schema_partial,omitempty"`
	Effect         string          `json:"effect"`
	Surfaces       []string        `json:"surfaces"`
	WorkspaceID    string          `json:"workspace_id"`
	ResourceID     string          `json:"resource_id,omitempty"`
	ProfileID      string          `json:"profile_id,omitempty"`
	SessionID      string          `json:"session_id,omitempty"`
	Revision       string          `json:"revision"`
	Health         string          `json:"health"`
	Reason         string          `json:"reason"`
	Configured     bool            `json:"configured"`
	Attached       bool            `json:"attached"`
	InspectAllowed bool            `json:"inspect_allowed"`
}

type CapabilityQuery struct {
	OwnerID, WorkspaceID, ConversationID, SessionID, Kind string
	After, Generation                                     string
	Limit                                                 int
}

type CapabilityPage struct {
	Entries    []Capability `json:"entries"`
	Generation string       `json:"generation"`
	After      string       `json:"-"`
	NextCursor string       `json:"next_cursor"`
}
