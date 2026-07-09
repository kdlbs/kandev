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

// Provide builds the Sentry service. eventBus is used by the issue watcher to
// publish NewSentryIssueEvent for the orchestrator to consume. Cleanup is a
// no-op — the service holds only in-memory client caches.
//
// When KANDEV_MOCK_SENTRY=true, the service is wired to a process-wide
// MockClient and the same instance is exposed via Service.MockClient() so the
// E2E mock controller can drive it.
func Provide(writer, reader *sqlx.DB, secrets SecretStore, eventBus bus.EventBus, log *logger.Logger) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	// Two-stage secret migration mirroring the schema migration lineage:
	// singleton→workspace key (kept from PR #1572), then workspace/singleton
	// key→per-instance key for the multi-instance model.
	migrateLegacySecret(store, secrets, log)
	migrateInstanceSecrets(store, secrets, log)
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

// migrateLegacySecret rekeys the pre-workspace install-wide token from the
// singleton secret key to the workspace key that received the migrated config
// row. Retained from the workspace-scoped model so the subsequent per-instance
// rekey has a workspace key to read.
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

// migrateInstanceSecrets rekeys each instance's auth token from the legacy
// secret keys (workspace-scoped, or the install-wide singleton) to the
// per-instance key. Derived purely from the post-migration instance rows so it
// re-runs safely after a crash: an instance whose per-instance key already
// exists is skipped. This is the only live code path that reads the legacy keys.
func migrateInstanceSecrets(store *Store, secrets SecretStore, log *logger.Logger) {
	if secrets == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	instances, err := store.ListAllInstances(ctx)
	if err != nil {
		log.Warn("sentry: list instances for secret migration failed", zap.Error(err))
		return
	}
	for _, inst := range instances {
		rekeyInstanceSecret(ctx, secrets, inst, log)
	}
}

func rekeyInstanceSecret(ctx context.Context, secrets SecretStore, inst *SentryConfig, log *logger.Logger) {
	instanceKey := secretKeyForInstance(inst.ID)
	exists, err := secrets.Exists(ctx, instanceKey)
	if err != nil {
		log.Warn("sentry: instance secret existence check failed",
			zap.String("instance_id", inst.ID), zap.Error(err))
		return
	}
	if exists {
		return
	}
	value := revealLegacySecret(ctx, secrets, inst.WorkspaceID)
	if value == "" {
		return
	}
	if err := secrets.Set(ctx, instanceKey, "Sentry auth token", value); err != nil {
		log.Warn("sentry: instance secret migration failed",
			zap.String("instance_id", inst.ID), zap.Error(err))
	}
}

// revealLegacySecret reads the pre-instance token for a workspace, trying the
// workspace-scoped key first, then the install-wide singleton. Confined to the
// migration path: live reads use secretKeyForInstance only.
func revealLegacySecret(ctx context.Context, secrets SecretStore, workspaceID string) string {
	for _, key := range []string{SecretKeyForWorkspace(workspaceID), SecretKey} {
		exists, err := secrets.Exists(ctx, key)
		if err != nil || !exists {
			continue
		}
		if value, err := secrets.Reveal(ctx, key); err == nil && value != "" {
			return value
		}
	}
	return ""
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
