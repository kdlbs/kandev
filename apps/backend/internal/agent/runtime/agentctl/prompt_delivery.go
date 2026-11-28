package client

import "errors"

// PromptDefinitelyUnsent excludes response loss and write ambiguity.
func PromptDefinitelyUnsent(err error) bool {
	var unsent *RequestNotSentError
	return errors.As(err, &unsent)
}
