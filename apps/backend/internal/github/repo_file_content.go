package github

import (
	"errors"
	"fmt"
)

// ErrRepoFileTooLarge marks a file the contents API does not inline. GitHub
// returns encoding "none" for files above its 1 MB inline limit.
var ErrRepoFileTooLarge = errors.New("repository file is too large for the contents API")

// contentsEncodingNone is the contents-API encoding for a file too large to inline.
const contentsEncodingNone = "none"

func unsupportedRepoFileEncoding(encoding string) error {
	if encoding == contentsEncodingNone {
		return fmt.Errorf("get repo file content: unsupported encoding %q: %w", encoding, ErrRepoFileTooLarge)
	}
	return fmt.Errorf("get repo file content: unsupported encoding %q", encoding)
}
