package slack

import (
	"context"
	"time"

	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
)

// Provide builds the Slack service. Cleanup is a no-op — the service holds
// only an in-memory client cache. Mirrors the other integration providers so
// callers register it uniformly.
//
// The agent runner is wired post-construction via SetRunner because the
// host-utility manager is built later in the backend's startup sequence.
func Provide(writer, reader *sqlx.DB, secrets SecretStore, log *logger.Logger) (*Service, func() error, error) {
	store, err := NewStore(writer, reader)
	if err != nil {
		return nil, nil, err
	}
	migrateLegacySecrets(store, secrets, log)
	svc := NewService(store, secrets, nil, DefaultClientFactory, log)
	cleanup := func() error { return nil }
	return svc, cleanup, nil
}

// migrateLegacySecrets copies the old install-wide token + cookie onto the
// workspace-scoped keys created by the config migration, then deletes the
// legacy entries. Best-effort: any error is logged and ignored — the worst
// case is the user re-pastes their credentials in the settings UI.
func migrateLegacySecrets(store *Store, secrets SecretStore, log *logger.Logger) {
	target := store.MigratedFromWorkspace()
	if target == "" || secrets == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	migrateOne(ctx, secrets, SecretKeyToken, SecretKeyForToken(target), "Slack token", log)
	migrateOne(ctx, secrets, SecretKeyCookie, SecretKeyForCookie(target), "Slack d cookie", log)
}

func migrateOne(ctx context.Context, secrets SecretStore, legacyKey, newKey, displayName string, log *logger.Logger) {
	// If a fresh value already exists at the new key (someone migrated
	// manually, or this codepath ran on a previous boot), don't clobber it.
	if exists, err := secrets.Exists(ctx, newKey); err == nil && exists {
		return
	}
	value, err := secrets.Reveal(ctx, legacyKey)
	if err != nil || value == "" {
		return
	}
	if err := secrets.Set(ctx, newKey, displayName, value); err != nil {
		log.Warn("slack: legacy secret migration failed",
			zap.String("key", legacyKey), zap.Error(err))
		return
	}
	if err := secrets.Delete(ctx, legacyKey); err != nil {
		log.Warn("slack: legacy secret cleanup failed",
			zap.String("key", legacyKey), zap.Error(err))
	}
}
