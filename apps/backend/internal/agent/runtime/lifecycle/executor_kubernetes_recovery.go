package lifecycle

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"

	kubeexecutor "github.com/kandev/kandev/internal/agent/kubernetes"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *KubernetesExecutor) RecoverInstancesDetailed(
	ctx context.Context,
	records []*models.ExecutorRunning,
) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var instances []*ExecutorInstance
	outcomes := make(map[string]RecoveryCandidateOutcome)
	for _, record := range records {
		if record == nil || record.Runtime != agentruntime.RuntimeKubernetes || record.SessionID == "" {
			continue
		}
		instance, outcome := r.recoverKubernetesRecord(ctx, record)
		if instance == nil {
			outcomes[record.SessionID] = outcome
			continue
		}
		delete(outcomes, record.SessionID)
		instances = append(instances, instance)
	}
	return instances, outcomes, nil
}

func (r *KubernetesExecutor) recoverKubernetesRecord(
	parent context.Context,
	record *models.ExecutorRunning,
) (*ExecutorInstance, RecoveryCandidateOutcome) {
	req, ok := kubernetesRecoveryRequest(record)
	if !ok || r.clientFactory == nil {
		return nil, RecoveryOutcomeUnknown
	}
	attemptCtx, cancel := context.WithTimeout(parent, remoteRecoveryAttemptTimeout)
	defer cancel()

	target, outcome := r.prepareKubernetesRecoveryTarget(attemptCtx, req)
	if outcome != "" {
		return nil, outcome
	}
	controlForward, control, outcome := r.openKubernetesRecoveryControl(attemptCtx, target)
	if outcome != "" {
		return nil, outcome
	}
	defer func() {
		control.Close()
		_ = controlForward.Close()
	}()
	instanceInfo, token, outcome := recoverKubernetesControlInstance(
		attemptCtx, control, req, record, target.recorded,
	)
	if outcome != "" {
		return nil, outcome
	}
	if !validKubernetesRecoveredInstance(instanceInfo, record, target.recorded) {
		return nil, RecoveryOutcomeUnknown
	}

	instanceForward, client, token, outcome := r.verifyKubernetesRecoveredProcess(
		attemptCtx, target, control, req, record, token,
	)
	if outcome != "" {
		return nil, outcome
	}
	keepInstanceForward := false
	defer func() {
		if !keepInstanceForward {
			client.Close()
			_ = instanceForward.Close()
		}
	}()
	instance := buildRecoveredKubernetesInstance(
		record, req, target.recorded, instanceInfo, target.pod, client, token,
	)
	req.AuthToken = token
	session := &kubernetesSession{
		runtime: target.runtime, forward: instanceForward, client: client,
		request:      cloneKubernetesCreateRequest(req),
		restartCount: kubernetesMainContainerRestartCount(target.pod, target.recorded.mainContainer),
	}
	if !r.trackRecoveredKubernetesSession(record.AgentExecutionID, session) {
		return nil, RecoveryOutcomeUnknown
	}
	instance.DiscardRecovery = discardRecoveredKubernetesSession(r, record.AgentExecutionID, session)
	keepInstanceForward = true
	return instance, ""
}

type kubernetesRecoveryTarget struct {
	runtime  *kubernetesRuntimeClient
	recorded kubernetesRecordedState
	pod      *corev1.Pod
}

func (r *KubernetesExecutor) prepareKubernetesRecoveryTarget(
	ctx context.Context,
	req *ExecutorCreateRequest,
) (*kubernetesRecoveryTarget, RecoveryCandidateOutcome) {
	executorConfig, err := kubernetesExecutorConfigFromMetadata(req.Metadata)
	if err != nil {
		return nil, RecoveryOutcomeUnknown
	}
	runtime, err := r.clientFactory(executorConfig)
	if err != nil || runtime == nil || runtime.resources == nil || runtime.streams == nil {
		return nil, RecoveryOutcomeUnknown
	}
	recorded, identity, err := kubernetesRecordedInventory(req, true)
	if err != nil {
		return nil, RecoveryOutcomeUnknown
	}
	pod, err := runtime.resources.GetPod(ctx, recorded.namespace, recorded.podName)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, RecoveryOutcomeNoMatchingInstance
		}
		return nil, RecoveryOutcomeUnknown
	}
	if err := verifyRecordedPod(pod, recorded.namespace, recorded.podName, recorded.podUID, identity); err != nil {
		return nil, RecoveryOutcomeUnknown
	}
	if err := verifyKubernetesRecordedPVC(ctx, runtime.resources, recorded, identity); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, RecoveryOutcomeNoMatchingInstance
		}
		return nil, RecoveryOutcomeUnknown
	}
	if ctx.Err() != nil {
		return nil, RecoveryOutcomeUnknown
	}
	return &kubernetesRecoveryTarget{runtime: runtime, recorded: recorded, pod: pod}, ""
}

