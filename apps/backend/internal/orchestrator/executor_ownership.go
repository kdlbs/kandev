package orchestrator

import (
	"context"
)

func (s *Service) SetExecutorAdmission(admit func(context.Context, string, string, string) (context.Context, error)) {
	s.executor.SetExecutorAdmission(admit)
}

func (s *Service) SetAutonomousExecutorContext(resolve func(context.Context, string, string) (context.Context, error)) {
	s.autonomousExecutorContext = resolve
	s.executor.SetAutonomousExecutorContext(resolve)
}

func (s *Service) autonomousExecutorActor(ctx context.Context, creator, profile string) (context.Context, error) {
	if s.autonomousExecutorContext == nil {
		return ctx, nil
	}
	return s.autonomousExecutorContext(ctx, creator, profile)
}
