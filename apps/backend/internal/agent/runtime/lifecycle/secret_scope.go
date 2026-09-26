package lifecycle

import (
	"context"

	"github.com/kandev/kandev/internal/secrets"
)

func revealGlobalSecret(ctx context.Context, store secrets.SecretStore, id string) (string, error) {
	if scoped, ok := store.(secrets.ScopedSecretStore); ok {
		return scoped.RevealGlobal(ctx, id)
	}
	return store.Reveal(ctx, id)
}
