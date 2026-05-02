package jira

import (
	"os"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
)

// mockEnvVar gates the in-memory mock client used in E2E tests. Production
// builds never set this — the real CloudClient hits Atlassian.
const mockEnvVar = "KANDEV_MOCK_JIRA"

// MockEnabled reports whether KANDEV_MOCK_JIRA is set to "true". Exposed so
// route registration can branch on the same signal Provide uses.
func MockEnabled() bool {
	return os.Getenv(mockEnvVar) == "true"
}

// Provide builds the Jira service. eventBus may be nil — used in tests and
// during early boot before the bus is ready; the service falls back to a
// no-op publish path. Cleanup is a no-op today — the service holds only
// in-memory client caches — but the signature mirrors other integration
// providers so callers can register it uniformly.
//
// When KANDEV_MOCK_JIRA=true, the service is wired to a process-wide
// MockClient and the same instance is exposed via Service.MockClient() so the
// E2E mock controller can drive it.
func Provide(writer, reader *sqlx.DB, secrets SecretStore, eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	clientFn := DefaultClientFactory
	var mock *MockClient
	if MockEnabled() {
		mock = NewMockClient()
		clientFn = MockClientFactory(mock)
		log.Info("jira: using in-memory mock client (KANDEV_MOCK_JIRA=true)")
	}
	svc := NewService(store, secrets, clientFn, log)
	svc.mockClient = mock
	if eventBus != nil {
		svc.SetEventBus(eventBus)
	}
	cleanup := func() error { return nil }
	return svc, cleanup, nil
}

// RegisterMockRoutes mounts the mock control routes when the service was built
// with a MockClient. No-op otherwise — production builds skip this entirely.
func RegisterMockRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	mock := svc.MockClient()
	if mock == nil {
		return
	}
	NewMockController(mock, svc.Store(), log).RegisterRoutes(router)
	log.Info("registered Jira mock control endpoints")
}
