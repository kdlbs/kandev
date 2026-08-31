package lifecycle

import (
	"context"
	"errors"
	"fmt"

	agentctltypes "github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/secrets"
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
	refresh, err := refresher.RefreshRemoteInstance(ctx, m.remoteStatusInstance(ctx, execution))
	if err != nil || refresh == nil {
		return err
	}
	return m.applyTrackedRemoteRefresh(ctx, execution, refresh)
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
	execution.mergeMetadata(instance.Metadata)
	if err := m.persistExecutorRunningResult(ctx, execution); err != nil {
		execution.replaceMetadataSnapshot(oldMetadata)
		restoreErr := m.secretStore.Update(ctx, authID, &secrets.UpdateSecretRequest{Value: &oldToken})
		return nil, errors.Join(fmt.Errorf("persist Kubernetes refreshed inventory: %w", err), restoreErr)
	}
	rollback := func(rollbackCtx context.Context) error {
		execution.replaceMetadataSnapshot(oldMetadata)
		secretErr := m.secretStore.Update(
			rollbackCtx, authID, &secrets.UpdateSecretRequest{Value: &oldToken},
		)
		rowErr := m.persistExecutorRunningResult(rollbackCtx, execution)
		return errors.Join(secretErr, rowErr)
	}
	return rollback, nil
}
