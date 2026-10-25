package sessioncapacity

import (
	"context"
	"errors"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type Target interface {
	SetSessionCapacity(int)
}

type Service struct {
	mu          sync.Mutex
	store       *Store
	target      Target
	environment Environment
	logger      *logger.Logger
}

func NewService(
	store *Store,
	target Target,
	environment Environment,
	log *logger.Logger,
) *Service {
	return &Service{
		store:       store,
		target:      target,
		environment: environment,
		logger:      log,
	}
}

func (s *Service) Get(ctx context.Context) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	configured, err := s.loadConfigured(ctx)
	if err != nil {
		return Response{}, err
	}
	resolution, err := Resolve(configured, s.environment)
	if err != nil {
		return Response{}, err
	}
	s.warnInvalidEnvironment(resolution)
	return resolution.Response, nil
}

func (s *Service) Update(ctx context.Context, patch SettingsPatch) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.target == nil {
		return Response{}, ErrTargetUnavailable
	}
	if environmentIsLocked(s.environment) {
		return Response{}, ErrEnvironmentLocked
	}
	if s.store == nil {
		return Response{}, errors.New("session capacity settings store unavailable")
	}
	settings, err := s.store.Update(ctx, func(current *Settings) (Settings, error) {
		resolution, resolveErr := Resolve(current, s.environment)
		if resolveErr != nil {
			return Settings{}, resolveErr
		}
		s.warnInvalidEnvironment(resolution)
		updated := patch.Apply(resolution.Settings)
		if validateErr := Validate(updated); validateErr != nil {
			return Settings{}, validateErr
		}
		return updated, nil
	})
	if err != nil {
		return Response{}, err
	}
	resolution, err := Resolve(&settings, s.environment)
	if err != nil {
		return Response{}, err
	}
	s.target.SetSessionCapacity(resolution.Effective.MaxSessions)
	return resolution.Response, nil
}

func (s *Service) loadConfigured(ctx context.Context) (*Settings, error) {
	if s.store == nil {
		return nil, errors.New("session capacity settings store unavailable")
	}
	return s.store.Load(ctx)
}

func environmentIsLocked(environment Environment) bool {
	resolution, err := Resolve(nil, environment)
	return err == nil && resolution.Effective.Locked
}

func (s *Service) warnInvalidEnvironment(resolution Resolution) {
	if !resolution.InvalidEnvironment || s.logger == nil {
		return
	}
	s.logger.Warn(
		"Ignoring invalid session capacity environment value",
		zap.String("environment_variable", EnvironmentVariable),
	)
}
