package types

const (
	// CanvasSourceTransferPath is the authenticated agentctl route used by the
	// backend to stream an assigned canvas source root into release validation.
	CanvasSourceTransferPath = "/api/v1/workspace/canvas-source"
	CanvasSourceContentType  = "application/x-tar"

	// These limits are shared by the agentctl server and the backend transfer
	// client so local, Docker, and remote executors use one bounded contract.
	MaxCanvasSourceFiles        = 512
	MaxCanvasSourceFileData     = 25 * 1024 * 1024
	MaxCanvasSourceWireBytes    = 30 * 1024 * 1024
	MaxCanvasSourceRequestBytes = 64 * 1024
)

// CanvasSourceTransferRequest selects one workspace-relative source root. The
// server resolves and validates the root against its own agent workspace.
type CanvasSourceTransferRequest struct {
	Root string `json:"root"`
}

// CanvasSourceTransferError is returned as JSON before a transfer starts when
// the request is invalid or exceeds a source bound. A partially streamed tar
// is never represented as a successful JSON response.
type CanvasSourceTransferError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
