package types

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
