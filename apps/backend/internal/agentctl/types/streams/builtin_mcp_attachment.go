package streams

import (
	"time"

	p "github.com/kandev/kandev/internal/mcp/profile"
)

// BuiltinMCPAcceptedBinding records process acceptance separately from desired
// configuration and connection observations. Failed attempts do not replace it.
type BuiltinMCPAcceptedBinding struct {
	Manifest    *p.BuiltinManifest `json:"manifest"`
	SessionID   string             `json:"session_id"`
	ExecutionID string             `json:"execution_id"`
	AttemptID   string             `json:"attachment_attempt_id"`
	AcceptedAt  time.Time          `json:"accepted_at"`
}

func (h MCPAttachmentHistory) validBuiltinBinding() bool {
	binding := h.BuiltinBinding
	if binding == nil {
		return true
	}
	return binding.Manifest != nil && binding.Manifest.Validate() == nil &&
		binding.Manifest.TaskID == h.Current.TaskID && binding.SessionID == h.Current.SessionID &&
		binding.ExecutionID != "" && binding.AttemptID != "" && !binding.AcceptedAt.IsZero()
}

func (h *MCPAttachmentHistory) updateBuiltinAcceptance() {
	attempt := h.Current
	manifest := attempt.BuiltinPackages
	if manifest == nil || manifest.Validate() != nil || manifest.TaskID != attempt.TaskID || attempt.SessionID == "" || attempt.ExecutionID == "" {
		return
	}
	var acceptedAt time.Time
	for _, id := range manifest.Packages {
		found := false
		for _, server := range attempt.Servers {
			if server.BuiltinPackage != id || server.Source != MCPServerSourceKandev || p.BuiltinPackageForRuntimeName(server.Name) != id || server.AcceptedAt == nil || server.AcceptedAt.IsZero() {
				continue
			}
			found = true
			if server.AcceptedAt.After(acceptedAt) {
				acceptedAt = *server.AcceptedAt
			}
			break
		}
		if !found {
			return
		}
	}
	h.BuiltinBinding = &BuiltinMCPAcceptedBinding{Manifest: manifest.Clone(), SessionID: attempt.SessionID, ExecutionID: attempt.ExecutionID, AttemptID: attempt.AttemptID, AcceptedAt: acceptedAt}
}