func (r *KubernetesExecutor) openKubernetesRecoveryControl(
	ctx context.Context,
	target *kubernetesRecoveryTarget,
) (kubeexecutor.PortForwardSession, *agentctl.ControlClient, RecoveryCandidateOutcome) {
	forward, control, err := r.connectHealthyKubernetesControl(ctx, target.runtime, target.pod)
	if err != nil {
		return nil, nil, RecoveryOutcomeUnknown
	}
	return forward, control, ""
}

func (r *KubernetesExecutor) verifyKubernetesRecoveredProcess(
	ctx context.Context,
	target *kubernetesRecoveryTarget,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	token string,
) (kubeexecutor.PortForwardSession, *agentctl.Client, string, RecoveryCandidateOutcome) {
	forward, err := startKubernetesForward(
		ctx, target.runtime.streams, target.pod, uint16(target.recorded.remotePort),
	)
	if err != nil {
		return nil, nil, "", RecoveryOutcomeUnknown
	}
	client := newKubernetesAgentctlClient(
		r.logger, req, target.recorded.agentctlInstanceID, token, forward.LocalPort(),
	)
	keep := false
	defer func() {
		if !keep {
			client.Close()
			_ = forward.Close()
		}
	}()
	var outcome RecoveryCandidateOutcome
	client, token, outcome = r.authenticateKubernetesRecoveredClient(
		ctx, target, control, req, record, forward.LocalPort(), client, token,
	)
	if outcome != "" {
		return nil, nil, "", outcome
	}
	var status *agentctl.StatusResponse
	client, token, status, outcome = r.readKubernetesRecoveredStatus(
		ctx, target, control, req, record, forward.LocalPort(), client, token,
	)
	if outcome != "" {
		return nil, nil, "", outcome
	}
	if status == nil {
		return nil, nil, "", RecoveryOutcomeUnknown
	}
	if !status.IsAgentRunning() {
		return nil, nil, "", RecoveryOutcomeNoMatchingInstance
	}
	if ctx.Err() != nil {
		return nil, nil, "", RecoveryOutcomeUnknown
	}
	keep = true
	return forward, client, token, ""
}

func (r *KubernetesExecutor) authenticateKubernetesRecoveredClient(
	ctx context.Context,
	target *kubernetesRecoveryTarget,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	localPort uint16,
	client *agentctl.Client,
	token string,
) (*agentctl.Client, string, RecoveryCandidateOutcome) {
	if err := client.Health(ctx); err == nil {
		return client, token, ""
	} else if !isAgentctlAuthError(err) {
		return client, token, RecoveryOutcomeUnknown
	}
	client, token, refreshed := r.refreshKubernetesRecoveredClient(
		ctx, target, control, req, record, localPort, client, token,
	)
	if !refreshed {
		return client, token, RecoveryOutcomeUnknown
	}
	return client, token, ""
}

func (r *KubernetesExecutor) readKubernetesRecoveredStatus(
	ctx context.Context,
	target *kubernetesRecoveryTarget,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	localPort uint16,
	client *agentctl.Client,
	token string,
) (*agentctl.Client, string, *agentctl.StatusResponse, RecoveryCandidateOutcome) {
	status, err := client.GetStatus(ctx)
	if err != nil && isAgentctlAuthError(err) {
		var refreshed bool
		client, token, refreshed = r.refreshKubernetesRecoveredClient(
			ctx, target, control, req, record, localPort, client, token,
		)
		if refreshed {
			status, err = client.GetStatus(ctx)
		}
	}
	if err != nil || status == nil {
		return client, token, nil, RecoveryOutcomeUnknown
	}
	return client, token, status, ""
}

func (r *KubernetesExecutor) refreshKubernetesRecoveredClient(
	ctx context.Context,
	target *kubernetesRecoveryTarget,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	localPort uint16,
	client *agentctl.Client,
	token string,
) (*agentctl.Client, string, bool) {
	if req.BootstrapNonce == "" || token != req.AuthToken {
		return client, token, false
	}
	if _, err := refreshKubernetesRecoveryToken(ctx, control, req, record, target.recorded); err != nil {
		return client, token, false
	}
	token = control.AuthToken()
	client.Close()
	client = newKubernetesAgentctlClient(
		r.logger, req, target.recorded.agentctlInstanceID, token, localPort,
	)
	return client, token, true
}

func kubernetesRecoveryRequest(record *models.ExecutorRunning) (*ExecutorCreateRequest, bool) {
	if record == nil || strings.TrimSpace(record.AgentExecutionID) == "" ||
		strings.TrimSpace(record.SessionID) == "" || strings.TrimSpace(record.TaskID) == "" {
		return nil, false
	}
	metadata := cloneKubernetesMetadata(record.Metadata)
	return &ExecutorCreateRequest{
		InstanceID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID,
		TaskEnvironmentID: getMetadataString(metadata, MetadataKeyKubernetesResourceEnvironmentID),
		AuthToken:         record.TransientAuthToken, BootstrapNonce: record.TransientBootstrapNonce,
		Metadata: metadata, Env: make(map[string]string),
	}, true
}

