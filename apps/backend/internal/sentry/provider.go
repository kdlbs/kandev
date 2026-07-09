package sentry

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

// mockEnvVar gates the in-memory mock client used in E2E tests.
const mockEnvVar = "KANDEV_MOCK_SENTRY"

// MockEnabled reports whether KANDEV_MOCK_SENTRY is set to "true".
func MockEnabled() bool {
	return os.Getenv(mockEnvVar) == "true"
}

// Provide builds the Sentry service. eventBus is used by the Phase 2 issue
// watcher to publish NewSentryIssueEvent for the orchestrator to consume.
// Cleanup is a no-op — the service holds only in-memory client caches.
//
// When KANDEV_MOCK_SENTRY=true, the service is wired to a process-wide
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
		log.Info("sentry: using in-memory mock client (KANDEV_MOCK_SENTRY=true)")
	}
	svc := NewService(store, secrets, clientFn, log)
	svc.mockClient = mock
	if eventBus != nil {
		svc.SetEventBus(eventBus)
	}
	cleanup := func() error { return nil }
	return svc, cleanup, nil
}

func migrateLegacySecret(store *Store, secrets SecretStore, log *logger.Logger) {
	target := store.MigratedFromWorkspace()
	if target == "" || secrets == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	targetKey := SecretKeyForWorkspace(target)
	if exists, err := secrets.Exists(ctx, targetKey); err == nil && exists {
		return
	}
	value, err := secrets.Reveal(ctx, SecretKey)
	if err != nil || value == "" {
		return
	}
	if err := secrets.Set(ctx, targetKey, "Sentry auth token", value); err != nil {
		log.Warn("sentry: legacy secret migration failed", zap.Error(err))
		return
	}
	if err := secrets.Delete(ctx, SecretKey); err != nil {
		log.Warn("sentry: legacy secret cleanup failed", zap.Error(err))
	}
}

// RegisterMockRoutes mounts the mock control routes when the service was built
// with a MockClient. No-op otherwise.
func RegisterMockRoutes(router *gin.Engine, svc *Service, log *logger.Logger) {
	mock := svc.MockClient()
	if mock == nil {
		return
	}
	NewMockController(mock, svc.Store(), log).RegisterRoutes(router)
	log.Info("registered Sentry mock control endpoints")
}
