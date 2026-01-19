package websocket

import "github.com/kandev/kandev/internal/common/logger"

// Provide creates the unified WebSocket gateway.
func Provide(log *logger.Logger) (*Gateway, func() error, error) {
	gateway := NewGateway(log)
	return gateway, func() error { return nil }, nil
}
