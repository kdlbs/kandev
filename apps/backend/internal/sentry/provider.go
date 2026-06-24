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

// migrateLegacySecret moves the install-wide singleton token onto its
// per-instance key after the singleton→multi-instance upgrade, then deletes the
// legacy entry. Crash-safe: the target id normally comes from the in-memory
// migration record, but if that was lost (process crashed after the DB rebuild
// but before this ran), it recovers the mapping only when unambiguous — exactly
// one configured instance with no per-instance secret yet. Best-effort: any
// error is logged; worst case the user re-pastes the token in settings.
func migrateLegacySecret(store *Store, secrets SecretStore, log *logger.Logger) {
	if secrets == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	target := store.MigratedSingletonID()
	if target == "" {
		configs, err := store.ListConfigs(ctx)
		if err != nil || len(configs) != 1 {
			return
		}
		target = configs[0].ID
	}
	if exists, err := secrets.Exists(ctx, secretKeyFor(target)); err == nil && exists {
		return
	}
	value, err := secrets.Reveal(ctx, legacySecretKey)
	if err != nil || value == "" {
		return
	}
	if err := secrets.Set(ctx, secretKeyFor(target), "Sentry auth token", value); err != nil {
		log.Warn("sentry: legacy secret migration failed", zap.Error(err))
		return
	}
	if err := secrets.Delete(ctx, legacySecretKey); err != nil {
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
