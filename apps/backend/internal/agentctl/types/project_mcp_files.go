package types

import "github.com/kandev/kandev/internal/common/passthroughconfig"

// ProjectMCPFilesVersion versions the executor-local project MCP file
// contract. The target and prepare responses carry the helper's own view of
// the workspace so the backend never infers a remote filesystem path.
const ProjectMCPFilesVersion = 1

// ProjectMCPTarget is the helper's executor-local project MCP placement.
// MCPURL is the loopback endpoint the agent process can reach inside its own
// filesystem/network namespace.
type ProjectMCPTarget struct {
	Version       int    `json:"version"`
	InstanceID    string `json:"instance_id"`
	SessionID     string `json:"session_id"`
	WorkspacePath string `json:"workspace_path"`
	MCPURL        string `json:"mcp_url"`
	Generation    uint64 `json:"generation"`
}

// ProjectMCPPrepareRequest materializes generated project configuration files
// inside the helper workspace. Generation fences stale preparation attempts.
type ProjectMCPPrepareRequest struct {
	Version    int                      `json:"version"`
	Generation uint64                   `json:"generation"`
	Files      []passthroughconfig.File `json:"files"`
}

// ProjectMCPPreparedFile is read-back evidence for one materialized file.
type ProjectMCPPreparedFile struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

// ProjectMCPPrepareResponse confirms materialization with the helper's own
// identity, workspace, endpoint, and read-back file fingerprints.
type ProjectMCPPrepareResponse struct {
	Version       int                      `json:"version"`
	InstanceID    string                   `json:"instance_id"`
	SessionID     string                   `json:"session_id"`
	WorkspacePath string                   `json:"workspace_path"`
	MCPURL        string                   `json:"mcp_url"`
	Generation    uint64                   `json:"generation"`
	Files         []ProjectMCPPreparedFile `json:"files,omitempty"`
}
