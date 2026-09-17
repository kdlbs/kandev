package messagequeue

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/orchestrator/dispatchcontext"
)

// SetDispatchContextResolver is configured once at startup. Queue metadata is
// already durable; retries preserve the original snapshot.
func (s *Service) SetDispatchContextResolver(resolve func(context.Context, string) (string, error)) {
	s.dispatchContextResolver = resolve
}

func (s *Service) captureDispatchContext(ctx context.Context, taskID string, metadata map[string]interface{}) (map[string]interface{}, error) {
	if _, exists := metadata[dispatchcontext.MetadataKey]; exists {
		return metadata, nil
	}
	ref, explicit := dispatchcontext.Reference(ctx)
	if !explicit && s.dispatchContextResolver != nil {
		var err error
		ref, err = s.dispatchContextResolver(ctx, taskID)
		if err != nil {
			return nil, err
		}
	}
	if ref == "" {
		return metadata, nil
	}
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata[dispatchcontext.MetadataKey] = ref
	return metadata, nil
}

func sameDispatchContext(a, b map[string]interface{}) bool {
	first, firstOK := a[dispatchcontext.MetadataKey].(string)
	second, secondOK := b[dispatchcontext.MetadataKey].(string)
	return firstOK == secondOK && first == second
}

func validateSendNowContexts(entries []QueuedMessage) error {
	for _, entry := range entries[1:] {
		if !sameDispatchContext(entries[0].Metadata, entry.Metadata) {
			return fmt.Errorf("send-now cannot combine different assistant context revisions")
		}
	}
	return nil
}
