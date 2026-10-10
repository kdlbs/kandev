package messagequeue

import (
	"testing"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/stretchr/testify/require"
)

func TestServiceManagedInputStorage(t *testing.T) {
	var nilService *Service
	require.Nil(t, nilService.ManagedInputStorage())

	for _, factory := range autoRunRepositoryFactories {
		t.Run(factory.name, func(t *testing.T) {
			repository := factory.new(t)
			service := NewService(repository, DefaultMaxPerSession, logger.Default())

			storage := service.ManagedInputStorage()
			require.NotNil(t, storage)
			require.Same(t, repository, storage)

			unsupported := NewService(
				repositoryWithoutManagedInputStorage{Repository: repository},
				DefaultMaxPerSession,
				logger.Default(),
			)
			require.Nil(t, unsupported.ManagedInputStorage())
		})
	}
}

type repositoryWithoutManagedInputStorage struct {
	Repository
}
