package process

import (
	"context"

	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
)

// providerUserInputHandler captures scope when the adapter is constructed.
// A later provider cannot grant authority to an older question callback.
func (m *Manager) providerUserInputHandler() types.UserInputRequestHandler {
	handler := m.userInputRequestHandler
	if handler == nil || m.cfg.ProviderMCP == nil {
		return handler
	}
	scope := m.cfg.ProviderMCP.Scope
	return func(ctx context.Context, request *types.UserInputRequest) (*types.UserInputResponse, error) {
		return handler(streams.WithProviderMCPScope(ctx, scope), request)
	}
}
