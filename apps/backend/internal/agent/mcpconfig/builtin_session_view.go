package mcpconfig

import (
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	p "github.com/kandev/kandev/internal/mcp/profile"
	"github.com/kandev/kandev/internal/task/models"
	"time"
)

// BuiltinSessionView separates next-start selection from accepted execution evidence.
type BuiltinSessionView struct {
	ProviderBinding         *BuiltinProviderBindingView        `json:"provider_binding,omitempty"`
	ProviderAvailable       *bool                              `json:"provider_available,omitempty"`
	CanRetryApplication     bool                               `json:"can_retry_application"`
	Application             *models.BuiltinMCPApplication      `json:"application,omitempty"`
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

type BuiltinProviderBindingView struct {
	SessionID  string              `json:"session_id"`
	Scope      p.ProviderMCPScope  `json:"scope"`
	Manifest   *p.BuiltinManifest  `json:"manifest"`
	Catalog    map[string][]string `json:"catalog"`
	AcceptedAt time.Time           `json:"accepted_at"`
}
