package share

import (
	"context"
	"errors"
	"net/http"

	"github.com/kandev/kandev/internal/github"
)

// ErrSnapshotTooLarge is returned when a snapshot exceeds the backend's
// per-file size cap. The HTTP handler maps this to 413.
var ErrSnapshotTooLarge = errors.New("snapshot exceeds maximum size")

// ErrWorkspaceRequired prevents share operations from falling back to an
// installation-global GitHub identity when task ownership is missing.
var ErrWorkspaceRequired = errors.New("task workspace is required for GitHub sharing")

// Backend is the contract every storage backend must implement. The snapshot
// has already been redacted by the caller; the backend is responsible only
// for transport and persistence.
type Backend interface {
	// Name returns the backend identifier persisted in task_shares.backend.
	Name() string

	// Upload publishes the snapshot and returns the backend-specific
	// identifier (e.g. gist ID) and the public URL where it can be viewed.
	Upload(ctx context.Context, workspaceID string, snap *Snapshot) (externalID, externalURL string, err error)

	// Delete removes a previously-uploaded snapshot. Backends MAY return a
	// "not found" error which the service detects via IsAlreadyGone.
	Delete(ctx context.Context, workspaceID, externalID string) error

	// CheckAccess validates that the workspace-owned principal can use this
	// backend without performing the upload.
	CheckAccess(ctx context.Context, workspaceID string) error
}

// IsAlreadyGone reports whether err signals the backend resource no longer
// exists. The service uses this to treat repeated revoke calls as success.
// Currently scoped to GitHub-API 404s but the helper lives here so future
// backends can extend it without leaking package boundaries.
func IsAlreadyGone(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *github.GitHubAPIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound
	}
	return false
}
