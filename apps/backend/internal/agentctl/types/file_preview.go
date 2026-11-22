package types

import "errors"

var ErrFilePreviewLimit = errors.New("file preview limit exceeded")

// FilePreviewMetadata describes a metadata-only selection on one agentctl instance.
// Token is a single-use read handle, never a grant of user permission.
type FilePreviewMetadata struct {
	Token     string `json:"token"`
	Path      string `json:"path"`
	External  bool   `json:"external"`
	SizeBytes int64  `json:"size_bytes"`
}
