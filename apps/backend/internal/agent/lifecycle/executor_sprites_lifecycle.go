package lifecycle

import (
	"context"
	"fmt"
	"strings"
	"time"

	sprites "github.com/superfly/sprites-go"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/scriptengine"
	spritesutil "github.com/kandev/kandev/internal/sprites"
)

func (r *SpritesExecutor) reconnectSprite(ctx context.Context, client *sprites.Client, name string) (*sprites.Sprite, error) {
	stepCtx, cancel := context.WithTimeout(ctx, spriteStepTimeout)
	defer cancel()
	sprite, err := client.GetSprite(stepCtx, name)
	if err != nil {
		return nil, fmt.Errorf("failed to reconnect sprite %q: %w", name, spritesutil.WrapNotFound(err))
	}
	return sprite, nil
}

func (r *SpritesExecutor) StopInstance(ctx context.Context, instance *ExecutorInstance, _ bool) error {
	if instance == nil {
		return nil
	}
	spriteName := getMetadataString(instance.Metadata, MetadataKeySpriteName)
	if spriteName == "" {
		return nil
	}

	// Always tear down the local proxy session — it's tied to the agentctl
	// instance we just stopped and would point at a stale TCP connection
	// after the agent process exits.
	r.mu.Lock()
	if proxy, ok := r.proxies[instance.InstanceID]; ok {
		r.closeProxySession(proxy)
		delete(r.proxies, instance.InstanceID)
	}
	r.mu.Unlock()

	// Plain "stop the agent" runs (e.g. user clicks Stop, then later wants to
	// resume) must NOT destroy the cloud sandbox: the user's working tree,
	// installed deps, and any in-progress files live there. Only destroy the
	// sandbox for explicit terminal lifecycle events (task/session deleted or
	// archived). Resume then re-attaches the same sandbox in seconds.
	if !shouldRunExecutorCleanup(instance.StopReason) {
		r.logger.Info("preserving sprite sandbox after agent stop",
			zap.String(MetadataKeySpriteName, spriteName),
			zap.String("instance_id", instance.InstanceID),
			zap.String("stop_reason", instance.StopReason))
		return nil
	}

	r.mu.RLock()
	token := r.tokens[instance.InstanceID]
	r.mu.RUnlock()
	if token == "" {
		token = r.resolveTokenFromMetadata(ctx, instance)
	}
	if token == "" {
		r.logger.Warn("no cached API token for sprite instance, cannot destroy",
			zap.String("instance_id", instance.InstanceID))
		return nil
	}
	client := sprites.New(token)
	sprite := client.Sprite(spriteName)
	r.runTerminalCleanupScript(ctx, sprite, instance)
	if err := sprite.Destroy(); err != nil {
		r.logger.Warn("failed to destroy sprite",
			zap.String(MetadataKeySpriteName, spriteName),
			zap.Error(err))
		return fmt.Errorf("failed to destroy sprite: %w", err)
	}

	r.mu.Lock()
	delete(r.tokens, instance.InstanceID)
	r.mu.Unlock()

	r.logger.Info("sprite destroyed", zap.String(MetadataKeySpriteName, spriteName),
		zap.String("stop_reason", instance.StopReason))
	return nil
}

func (r *SpritesExecutor) runTerminalCleanupScript(ctx context.Context, sprite *sprites.Sprite, instance *ExecutorInstance) {
	if sprite == nil || instance == nil {
		return
	}
	if !shouldRunExecutorCleanup(instance.StopReason) {
		return
	}
	script := strings.TrimSpace(getMetadataString(instance.Metadata, MetadataKeyCleanupScript))
	if script == "" {
		return
	}

	resolver := scriptengine.NewResolver().
		WithProvider(scriptengine.WorkspaceProvider(spritesWorkspacePath)).
		WithProvider(scriptengine.AgentctlProvider(r.agentctlPort, spritesWorkspacePath)).
		WithProvider(scriptengine.GitIdentityProvider(instance.Metadata)).
		WithProvider(scriptengine.RepositoryProvider(
			instance.Metadata,
			nil,
			getGitRemoteURL,
			injectGitHubTokenIntoCloneURL,
		))
	resolved := resolver.Resolve(script)
	if strings.TrimSpace(resolved) == "" {
		return
	}

	stepCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	out, err := sprite.CommandContext(stepCtx, "sh", "-c", resolved).CombinedOutput()
	if err != nil {
		r.logger.Warn("cleanup script failed in sprite",
			zap.String("instance_id", instance.InstanceID),
			zap.String("reason", instance.StopReason),
			zap.String("output", strings.TrimSpace(lastLines(string(out), spriteOutputMaxLines))),
			zap.Error(err))
		return
	}
	r.logger.Debug("cleanup script completed in sprite",
		zap.String("instance_id", instance.InstanceID),
		zap.String("reason", instance.StopReason))
}

