package lifecycle

import (
	"context"
	"errors"
	"fmt"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/secrets"
	corev1 "k8s.io/api/core/v1"
)

func (r *KubernetesExecutor) connectSharedKubernetesAgentctl(ctx context.Context, runtime *kubernetesRuntimeClient, req *ExecutorCreateRequest, pod *corev1.Pod, remoteID string) (_ *agentctl.Client, _ kubeexecutor.PortForwardSession, _ string, _ int, returnedErr error) {
	if err := r.prepareSharedKubernetesCredentials(ctx, runtime, req, pod, getMetadataString(req.Metadata, MetadataKeyKubernetesMainContainer)); err != nil {
		return nil, nil, "", 0, err
	}
	forward, control, err := r.connectHealthyKubernetesControl(ctx, runtime, pod)
	if err != nil {
		return nil, nil, "", 0, err
	}
	defer func() { _ = forward.Close() }()
	unlock := r.lockInstance("control:" + req.TaskEnvironmentID)
	defer unlock()
	token, err := r.sharedKubernetesControlToken(ctx, req)
	if err != nil {
		return nil, nil, "", 0, err
	}
	secretID := getMetadataString(req.Metadata, MetadataKeyAuthTokenSecret)
	control.SetAuthToken(token)
	response, created, err := getOrCreateSharedKubernetesInstance(ctx, control, req, remoteID)
	defer func() {
		if returnedErr != nil && created {
			rollbackCtx, cancel := kubernetesDurableContext(ctx)
			defer cancel()
			returnedErr = errors.Join(returnedErr, control.DeleteInstance(rollbackCtx, remoteID))
		}
	}()
	if isAgentctlAuthError(err) && req.BootstrapNonce != "" {
		token, err = control.Handshake(ctx, req.BootstrapNonce)
		if err == nil && r.secretStore != nil && secretID != "" {
			err = r.persistSharedKubernetesControlToken(ctx, secretID, token)
		}
		if err == nil {
			response, created, err = getOrCreateSharedKubernetesInstance(ctx, control, req, remoteID)
		}
	}
	if err != nil {
		return nil, nil, "", 0, err
	}
	instanceForward, err := startKubernetesForward(ctx, runtime.streams, pod, uint16(response.Port))
	if err != nil {
		return nil, nil, "", 0, err
	}
	client := newKubernetesAgentctlClient(r.logger, req, remoteID, token, instanceForward.LocalPort())
	if err := client.Health(ctx); err != nil {
		client.Close()
		_ = instanceForward.Close()
		return nil, nil, "", 0, err
	}
	return client, instanceForward, token, response.Port, nil
}

func getOrCreateSharedKubernetesInstance(ctx context.Context, control kubernetesAgentctlInstanceControl, req *ExecutorCreateRequest, remoteID string) (*agentctl.CreateInstanceResponse, bool, error) {
	request := buildReconnectCreateInstanceRequest(req, remoteID)
	applyKubernetesDurableJournalPath(request, req)
	info, err := control.GetInstance(ctx, remoteID)
	if err == nil {
		response := &agentctl.CreateInstanceResponse{ID: info.ID, Port: info.Port}
		if err := validateKubernetesAgentctlInstanceResponse(response, request); err != nil {
			return nil, false, err
		}
		if info.WorkspacePath != request.WorkspacePath {
			return nil, false, fmt.Errorf("kubernetes instance workspace mismatch")
		}
		return response, false, nil
	}
	if !errors.Is(err, agentctl.ErrInstanceNotFound) {
		return nil, false, err
	}
	response, createErr := createOrReconcileKubernetesAgentctlInstance(ctx, control, request)
	return response, true, createErr
}

func (r *KubernetesExecutor) sharedKubernetesControlToken(ctx context.Context, req *ExecutorCreateRequest) (string, error) {
	id := getMetadataString(req.Metadata, MetadataKeyAuthTokenSecret)
	if r.secretStore == nil || id == "" {
		return req.AuthToken, nil
	}
	r.mu.Lock()
	pending := r.pendingControlTokens[id]
	r.mu.Unlock()
	if pending == "" {
		var err error
		pending, err = r.secretStore.Reveal(ctx, kubernetesControlRecoverySecretID(id))
		if err != nil && !errors.Is(err, secrets.ErrNotFound) {
			return "", err
		}
	}
	if pending != "" {
		if err := r.persistSharedKubernetesControlToken(ctx, id, pending); err != nil {
			return "", err
		}
	}
	return r.secretStore.Reveal(ctx, id)
}
