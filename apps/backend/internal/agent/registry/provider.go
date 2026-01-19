package registry

import "github.com/kandev/kandev/internal/common/logger"

// Provide creates and loads the agent registry.
func Provide(log *logger.Logger) (*Registry, func() error, error) {
	reg := NewRegistry(log)
	reg.LoadDefaults()
	return reg, func() error { return nil }, nil
}
