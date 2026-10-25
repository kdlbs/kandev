package types

import "fmt"

const MaxWorkspacePreviewContentBytes = 5 * 1024 * 1024

// WorkspacePreviewRequest is the current editor buffer to publish through
// agentctl. The content is ephemeral and is not persisted by the client.
type WorkspacePreviewRequest struct {
	Repo    string `json:"repo,omitempty"`
	Path    string `json:"path"`
	Content string `json:"content"`
}

// WorkspacePreviewResponse identifies the live agentctl preview server and
// the published entry document.
type WorkspacePreviewResponse struct {
	Port    int    `json:"port"`
	Path    string `json:"path"`
	Version uint64 `json:"version"`
}

// WorkspacePreviewHTTPError preserves an agentctl response status without
// retaining response content supplied by the workspace.
type WorkspacePreviewHTTPError struct {
	Status int
}

// StatusCode returns the upstream HTTP status for stable handler mapping.
func (e *WorkspacePreviewHTTPError) StatusCode() int {
	return e.Status
}

func (e *WorkspacePreviewHTTPError) Error() string {
	return fmt.Sprintf("workspace preview publish failed with status %d", e.Status)
}
