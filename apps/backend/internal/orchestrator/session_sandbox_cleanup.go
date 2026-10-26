package orchestrator

import "context"

// SetSessionSandboxCleanup binds owned child-state removal before session deletion.
func (s *Service) SetSessionSandboxCleanup(cleanup func(context.Context, string, string) error) {
	s.sessionSandboxCleanup = cleanup
}
