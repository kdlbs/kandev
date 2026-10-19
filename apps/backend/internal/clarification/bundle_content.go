package clarification

import (
	"context"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// BundleMessageReader is the one store read HydrateBundle needs.
type BundleMessageReader interface {
	FindMessagesByPendingIDs(ctx context.Context, pendingIDs []string) (map[string][]*taskmodels.Message, error)
}

// BundleContent is a bundle's rendered messages and shared context, exactly
// as the Needs-you Inbox emits them.
type BundleContent struct {
	Context  string
	Messages []*v1.Message
}

// HydrateBundle loads and renders one bundle's messages. It returns nil when
// the bundle has no resolvable messages.
func HydrateBundle(
	ctx context.Context, store BundleMessageReader, summary taskmodels.ClarificationBundleSummary,
) (*BundleContent, error) {
	byPending, err := store.FindMessagesByPendingIDs(ctx, []string{summary.PendingID})
	if err != nil {
		return nil, err
	}
	return bundleContentFromMessages(byPending[summary.PendingID]), nil
}

func bundleContentFromMessages(msgs []*taskmodels.Message) *BundleContent {
	if len(msgs) == 0 {
		return nil
	}
	ordered := orderInboxMessages(msgs)
	return &BundleContent{Context: inboxBundleContext(ordered), Messages: renderInboxMessages(ordered)}
}
