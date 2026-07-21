package azuredevops

import (
	"os"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

const mockEnvVar = "KANDEV_MOCK_AZURE_DEVOPS"

// Provide constructs the Azure DevOps service. The event bus is accepted to
// match other integration providers; Task 01 does not publish events.
func Provide(
	writer *sqlx.DB,
	reader *sqlx.DB,
	secrets SecretStore,
	_ bus.EventBus,
	log *logger.Logger,
) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	var factory ClientFactory
	var mock *MockClient
	if os.Getenv(mockEnvVar) == "true" {
		mock = NewMockClient()
		factory = func(*Config, string) Client { return mock }
	}
	service := NewService(store, secrets, factory, log)
	service.mock = mock
	return service, func() error { return nil }, nil
}
