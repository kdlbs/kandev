package gitlab

import (
	"context"

	"github.com/kandev/kandev/internal/common/logger"
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
		if persisted, err := hostStore.GetHost(ctx); err == nil && persisted != "" {
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
