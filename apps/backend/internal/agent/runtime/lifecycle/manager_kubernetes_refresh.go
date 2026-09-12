package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"strings"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/secrets"
	"github.com/kandev/kandev/internal/task/models"
)

func (m *Manager) refreshTrackedRemoteInstance(
	ctx context.Context,
	execution *AgentExecution,
	refresher RemoteInstanceRefresher,
) error {
	_, err, _ := m.remoteRefreshGroup.Do(execution.ID, func() (interface{}, error) {
		return nil, m.refreshTrackedRemoteInstanceOnce(ctx, execution, refresher)
	})
	return err
}

func (m *Manager) refreshTrackedRemoteInstanceOnce(
	ctx context.Context,
	execution *AgentExecution,
	refresher RemoteInstanceRefresher,
) error {
	execution.remoteInstanceLifecycleMu.Lock()
	defer execution.remoteInstanceLifecycleMu.Unlock()
	current, exists := m.executionStore.Get(execution.ID)
	if !exists || current != execution {
		return nil
	}
	instance, err := m.kubernetesRefreshStatusInstance(ctx, execution)
	if err != nil {
		return err
	}
	refresh, err := refresher.RefreshRemoteInstance(ctx, instance)
	if err != nil || refresh == nil {
		return err
	}
	return m.applyTrackedRemoteRefresh(ctx, execution, refresh)
}

type kubernetesExecutorReader interface {
	GetExecutor(ctx context.Context, id string) (*models.Executor, error)
}

func (m *Manager) kubernetesRefreshStatusInstance(
	ctx context.Context,
	execution *AgentExecution,
) (*ExecutorInstance, error) {
	instance := m.remoteStatusInstance(ctx, execution)
	if execution == nil || execution.RuntimeName != agentruntime.RuntimeKubernetes {
		return instance, nil
	}
	metadata, err := m.currentKubernetesConnectionMetadata(ctx, instance.Metadata)
	if err != nil {
		return nil, err
	}
	instance.Metadata = metadata
	return instance, nil
}

func (m *Manager) currentKubernetesConnectionMetadata(
	ctx context.Context,
	metadata map[string]interface{},
) (map[string]interface{}, error) {
	reader, ok := m.runningWriter.(kubernetesExecutorReader)
	executorID := strings.TrimSpace(getMetadataString(metadata, "executor_id"))
	if !ok || executorID == "" {
		return metadata, nil
	}
	current, err := reader.GetExecutor(ctx, executorID)
	if err != nil {
		return nil, fmt.Errorf("load current Kubernetes executor %q: %w", executorID, err)
	}
	if current == nil || current.Type != models.ExecutorTypeKubernetes {
		return nil, fmt.Errorf("current executor %q is not a Kubernetes executor", executorID)
	}
	config, err := kubeexecutor.ParseExecutorConfig(current.Config)
	if err != nil {
		return nil, fmt.Errorf("parse current Kubernetes executor %q: %w", executorID, err)
	}
	return overlayCurrentKubernetesConnectionMetadata(metadata, current.Config, config), nil
}

func overlayCurrentKubernetesConnectionMetadata(
	metadata map[string]interface{},
	configValues map[string]string,
	config kubeexecutor.ExecutorConfig,
) map[string]interface{} {
	resolved := cloneKubernetesMetadata(metadata)
	for _, key := range kubernetesConnectionMetadataKeys {
		resolved[key] = configValues[key]
	}
	resolved[MetadataKeyKubernetesExecutorConfigHash] = kubernetesConfigHash(config)
	return resolved
}

func (m *Manager) applyTrackedRemoteRefresh(
	ctx context.Context,
	execution *AgentExecution,
	refresh *RemoteInstanceRefresh,
) error {
	committed := false
	defer func() {
		if !committed && refresh.Abort != nil {
			refresh.Abort()
		}
	}()
	if refresh.Instance == nil || refresh.Instance.Client == nil {
		return errors.New("remote refresh returned an incomplete agentctl instance")
	}
	newACPSessionID, err := m.prepareKubernetesRefreshProcess(ctx, execution, refresh)
	if err != nil {
		return err
	}
	rollbackPersistence, err := m.persistActiveKubernetesRefresh(ctx, execution, refresh.Instance)
	if err != nil {
		return err
	}
	committed, err = m.commitTrackedRemoteRefresh(
		ctx, execution, refresh, newACPSessionID, rollbackPersistence,
	)
	if err != nil {
		return err
	}
	if m.streamManager != nil {
		go m.streamManager.ReconnectAll(execution)
	}
	return nil
}

func (m *Manager) prepareKubernetesRefreshProcess(
	ctx context.Context,
	execution *AgentExecution,
	refresh *RemoteInstanceRefresh,
) (string, error) {
	if !refresh.ProcessRestarted {
		return "", nil
	}
	return m.prepareRestartedKubernetesAgentctl(ctx, execution, refresh)
}

