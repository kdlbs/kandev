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
	AutoMergeEnabled() bool
	SetAutoMergeEnabled(bool)
}
type revisionedAutoMergeTarget interface {
	SetAutoMergePolicy(enabled bool, revision int64)
}
type autoMergePolicyLoaderTarget interface {
	SetAutoMergePolicyLoader(func(context.Context) (bool, int64, error))
}

type EnvironmentReader func() Environment

type Service struct {
	mu              sync.Mutex
	store           *Store
	target          Target
	readEnvironment EnvironmentReader
	configuration   Configuration
	logger          *logger.Logger
}

func NewService(
	store *Store,
	target Target,
	readEnvironment EnvironmentReader,
	log *logger.Logger,
	startup ...Configuration,
) *Service {
	if readEnvironment == nil {
		readEnvironment = ReadEnvironment
	}
	var configuration Configuration
	if len(startup) > 0 {
		configuration = startup[0]
	}
	service := &Service{
		store: store, target: target, readEnvironment: readEnvironment,
		configuration: configuration, logger: log,
	}
	if target, ok := target.(autoMergePolicyLoaderTarget); ok {
		target.SetAutoMergePolicyLoader(service.loadAutoMergePolicy)
	}
	return service
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
	resolution, err := Resolve(configured, s.readEnvironment(), s.configuration)
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
// attempts to change max_per_session. It controls neither merge setting.
func (s *Service) Update(ctx context.Context, patch SettingsPatch) (Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.target == nil {
		return Response{}, ErrTargetUnavailable
	}
	environment := s.readEnvironment()
	settings, err := s.store.Update(ctx, func(current *Settings) (Settings, error) {
		resolution, resolveErr := Resolve(current, environment, s.configuration)
		if resolveErr != nil {
			return Settings{}, resolveErr
		}
		s.warnInvalidEnvironment(resolution)
		if patch.MaxPerSession != nil && resolution.Effective.Locked {
			if resolution.Effective.Source == SourceConfiguration {
				return Settings{}, ErrConfigurationLocked
			}
			return Settings{}, ErrEnvironmentLocked
		}
		updated := patch.Apply(resolution.Settings)
		if patch.AutoMergeEnabled != nil && *patch.AutoMergeEnabled != resolution.Settings.AutoMergeEnabled {
			updated.AutoMergeRevision = resolution.Settings.AutoMergeRevision + 1
		}
		if validateErr := Validate(updated); validateErr != nil {
			return Settings{}, validateErr
		}
		return updated, nil
	})
	if err != nil {
		return Response{}, err
	}
	updated, err := Resolve(&settings, environment, s.configuration)
	if err != nil {
		return Response{}, err
	}
	s.target.SetMaxPerSession(updated.Effective.MaxPerSession)
	s.target.SetMergeEnabled(updated.Effective.MergeEnabled)
	if target, ok := s.target.(revisionedAutoMergeTarget); ok {
		target.SetAutoMergePolicy(updated.Effective.AutoMergeEnabled, settings.AutoMergeRevision)
	} else {
		s.target.SetAutoMergeEnabled(updated.Effective.AutoMergeEnabled)
	}
	return updated.Response, nil
}

func (s *Service) loadAutoMergePolicy(ctx context.Context) (bool, int64, error) {
	configured, err := s.loadConfiguredConsistent(ctx)
	if err != nil {
		return false, 0, err
	}
	resolution, err := Resolve(configured, s.readEnvironment(), s.configuration)
	if err != nil {
		return false, 0, err
	}
	return resolution.Effective.AutoMergeEnabled, resolution.Settings.AutoMergeRevision, nil
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

func (s *Service) loadConfiguredConsistent(ctx context.Context) (*Settings, error) {
	configured, err := s.store.LoadConsistent(ctx)
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
