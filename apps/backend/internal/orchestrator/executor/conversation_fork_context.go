package executor

import (
	"context"

	"github.com/kandev/kandev/internal/task/models"
)

type conversationForkSessionInputKey struct{}

type conversationForkSessionInput struct {
	admission   *models.ConversationForkAdmission
	pendingTask bool
}

// WithConversationForkSessionInput carries task-owned fork admission into the
// repository transaction that creates the destination session.
func WithConversationForkSessionInput(ctx context.Context, admission *models.ConversationForkAdmission, pendingTask bool) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	var copyAdmission *models.ConversationForkAdmission
	if admission != nil {
		copyValue := *admission
		copyAdmission = &copyValue
	}
	return context.WithValue(ctx, conversationForkSessionInputKey{}, conversationForkSessionInput{
		admission: copyAdmission, pendingTask: pendingTask,
	})
}

func conversationForkSessionInputFromContext(ctx context.Context) (conversationForkSessionInput, bool) {
	if ctx == nil {
		return conversationForkSessionInput{}, false
	}
	input, ok := ctx.Value(conversationForkSessionInputKey{}).(conversationForkSessionInput)
	return input, ok
}
