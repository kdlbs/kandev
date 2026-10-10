package lifecycle

import (
	"context"
	"strings"
	"time"

	spritesapi "github.com/superfly/sprites-go"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	spritesutil "github.com/kandev/kandev/internal/sprites"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *SpritesExecutor) setSpritesClientFactory(factory func(string) *spritesapi.Client) {
	r.mu.Lock()
	r.recoveryClientFactory = factory
	r.mu.Unlock()
}

func (r *SpritesExecutor) recoveryClient(token string) *spritesapi.Client {
	r.mu.RLock()
	factory := r.recoveryClientFactory
	r.mu.RUnlock()
	if factory != nil {
		return factory(token)
	}
	return spritesapi.New(token, spritesapi.WithDisableControl())
}

func (r *SpritesExecutor) RecoverInstancesDetailed(
	ctx context.Context,
	records []*models.ExecutorRunning,
) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var instances []*ExecutorInstance
	outcomes := make(map[string]RecoveryCandidateOutcome)
	for _, record := range records {
		if record == nil || record.Runtime != agentruntime.RuntimeSprites || record.SessionID == "" {
			continue
		}
		instance, outcome := r.recoverSpriteRecord(ctx, record)
		if instance != nil {
			delete(outcomes, record.SessionID)
			instances = append(instances, instance)
			continue
		}
		outcomes[record.SessionID] = outcome
	}
	return instances, outcomes, nil
}

func (r *SpritesExecutor) recoverSpriteRecord(
	ctx context.Context,
	record *models.ExecutorRunning,
) (*ExecutorInstance, RecoveryCandidateOutcome) {
	instanceID, spriteName, ok := spritesRecoveryIdentity(record)
	if !ok {
		return nil, RecoveryOutcomeUnknown
	}
	if r.hasSpritesProxy(instanceID) {
		return nil, RecoveryOutcomeUnknown
	}

	probeCtx, cancel := context.WithTimeout(ctx, spriteHealthTimeout)
	defer cancel()
	sprite, instanceInfo, outcome := r.inspectSavedSprite(probeCtx, record, instanceID, spriteName)
	if outcome != "" {
		return nil, outcome
	}
	if err := ctx.Err(); err != nil {
		return nil, RecoveryOutcomeUnknown
	}
	return r.attachRecoveredSprite(ctx, record, spriteName, sprite, instanceInfo)
}

func spritesRecoveryIdentity(record *models.ExecutorRunning) (string, string, bool) {
	if record == nil {
		return "", "", false
	}
	instanceID := strings.TrimSpace(record.AgentExecutionID)
	spriteName := strings.TrimSpace(getMetadataString(record.Metadata, MetadataKeySpriteName))
	if instanceID == "" || instanceID != record.AgentExecutionID || record.TaskID == "" ||
		strings.TrimSpace(record.SessionID) != record.SessionID || strings.TrimSpace(record.TaskID) != record.TaskID || spriteName == "" {
		return "", "", false
	}
	return instanceID, spriteName, true
}

func (r *SpritesExecutor) inspectSavedSprite(
	ctx context.Context,
	record *models.ExecutorRunning,
	instanceID string,
	spriteName string,
) (*spritesapi.Sprite, *agentctl.InstanceInfo, RecoveryCandidateOutcome) {
	stored := &ExecutorInstance{InstanceID: instanceID, Metadata: record.Metadata}
	token := r.resolveTokenFromMetadata(ctx, stored)
	if token == "" {
		return nil, nil, RecoveryOutcomeUnknown
	}
	client := r.recoveryClient(token)
	sprite, err := client.GetSprite(ctx, spriteName)
	if err != nil {
		if isSpritesNotFound(err) {
			return nil, nil, RecoveryOutcomeNoMatchingInstance
		}
		return nil, nil, RecoveryOutcomeUnknown
	}
	if sprite == nil || spritesutil.NormalizeSpriteStatus(sprite.Status) != "running" {
		return nil, nil, RecoveryOutcomeUnknown
	}
	instanceInfo, err := r.getExistingInstanceInfo(ctx, sprite, instanceID)
	if err != nil || instanceInfo == nil || instanceInfo.Port < 1 || instanceInfo.Port > 65535 {
		return nil, nil, RecoveryOutcomeUnknown
	}
	if instanceInfo.SessionID != record.SessionID || instanceInfo.TaskID != record.TaskID {
		return nil, nil, RecoveryOutcomeUnknown
	}
	return sprite, instanceInfo, ""
}