func (m *Manager) commitTrackedRemoteRefresh(
	ctx context.Context,
	execution *AgentExecution,
	refresh *RemoteInstanceRefresh,
	newACPSessionID string,
	rollbackPersistence func(context.Context) error,
) (bool, error) {
	if refresh.Commit == nil {
		rollbackErr := rollbackPersistence(ctx)
		return false, errors.Join(errors.New("remote refresh has no commit operation"), rollbackErr)
	}
	published := false
	execution.agentctlLifecycleMu.Lock()
	commitErr := refresh.Commit(func() {
		refresh.Instance.Client.SetTraceContext(execution.SessionTraceContext())
		execution.replaceAgentctlClient(refresh.Instance.Client)
		if newACPSessionID != "" {
			execution.ACPSessionID = newACPSessionID
			execution.setSessionInitialized(true)
		}
		published = true
	})
	execution.agentctlLifecycleMu.Unlock()
	if commitErr != nil && published {
		return true, fmt.Errorf("remote refresh commit failed after publishing replacement: %w", commitErr)
	}
	if commitErr != nil {
		return false, errors.Join(commitErr, rollbackPersistence(ctx))
	}
	if !published {
		return true, errors.New("remote refresh commit did not publish its replacement")
	}
	return true, nil
}

func (m *Manager) prepareRestartedKubernetesAgentctl(
	ctx context.Context,
	execution *AgentExecution,
	refresh *RemoteInstanceRefresh,
) (string, error) {
	client := refresh.Instance.Client
	env := execution.RuntimeEnvironment()
	if env == nil {
		env = runtimeEnvFromMetadata(execution.MetadataSnapshot())
	}
	approvalPolicy := "untrusted"
	if refresh.AutoApprovePermissions {
		approvalPolicy = "never"
	}
	if execution.AgentCommand == "" {
		return "", fmt.Errorf("execution %q has no recorded agent command for Kubernetes restart", execution.ID)
	}
	if err := client.ConfigureAgent(
		ctx, execution.AgentCommand, execution.AgentArgs, env, approvalPolicy,
		execution.ContinueCommand, execution.ContinueArgs,
	); err != nil {
		return "", fmt.Errorf("configure agent after Kubernetes restart: %w", err)
	}
	if _, err := client.Start(ctx); err != nil {
		return "", fmt.Errorf("start agent after Kubernetes restart: %w", err)
	}
	if refresh.AgentConfig == nil || m.sessionManager == nil {
		return "", nil
	}
	result, err := m.sessionManager.InitializeSession(
		ctx, client, refresh.AgentConfig, execution.ACPSessionID,
		execution.WorkspacePath, kubernetesRefreshMcpServers(refresh.McpServers),
	)
	if err != nil {
		return "", fmt.Errorf("resume ACP session after Kubernetes restart: %w", err)
	}
	return result.SessionID, nil
}

func kubernetesRefreshMcpServers(configs []McpServerConfig) []agentctltypes.McpServer {
	servers := make([]agentctltypes.McpServer, 0, len(configs))
	for _, config := range configs {
		servers = append(servers, agentctltypes.McpServer{
			Name: config.Name, Command: config.Command, Args: append([]string(nil), config.Args...),
			URL: config.URL, Type: config.Type, Env: cloneStringMap(config.Env),
			Headers: cloneStringMap(config.Headers),
		})
	}
	return servers
}

func (m *Manager) persistActiveKubernetesRefresh(
	ctx context.Context,
	execution *AgentExecution,
	instance *ExecutorInstance,
) (func(context.Context) error, error) {
	if m.secretStore == nil || m.runningWriter == nil {
		return nil, errors.New("persist Kubernetes remote refresh: durable stores are unavailable")
	}
	persistCtx, cancelPersist := kubernetesDurableContext(ctx)
	defer cancelPersist()
	ctx = persistCtx
	oldMetadata := execution.MetadataSnapshot()
	authID := getMetadataString(oldMetadata, MetadataKeyAuthTokenSecret)
	nonceID := getMetadataString(oldMetadata, MetadataKeyBootstrapNonceSecret)
	if authID == "" || nonceID == "" || instance.AuthToken == "" || instance.BootstrapNonce == "" {
		return nil, errors.New("persist Kubernetes remote refresh: required secret references are incomplete")
	}
	oldToken, err := revealGlobalSecret(ctx, m.secretStore, authID)
	if err != nil {
		return nil, fmt.Errorf("reveal prior Kubernetes auth token: %w", err)
	}
	nonce, err := revealGlobalSecret(ctx, m.secretStore, nonceID)
	if err != nil {
		return nil, fmt.Errorf("reveal Kubernetes bootstrap nonce: %w", err)
	}
	if nonce != instance.BootstrapNonce {
		return nil, errors.New("persist Kubernetes remote refresh: bootstrap nonce changed")
	}
	if err := m.secretStore.Update(ctx, authID, &secrets.UpdateSecretRequest{Value: &instance.AuthToken}); err != nil {
		return nil, fmt.Errorf("rotate Kubernetes auth token: %w", err)
	}
	rollbackMetadata := execution.mergeMetadataWithRollback(instance.Metadata)
	if err := m.persistExecutorRunningResult(ctx, execution); err != nil {
		rollbackMetadata()
		restoreErr := m.secretStore.Update(ctx, authID, &secrets.UpdateSecretRequest{Value: &oldToken})
		return nil, errors.Join(fmt.Errorf("persist Kubernetes refreshed inventory: %w", err), restoreErr)
	}
	rollback := func(rollbackCtx context.Context) error {
		rollbackMetadata()
		durableRollbackCtx, cancelRollback := kubernetesDurableContext(rollbackCtx)
		defer cancelRollback()
		secretErr := m.secretStore.Update(
			durableRollbackCtx, authID, &secrets.UpdateSecretRequest{Value: &oldToken},
		)
		rowErr := m.persistExecutorRunningResult(durableRollbackCtx, execution)
		return errors.Join(secretErr, rowErr)
	}
	return rollback, nil
}
