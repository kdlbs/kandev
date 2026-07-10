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

// migrateInstanceSecrets rekeys a legacy workspace token only to its sole
// instance. The pre-instance schema allowed exactly one config per workspace,
// so a later second instance is not a migration target and must never inherit
// the old token. Completed rekeys remove the legacy keys; cardinality guards
// also make a restart safe if a cleanup attempt previously failed.
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
	instancesByWorkspace := make(map[string]int, len(instances))
	for _, inst := range instances {
		instancesByWorkspace[inst.WorkspaceID]++
	}
	allowSingleton := len(instances) == 1
	for _, inst := range instances {
		if instancesByWorkspace[inst.WorkspaceID] != 1 {
			continue
		}
		rekeyInstanceSecret(ctx, secrets, inst, allowSingleton, log)
	}
	cleanupLegacyInstanceSecretKeys(ctx, secrets, instances, log)
}

func rekeyInstanceSecret(ctx context.Context, secrets SecretStore, inst *SentryConfig, allowSingleton bool, log *logger.Logger) {
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
	value := revealLegacySecret(ctx, secrets, inst.WorkspaceID, allowSingleton)
	if value == "" {
		return
	}
	if err := secrets.Set(ctx, instanceKey, "Sentry auth token", value); err != nil {
		log.Warn("sentry: instance secret migration failed",
			zap.String("instance_id", inst.ID), zap.Error(err))
	}
}

// cleanupLegacyInstanceSecretKeys deletes a workspace key only after every
// instance in that workspace has an instance-scoped key. The global singleton
// key is likewise removed only after every instance has been rekeyed.
func cleanupLegacyInstanceSecretKeys(ctx context.Context, secrets SecretStore, instances []*SentryConfig, log *logger.Logger) {
	workspaceComplete := make(map[string]bool, len(instances))
	allComplete := len(instances) > 0
	for _, inst := range instances {
		if _, ok := workspaceComplete[inst.WorkspaceID]; !ok {
			workspaceComplete[inst.WorkspaceID] = true
		}
		exists, err := secrets.Exists(ctx, secretKeyForInstance(inst.ID))
		if err != nil {
			log.Warn("sentry: instance secret existence check failed during legacy cleanup",
				zap.String("instance_id", inst.ID), zap.Error(err))
			workspaceComplete[inst.WorkspaceID] = false
			allComplete = false
			continue
		}
		if !exists {
			workspaceComplete[inst.WorkspaceID] = false
			allComplete = false
		}
	}
	for workspaceID, complete := range workspaceComplete {
		if complete {
			deleteLegacySecret(ctx, secrets, SecretKeyForWorkspace(workspaceID), log)
		}
	}
	if allComplete {
		deleteLegacySecret(ctx, secrets, SecretKey, log)
	}
}

func deleteLegacySecret(ctx context.Context, secrets SecretStore, key string, log *logger.Logger) {
	exists, err := secrets.Exists(ctx, key)
	if err != nil {
		log.Warn("sentry: legacy secret existence check failed", zap.String("secret_key", key), zap.Error(err))
		return
	}
	if !exists {
		return
	}
	if err := secrets.Delete(ctx, key); err != nil {
		log.Warn("sentry: legacy secret cleanup failed", zap.String("secret_key", key), zap.Error(err))
	}
}

// revealLegacySecret reads the pre-instance token for a workspace. The
// install-wide singleton is considered only when it has one unambiguous
// instance destination; live reads use secretKeyForInstance only.
func revealLegacySecret(ctx context.Context, secrets SecretStore, workspaceID string, allowSingleton bool) string {
	keys := []string{SecretKeyForWorkspace(workspaceID)}
	if allowSingleton {
		keys = append(keys, SecretKey)
	}
	for _, key := range keys {
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
