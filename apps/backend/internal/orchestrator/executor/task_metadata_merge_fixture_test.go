package executor

import (
	"context"
	"fmt"
)

func (*mockRepository) MergeTaskMetadata(context.Context, string, map[string]interface{}) error {
	return fmt.Errorf("metadata merges are not supported by this test repository")
}
