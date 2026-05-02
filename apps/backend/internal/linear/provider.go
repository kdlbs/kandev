package linear

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
)

// mockEnvVar gates the in-memory mock client used in E2E tests.
const mockEnvVar = "KANDEV_MOCK_LINEAR"

// MockEnabled reports whether KANDEV_MOCK_LINEAR is set to "true".
func MockEnabled() bool {
	return os.Getenv(mockEnvVar) == "true"
}

// Provide builds the Linear service. Cleanup is a no-op today — the service
// holds only in-memory client caches — but the signature mirrors other
// integration providers so callers can register it uniformly.
//
// When KANDEV_MOCK_LINEAR=true, the service is wired to a process-wide
// MockClient and the same instance is exposed via Service.MockClient() so the
// E2E mock controller can drive it.
func Provide(writer, reader *sqlx.DB, secrets SecretStore, log *logger.Logger) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	clientFn := DefaultClientFactory
	var mock *MockClient
	if MockEnabled() {
		mock = NewMockClient()
		clientFn = MockClientFactory(mock)
		log.Info("linear: using in-memory mock client (KANDEV_MOCK_LINEAR=true)")
	}
	svc := NewService(store, secrets, clientFn, log)
	svc.mockClient = mock
	cleanup := func() error { return nil }
	return svc, cleanup, nil
}

// RegisterMockRoutes mounts the mock control routes when the service was built
// with a MockClient. No-op otherwise.
func RegisterMockRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	mock := svc.MockClient()
	if mock == nil {
		return
	}
	NewMockController(mock, svc.Store(), log).RegisterRoutes(router)
	log.Info("registered Linear mock control endpoints")
}