// Terminal stop reasons that trigger destructive executor cleanup
// (sandbox teardown, container removal, per-instance session-dir removal).
// Anything outside this set is treated as a "preserve" stop — see
// shouldRunExecutorCleanup.
const (
	StopReasonTaskArchived    = "task archived"
	StopReasonTaskDeleted     = "task deleted"
	StopReasonSessionArchived = "session archived"
	StopReasonSessionDeleted  = "session deleted"
)

func shouldRunExecutorCleanup(reason string) bool {
	switch strings.ToLower(strings.TrimSpace(reason)) {
	case StopReasonTaskArchived, StopReasonTaskDeleted, StopReasonSessionArchived, StopReasonSessionDeleted:
		return true
	default:
		return false
	}
}

func (r *SpritesExecutor) GetRemoteStatus(ctx context.Context, instance *ExecutorInstance) (*RemoteStatus, error) {
	if instance == nil {
		return nil, fmt.Errorf("instance is nil")
	}
	spriteName := strings.TrimSpace(getMetadataString(instance.Metadata, MetadataKeySpriteName))
	if spriteName == "" {
		return &RemoteStatus{
			RuntimeName:   string(r.Name()),
			State:         "unknown",
			LastCheckedAt: time.Now().UTC(),
		}, nil
	}

	r.mu.RLock()
	token := r.tokens[instance.InstanceID]
	r.mu.RUnlock()
	if token == "" {
		token = r.resolveTokenFromMetadata(ctx, instance)
	}
	if token == "" {
		return &RemoteStatus{
			RuntimeName:   string(r.Name()),
			RemoteName:    spriteName,
			State:         "unknown",
			LastCheckedAt: time.Now().UTC(),
		}, nil
	}

	stepCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	client := sprites.New(token, sprites.WithDisableControl())
	sprite, err := client.GetSprite(stepCtx, spriteName)
	if err != nil {
		return nil, err
	}

	return &RemoteStatus{
		RuntimeName:   string(r.Name()),
		RemoteName:    spriteName,
		State:         spritesutil.NormalizeSpriteStatus(sprite.Status),
		CreatedAt:     nonZeroTimePtr(sprite.CreatedAt),
		LastCheckedAt: time.Now().UTC(),
	}, nil
}

// resolveTokenFromMetadata resolves the Sprites API token from the secret store
// using the secret ID persisted in metadata. This handles the post-restart case
// where the in-memory token cache is empty. On success, the token is cached.
func (r *SpritesExecutor) resolveTokenFromMetadata(ctx context.Context, instance *ExecutorInstance) string {
	if r.secretStore == nil || instance == nil {
		return ""
	}
	secretID := getMetadataString(instance.Metadata, "env_secret_id_SPRITES_API_TOKEN")
	if secretID == "" {
		return ""
	}
	revealed, err := r.secretStore.Reveal(ctx, secretID)
	if err != nil || revealed == "" {
		return ""
	}
	r.mu.Lock()
	r.tokens[instance.InstanceID] = revealed
	r.mu.Unlock()
	return revealed
}

func nonZeroTimePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (r *SpritesExecutor) cleanupOnFailure(_ context.Context, sprite *sprites.Sprite, instanceID string, destroySprite bool) {
	if sprite == nil {
		return
	}
	r.logger.Warn("cleaning up sprite after failure", zap.String("instance_id", instanceID))

	r.mu.Lock()
	if proxy, ok := r.proxies[instanceID]; ok {
		r.closeProxySession(proxy)
		delete(r.proxies, instanceID)
	}
	delete(r.tokens, instanceID)
	r.mu.Unlock()

	if !destroySprite {
		r.logger.Info("preserving sprite during cleanup (reconnect flow)",
			zap.String("instance_id", instanceID))
		return
	}

	if err := sprite.Destroy(); err != nil {
		r.logger.Warn("failed to destroy sprite during cleanup", zap.Error(err))
	}
}

func (r *SpritesExecutor) closeProxySession(proxy *SpritesProxySession) {
	if proxy == nil {
		return
	}
	if proxy.cancel != nil {
		proxy.cancel()
	}
	if proxy.proxySession != nil {
		_ = proxy.proxySession.Close()
	}
}
