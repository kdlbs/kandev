package backendapp

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/conversationsearch"
)

func startConversationSearch(ctx context.Context, repository any, log *logger.Logger, register func(func() error)) {
	if store, ok := repository.(conversationsearch.Store); ok {
		stop := conversationsearch.Start(ctx, store, log)
		register(func() error { stop(); return nil })
	}
}
