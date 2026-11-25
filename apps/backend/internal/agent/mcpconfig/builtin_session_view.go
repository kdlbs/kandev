package mcpconfig

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
)

// BuiltinSessionView separates next-start selection from accepted execution evidence.
type BuiltinSessionView struct {
	WorkspaceID             string                             `json:"workspace_id"`
	TaskID                  string                             `json:"task_id"`
	SessionID               string                             `json:"session_id"`
	ExecutionID             string                             `json:"execution_id,omitempty"`
	Desired                 *p.BuiltinManifest                 `json:"desired,omitempty"`
	Bound                   *streams.BuiltinMCPAcceptedBinding `json:"bound,omitempty"`
	BindingState            string                             `json:"binding_state"`
	History                 *streams.MCPAttachmentHistory      `json:"history,omitempty"`
	WalkthroughAvailability string                             `json:"walkthrough_availability"`
}
