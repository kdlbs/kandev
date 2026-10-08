package lifecycle

import (
	"context"
	"fmt"
)

type requiredNativeConversationKey struct{}

func requiredNativeConversationID(ctx context.Context) string {
	id, _ := ctx.Value(requiredNativeConversationKey{}).(string)
	return id
}

func validateRequiredNativeConversation(ctx context.Context, existingID string, nativeResume bool) error {
	required := requiredNativeConversationID(ctx)
	if required != "" && (!nativeResume || existingID != required) {
		return fmt.Errorf("native continuation requires the unchanged provider conversation identity")
	}
	return nil
}
