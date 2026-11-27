package backendapp

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"go.uber.org/zap"
)

type mcpSessionDataCleaner interface {
	DeleteMCPSessionData(context.Context, string) error
}

func registerMCPSessionDeletionCleanup(eventBus bus.EventBus, repo interface{}, log *logger.Logger) {
	cleaner, ok := repo.(mcpSessionDataCleaner)
	if !ok {
		return
	}
	if _, err := eventBus.Subscribe(events.SessionRemoved, func(ctx context.Context, event *bus.Event) error {
		sessionID := mcpEventString(event, "session_id")
		if sessionID == "" {
			return nil
		}
		if err := cleaner.DeleteMCPSessionData(ctx, sessionID); err != nil {
			return fmt.Errorf("delete MCP session data: %w", err)
		}
		return nil
	}); err != nil {
		log.Error("failed to subscribe to MCP session deletion cleanup", zap.Error(err))
	}
}
