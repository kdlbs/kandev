package plugins

import (
	"context"
	"errors"
	"io"
)

var ErrFilePreviewTooLarge = errors.New("file preview limit exceeded")

// FilePreviewSource binds a selected file to its original authorized execution.
type FilePreviewSource struct {
	WorkspaceID, Path, ExecutionLabel string
	External                          bool
	SizeBytes                         int64
	Read                              func(context.Context, io.Writer) error
	Validate                          func(context.Context) error
	Close                             func()
}

func (s *Service) SetFilePreviewBackend(prepare func(context.Context, string, string, string) (*FilePreviewSource, error)) {
	s.filePreviewBackend = prepare
}
