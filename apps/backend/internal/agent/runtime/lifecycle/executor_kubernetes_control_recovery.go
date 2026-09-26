package lifecycle

import (
	"context"
	"errors"
	"github.com/kandev/kandev/internal/secrets"
)

// Keep the single-use handshake result until storage acknowledges it. All callers
// hold the environment's control lock, so a sibling retries the same credential.
func (r *KubernetesExecutor) persistSharedKubernetesControlToken(ctx context.Context, id, token string) error {
	r.mu.Lock()
	if r.pendingControlTokens == nil {
		r.pendingControlTokens = make(map[string]string)
	}
	r.pendingControlTokens[id] = token
	r.mu.Unlock()
	persistCtx, cancel := kubernetesDurableContext(ctx)
	defer cancel()
	if err := r.secretStore.Update(persistCtx, id, &secrets.UpdateSecretRequest{Value: &token}); err != nil {
		return errors.Join(err, r.saveKubernetesControlRecovery(persistCtx, id, token))
	}
	if err := r.secretStore.Delete(persistCtx, kubernetesControlRecoverySecretID(id)); err != nil && !errors.Is(err, secrets.ErrNotFound) {
		return err
	}
	r.mu.Lock()
	delete(r.pendingControlTokens, id)
	r.mu.Unlock()
	return nil
}

func kubernetesControlRecoverySecretID(id string) string {
	if id == "" {
		return ""
	}
	return id + ":recovery"
}

// A failed canonical update must not discard the credential after nonce
// consumption. The deterministic encrypted recovery record survives restart.
func (r *KubernetesExecutor) saveKubernetesControlRecovery(ctx context.Context, id, token string) error {
	recoveryID := kubernetesControlRecoverySecretID(id)
	_, err := r.secretStore.Get(ctx, recoveryID)
	if errors.Is(err, secrets.ErrNotFound) {
		return r.secretStore.Create(ctx, &secrets.SecretWithValue{Secret: secrets.Secret{ID: recoveryID, Name: recoveryID}, Value: token})
	}
	if err != nil {
		return err
	}
	return r.secretStore.Update(ctx, recoveryID, &secrets.UpdateSecretRequest{Value: &token})
}