func recoverKubernetesControlInstance(
	ctx context.Context,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	recorded kubernetesRecordedState,
) (*agentctl.InstanceInfo, string, RecoveryCandidateOutcome) {
	token := req.AuthToken
	if token == "" {
		if req.BootstrapNonce == "" {
			return nil, "", RecoveryOutcomeUnknown
		}
		var err error
		token, err = control.Handshake(ctx, req.BootstrapNonce)
		if err != nil {
			return nil, "", RecoveryOutcomeUnknown
		}
		control.SetAuthToken(token)
	}
	control.SetAuthToken(token)
	info, err := control.GetInstance(ctx, recorded.agentctlInstanceID)
	if err != nil && isAgentctlAuthError(err) && req.BootstrapNonce != "" && token == req.AuthToken {
		if info, err = refreshKubernetesRecoveryToken(ctx, control, req, record, recorded); err == nil {
			token = control.AuthToken()
		}
	}
	if errors.Is(err, agentctl.ErrInstanceNotFound) {
		return nil, "", RecoveryOutcomeNoMatchingInstance
	}
	if err != nil {
		return nil, "", RecoveryOutcomeUnknown
	}
	if !validKubernetesRecoveredInstance(info, record, recorded) {
		return nil, "", RecoveryOutcomeUnknown
	}
	return info, token, ""
}

func refreshKubernetesRecoveryToken(
	ctx context.Context,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	recorded kubernetesRecordedState,
) (*agentctl.InstanceInfo, error) {
	token, err := control.Handshake(ctx, req.BootstrapNonce)
	if err != nil {
		return nil, err
	}
	control.SetAuthToken(token)
	info, err := control.GetInstance(ctx, recorded.agentctlInstanceID)
	if err != nil {
		return nil, err
	}
	if !validKubernetesRecoveredInstance(info, record, recorded) {
		return nil, errors.New("kubernetes lifecycle: recorded agentctl identity changed during auth refresh")
	}
	return info, nil
}

func validKubernetesRecoveredInstance(
	info *agentctl.InstanceInfo,
	record *models.ExecutorRunning,
	recorded kubernetesRecordedState,
) bool {
	return info != nil && info.ID == recorded.agentctlInstanceID &&
		info.SessionID == record.SessionID && info.TaskID == record.TaskID &&
		info.WorkspacePath == kubernetesWorkspacePath && info.Port == recorded.remotePort
}

func buildRecoveredKubernetesInstance(
	record *models.ExecutorRunning,
	req *ExecutorCreateRequest,
	recorded kubernetesRecordedState,
	info *agentctl.InstanceInfo,
	pod *corev1.Pod,
	client *agentctl.Client,
	token string,
) *ExecutorInstance {
	metadata := cloneKubernetesMetadata(record.Metadata)
	metadata[MetadataKeyKubernetesAgentctlRemotePort] = strconv.Itoa(recorded.remotePort)
	metadata[MetadataKeyKubernetesAgentctlInstanceID] = recorded.agentctlInstanceID
	metadata[MetadataKeyKubernetesContainerRestartCount] = strconv.FormatInt(
		int64(kubernetesMainContainerRestartCount(pod, recorded.mainContainer)), 10,
	)
	metadata[MetadataKeyKubernetesResourceInstanceID] = getMetadataString(req.Metadata, MetadataKeyKubernetesResourceInstanceID)
	metadata[MetadataKeyIsRemote] = true
	return &ExecutorInstance{
		InstanceID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID,
		RuntimeName: agentruntime.RuntimeKubernetes, Client: client, WorkspacePath: info.WorkspacePath,
		Metadata: metadata, Env: cloneStringMap(info.Env),
		WorkspaceSourceRoots: append([]string(nil), info.WorkspaceSourceRoots...),
		ProviderSessionID:    info.ProviderSessionID, AuthToken: token, BootstrapNonce: req.BootstrapNonce,
	}
}

func (r *KubernetesExecutor) trackRecoveredKubernetesSession(instanceID string, session *kubernetesSession) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[instanceID] != nil {
		return false
	}
	r.sessions[instanceID] = session
	return true
}

func discardRecoveredKubernetesSession(
	r *KubernetesExecutor,
	instanceID string,
	session *kubernetesSession,
) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			if r.sessions[instanceID] == session {
				delete(r.sessions, instanceID)
			}
			r.mu.Unlock()
			_ = closeKubernetesSessionResources(session)
		})
	}
}

var _ DetailedRecoveryBackend = (*KubernetesExecutor)(nil)
