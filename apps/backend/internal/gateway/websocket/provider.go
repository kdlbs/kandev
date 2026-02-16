package websocket

import "github.com/kandev/kandev/internal/common/logger"

// Provide creates the unified WebSocket gateway.
func Provide(log *logger.Logger) (*Gateway, error) {
	gateway := NewGateway(log)
	return gateway, nil
}
