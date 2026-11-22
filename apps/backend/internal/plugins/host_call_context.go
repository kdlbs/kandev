package plugins

import (
	"context"
	"errors"
)

// SetHostCallContextPolicy is startup composition, before plugin processes start.
// Existing hosts read the same policy instead of snapshotting a global caller.
func (s *Service) SetHostCallContextPolicy(enabled bool, validate func(context.Context, bool) error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hostCallContextEnabled = enabled
	s.hostCallContextValidator = validate
}

func (h *pluginHost) HostCallContextEnabled() bool {
	if h.service == nil {
		return false
	}
	h.service.mu.Lock()
	defer h.service.mu.Unlock()
	return h.service.hostCallContextEnabled
}

func (h *pluginHost) ValidateHostCallContext(ctx context.Context, agentTool bool) error {
	h.service.mu.Lock()
	validate := h.service.hostCallContextValidator
	h.service.mu.Unlock()
	if validate == nil {
		return errors.New("plugin caller context validator is unavailable")
	}
	return validate(ctx, agentTool)
}
