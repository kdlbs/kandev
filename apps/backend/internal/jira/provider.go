package jira

import (
	"context"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

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
	migrateLegacySecret(store, secrets, log)
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

// migrateLegacySecret copies the per-workspace token from the pre-singleton
// secret key onto the new install-wide key, then deletes the legacy entry.
// Best-effort: any error is logged and ignored — the worst case is the user
// re-pastes their token in the settings UI.
func migrateLegacySecret(store *Store, secrets SecretStore, log *logger.Logger) {
	source := store.MigratedFromWorkspace()
	if source == "" || secrets == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if exists, err := secrets.Exists(ctx, SecretKey); err == nil && exists {
		return
	}
	legacyKey := LegacySecretKeyForWorkspace(source)
	value, err := secrets.Reveal(ctx, legacyKey)
	if err != nil || value == "" {
		return
	}
	if err := secrets.Set(ctx, SecretKey, "Jira token", value); err != nil {
		log.Warn("jira: legacy secret migration failed", zap.Error(err))
		return
	}
	if err := secrets.Delete(ctx, legacyKey); err != nil {
		log.Warn("jira: legacy secret cleanup failed", zap.Error(err))
	}
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
