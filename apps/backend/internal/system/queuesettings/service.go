package queuesettings

import (
	"context"
	"errors"
	"os"
	"sync"

	"github.com/kandev/kandev/internal/common/logger"
	"go.uber.org/zap"
)

type Target interface {
	MaxPerSession() int
	SetMaxPerSession(int)
	MergeEnabled() bool
	SetMergeEnabled(bool)
}

type EnvironmentReader func() Environment

type Service struct {
	mu              sync.Mutex
	store           *Store
	target          Target
	readEnvironment EnvironmentReader
	logger          *logger.Logger
}

func NewService(store *Store, target Target, readEnvironment EnvironmentReader, log *logger.Logger) *Service {
	if readEnvironment == nil {
		readEnvironment = ReadEnvironment
	}
	return &Service{store: store, target: target, readEnvironment: readEnvironment, logger: log}
}

func ReadEnvironment() Environment {
	value, present := os.LookupEnv(EnvironmentVariable)
	return Environment{Value: value, Present: present}
}

func (s *Service) Get(ctx context.Context) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	configured, err := s.loadConfigured(ctx)
	if err != nil {
		return Response{}, err
	}
	resolution, err := Resolve(configured, s.readEnvironment())
	if err != nil {
		return Response{}, err
	}
	s.warnInvalidEnvironment(resolution)
	return resolution.Response, nil
}

// Update applies a partial patch to the persisted settings: fields the caller
// omits keep their current effective value rather than resetting to zero, so
// a client that edits only one field can never silently clobber another (see
// SettingsPatch). The environment lock only blocks a patch that actually
// attempts to change max_per_session — it does not control merge_enabled.
func (s *Service) Update(ctx context.Context, patch SettingsPatch) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	environment := s.readEnvironment()
	current, err := s.loadConfigured(ctx)
	if err != nil {
		return Response{}, err
	}
	resolution, err := Resolve(current, environment)
	if err != nil {
		return Response{}, err
	}
	s.warnInvalidEnvironment(resolution)
	if patch.MaxPerSession != nil && resolution.Effective.Locked {
		return Response{}, ErrEnvironmentLocked
	}
	settings := patch.Apply(resolution.Settings)
	if err := Validate(settings); err != nil {
		return Response{}, err
	}
	if s.target == nil {
		return Response{}, ErrTargetUnavailable
	}
	if err := s.store.Save(ctx, settings); err != nil {
		return Response{}, err
	}
	s.target.SetMaxPerSession(settings.MaxPerSession)
	s.target.SetMergeEnabled(settings.MergeEnabled)
	updated, err := Resolve(&settings, environment)
	if err != nil {
		return Response{}, err
	}
	return updated.Response, nil
}

func (s *Service) loadConfigured(ctx context.Context) (*Settings, error) {
	configured, err := s.store.Load(ctx)
	if err == nil {
		return configured, nil
	}
	if !errors.Is(err, ErrInvalidPersisted) {
		return nil, err
	}
	if s.logger != nil {
		s.logger.Warn("Ignoring invalid persisted message queue settings", zap.Error(err))
	}
	return nil, nil
}

func (s *Service) warnInvalidEnvironment(resolution Resolution) {
	if !resolution.InvalidEnvironment || s.logger == nil {
		return
	}
	s.logger.Warn("Ignoring invalid message queue capacity environment value",
		zap.String("environment_variable", EnvironmentVariable))
}
