package handlers

import (
	"context"
	"errors"
)

func (*mockRepository) MergeTaskMetadata(context.Context, string, map[string]interface{}) error {
	return errors.New("metadata merges are not supported by this test repository")
}
