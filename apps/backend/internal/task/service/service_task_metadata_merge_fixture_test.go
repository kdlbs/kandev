package service

import (
	"context"
	"errors"
)

func (unsupportedTaskFieldUpdater) MergeTaskMetadata(context.Context, string, map[string]interface{}) error {
	return errors.New("metadata merges are not supported by this test repository")
}
