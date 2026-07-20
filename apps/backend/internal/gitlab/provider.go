package gitlab

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/integrations/workspacescope"
)

// Provide builds the GitLab service stack: discovers a host (via the
// optional HostStore, falling back to DefaultHost), resolves the best
// available client, and returns a *Service plus a cleanup function.
//
// secrets is required for PAT-fallback auth; passing nil disables it.
// hostStore is optional — when present, the persisted host is used
// instead of DefaultHost.
func Provide(
	ctx context.Context,
	secrets SecretProvider,
	hostStore HostStore,
	log *logger.Logger,
) (*Service, func() error, error) {
	host := DefaultHost
	if hostStore != nil {
		persisted, err := hostStore.GetHost(ctx)
		if err != nil {
			return nil, nil, fmt.Errorf("load GitLab host: %w", err)
		}
		if persisted != "" {
			host = persisted
		}
	}

	client, authMethod, err := NewClient(ctx, host, secrets, log)
	if err != nil {
		log.Warn("GitLab client not available: " + err.Error())
	}

	svc := NewService(host, client, authMethod, secrets, log)
	if hostStore != nil {
		svc.SetHostStore(hostStore)
	}

	cleanup := func() error { return nil }
	return svc, cleanup, nil
}

// MigrateLegacyConnection moves the installation-wide host/token onto the
// deterministic active-or-earliest workspace. Existing workspace config wins.
func MigrateLegacyConnection(ctx context.Context, store *Store, workspaceSecrets WorkspaceSecretStore, legacySecrets SecretProvider, legacyManager SecretManager, hostStore HostStore, log *logger.Logger) error {
	if store == nil {
		return nil
	}
	target, err := workspacescope.ResolveMigrationTarget(store.db)
	if err != nil {
		return err
	}
	existing, err := store.GetConfigForWorkspace(ctx, target)
	if err != nil || existing != nil {
		return err
	}
	legacy, err := loadLegacyConnection(ctx, hostStore, legacySecrets, workspaceSecrets)
	if err != nil {
		return err
	}
	if legacy == nil {
		return nil
	}
	var previousSecret workspaceSecretSnapshot
	if legacy.token != "" {
		previousSecret, err = snapshotWorkspaceSecret(ctx, workspaceSecrets, SecretKeyForWorkspace(target))
		if err != nil {
			return fmt.Errorf("snapshot migrated GitLab token: %w", err)
		}
		if err := workspaceSecrets.Set(ctx, SecretKeyForWorkspace(target), "GitLab token", legacy.token); err != nil {
			return restoreLegacyWorkspaceSecret(ctx, workspaceSecrets, target, previousSecret, err)
		}
	}
	cfg := &GitLabConfig{Host: legacy.host, AuthMethod: legacy.authMethod, CreatedAt: time.Now().UTC()}
	if err := store.UpsertConfigForWorkspace(ctx, target, cfg); err != nil {
		if legacy.token != "" {
			return restoreLegacyWorkspaceSecret(ctx, workspaceSecrets, target, previousSecret, err)
		}
		return err
	}
	cleanupLegacyConnection(ctx, legacy.tokenID, legacyManager, hostStore, log)
	return nil
}

func restoreLegacyWorkspaceSecret(ctx context.Context, secrets WorkspaceSecretStore, workspaceID string, snapshot workspaceSecretSnapshot, cause error) error {
	rollbackCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := restoreWorkspaceSecret(rollbackCtx, secrets, SecretKeyForWorkspace(workspaceID), snapshot); err != nil {
		return errors.Join(cause, fmt.Errorf("restore migrated GitLab token: %w", err))
	}
	return cause
}

type legacyConnection struct {
	host       string
	authMethod string
	token      string
	tokenID    string
}

func loadLegacyConnection(ctx context.Context, hostStore HostStore, legacySecrets SecretProvider, workspaceSecrets WorkspaceSecretStore) (*legacyConnection, error) {
	host, err := loadLegacyHost(ctx, hostStore)
	if err != nil {
		return nil, err
	}
	token, tokenID, err := findLegacyPAT(ctx, legacySecrets)
	if err != nil {
		return nil, err
	}
	environmentToken := strings.TrimSpace(os.Getenv(secretNameToken))
	if host == "" && token == "" && environmentToken == "" {
		return nil, nil
	}
	host, err = normalizeHostOrigin(host)
	if err != nil {
		return nil, err
	}
	authMethod, err := legacyAuthMethod(token, environmentToken, workspaceSecrets)
	if err != nil {
		return nil, err
	}
	return &legacyConnection{host: host, authMethod: authMethod, token: token, tokenID: tokenID}, nil
}

func loadLegacyHost(ctx context.Context, hostStore HostStore) (string, error) {
	if hostStore == nil {
		return "", nil
	}
	return hostStore.GetHost(ctx)
}

func legacyAuthMethod(token, environmentToken string, workspaceSecrets WorkspaceSecretStore) (string, error) {
	if token != "" {
		if workspaceSecrets == nil {
			return "", errors.New("gitlab workspace secret store is required to migrate a PAT")
		}
		return AuthMethodPAT, nil
	}
	if environmentToken != "" {
		return AuthMethodEnvironment, nil
	}
	return AuthMethodGLab, nil
}

func cleanupLegacyConnection(ctx context.Context, tokenID string, legacyManager SecretManager, hostStore HostStore, log *logger.Logger) {
	if tokenID != "" && legacyManager != nil {
		if err := legacyManager.Delete(ctx, tokenID); err != nil && log != nil {
			log.Warn("GitLab legacy token cleanup failed")
		}
	}
	if hostStore != nil {
		_ = hostStore.SetHost(ctx, "")
	}
}

func findLegacyPAT(ctx context.Context, secrets SecretProvider) (string, string, error) {
	if secrets == nil {
		return "", "", nil
	}
	items, err := secrets.List(ctx)
	if err != nil {
		return "", "", err
	}
	for _, item := range items {
		if item.HasValue && (item.Name == secretNameToken || item.Name == secretNameTokenLower) {
			value, err := secrets.Reveal(ctx, item.ID)
			return value, item.ID, err
		}
	}
	return "", "", nil
}
