package service

import "context"

func (s *Service) allowPersonaRun(ctx context.Context, id string) bool {
	if s.runAllowed == nil {
		return true
	}
	allowed, err := s.runAllowed(ctx, id)
	return err == nil && allowed
}