func (r *SpritesExecutor) hasSpritesProxy(instanceID string) bool {
	r.mu.RLock()
	proxy := r.proxies[instanceID]
	r.mu.RUnlock()
	return proxy != nil
}

func (r *SpritesExecutor) attachRecoveredSprite(
	ctx context.Context,
	record *models.ExecutorRunning,
	spriteName string,
	sprite *spritesapi.Sprite,
	instanceInfo *agentctl.InstanceInfo,
) (*ExecutorInstance, RecoveryCandidateOutcome) {
	proxy, err := r.createPortForwardingSession(sprite, spriteName, instanceInfo.Port)
	if err != nil {
		return nil, RecoveryOutcomeUnknown
	}
	r.mu.Lock()
	if r.proxies[record.AgentExecutionID] != nil {
		r.mu.Unlock()
		r.closeProxySession(proxy)
		return nil, RecoveryOutcomeUnknown
	}
	r.proxies[record.AgentExecutionID] = proxy
	r.mu.Unlock()
	instance := r.buildRecoveredSpriteInstance(record, spriteName, proxy.localPort, sprite.Status, sprite.CreatedAt, instanceInfo)
	instance.DiscardRecovery = func() {
		r.mu.Lock()
		if r.proxies[record.AgentExecutionID] == proxy {
			delete(r.proxies, record.AgentExecutionID)
		}
		r.mu.Unlock()
		r.closeProxySession(proxy)
	}
	return instance, ""
}

func (r *SpritesExecutor) buildRecoveredSpriteInstance(
	record *models.ExecutorRunning,
	spriteName string,
	localPort int,
	state string,
	createdAt time.Time,
	instanceInfo *agentctl.InstanceInfo,
) *ExecutorInstance {
	metadata := cloneSpriteRecoveryMetadata(record.Metadata)
	metadata[MetadataKeySpriteName] = spriteName
	metadata[MetadataKeySpriteState] = strings.TrimSpace(state)
	metadata[MetadataKeySpriteCreatedAt] = createdAt
	metadata[MetadataKeyLocalPort] = localPort
	metadata[MetadataKeyReuseExistingProcess] = true
	metadata[MetadataKeyIsRemote] = true
	return &ExecutorInstance{
		InstanceID:           record.AgentExecutionID,
		TaskID:               record.TaskID,
		SessionID:            record.SessionID,
		RuntimeName:          r.Name(),
		WorkspacePath:        spritesWorkspacePath,
		Metadata:             metadata,
		Env:                  cloneStringMap(instanceInfo.Env),
		WorkspaceSourceRoots: append([]string(nil), instanceInfo.WorkspaceSourceRoots...),
		Client: agentctl.NewClient("127.0.0.1", localPort, r.logger,
			agentctl.WithExecutionID(record.AgentExecutionID),
			agentctl.WithSessionID(record.SessionID)),
	}
}

func cloneSpriteRecoveryMetadata(metadata map[string]interface{}) map[string]interface{} {
	clone := make(map[string]interface{}, len(metadata)+6)
	for key, value := range metadata {
		clone[key] = value
	}
	return clone
}

func (r *SpritesExecutor) Close() error {
	r.mu.Lock()
	proxies := make([]*SpritesProxySession, 0, len(r.proxies))
	for _, proxy := range r.proxies {
		proxies = append(proxies, proxy)
	}
	r.proxies = make(map[string]*SpritesProxySession)
	r.tokens = make(map[string]string)
	r.mu.Unlock()
	for _, proxy := range proxies {
		r.closeProxySession(proxy)
	}
	return nil
}
