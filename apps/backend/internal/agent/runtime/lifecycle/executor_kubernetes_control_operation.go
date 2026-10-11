package lifecycle

import (
	"context"
	"errors"
	"net/http"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
)

// Serialize token recovery with the operation so siblings cannot consume one
// bootstrap nonce twice or publish a stale credential after recovery.
func (r *KubernetesExecutor) withSharedKubernetesControlAuth(ctx context.Context, req *ExecutorCreateRequest, control *agentctl.ControlClient, operation func() error) (string, error) {
	unlock := r.lockInstance("control:" + req.TaskEnvironmentID)
	defer unlock()
	token, err := r.sharedKubernetesControlToken(ctx, req)
	if err != nil {
		return "", err
	}
	control.SetAuthToken(token)
	err = operation()
	var response *agentctl.ControlHTTPError
	if !errors.As(err, &response) || response.Status != http.StatusUnauthorized {
		return token, err
	}
	nonce := req.BootstrapNonce
	if nonce == "" && r.secretStore != nil {
		id := getMetadataString(req.Metadata, MetadataKeyBootstrapNonceSecret)
		if id != "" {
			var revealErr error
			nonce, revealErr = r.secretStore.Reveal(ctx, id)
			if revealErr != nil && nonce == "" {
				return "", revealErr
			}
		}
	}
	if nonce == "" {
		return "", errors.New("kubernetes control bootstrap nonce is unavailable")
	}
	token, err = control.Handshake(ctx, nonce)
	if err != nil {
		return "", err
	}
	id := getMetadataString(req.Metadata, MetadataKeyAuthTokenSecret)
	if r.secretStore == nil || id == "" {
		return "", errors.New("kubernetes control credential store is unavailable")
	}
	if err := r.persistSharedKubernetesControlToken(ctx, id, token); err != nil {
		return "", err
	}
	return token, operation()
}
