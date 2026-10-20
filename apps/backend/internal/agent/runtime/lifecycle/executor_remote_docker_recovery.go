package lifecycle

import (
	"context"
	"errors"
	"strings"
	"sync"

	"github.com/containerd/errdefs"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/agent/docker"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *RemoteDockerExecutor) RecoverInstancesDetailed(
	ctx context.Context,
	records []*models.ExecutorRunning,
) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var instances []*ExecutorInstance
	outcomes := make(map[string]RecoveryCandidateOutcome)
	for _, record := range records {
		if record == nil || record.Runtime != agentruntime.RuntimeRemoteDocker || record.SessionID == "" {
			continue
		}
		instance, outcome := r.recoverRemoteDockerRecord(ctx, record)
		if instance == nil {
			outcomes[record.SessionID] = outcome
			continue
		}
		delete(outcomes, record.SessionID)
		instances = append(instances, instance)
	}
	return instances, outcomes, nil
}

func (r *RemoteDockerExecutor) recoverRemoteDockerRecord(
	parent context.Context,
	record *models.ExecutorRunning,
) (*ExecutorInstance, RecoveryCandidateOutcome) {
	req, containerID, ok := remoteDockerRecoveryRequest(record)
	if !ok || r.connect == nil {
		return nil, RecoveryOutcomeUnknown
	}
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, remoteRecoveryAttemptTimeout)
	defer cancel()
	session, err := r.connect(ctx, req)
	if err != nil || session == nil || session.dockerClient == nil || session.endpoints == nil {
		if session != nil {
			_ = session.close()
		}
		return nil, RecoveryOutcomeUnknown
	}
	keepSession := false
	var client *agentctl.Client
	defer func() {
		if keepSession {
			return
		}
		if client != nil {
			client.Close()
		}
		_ = session.close()
	}()

	instance, client, outcome := r.attachRemoteDockerRecord(ctx, req, record, containerID, session)
	if outcome != "" {
		return nil, outcome
	}
	session.setAgentctlClient(client)
	if !r.trackRecoveredRemoteDocker(req, session, instance) {
		return nil, RecoveryOutcomeUnknown
	}
	instance.DiscardRecovery = discardRecoveredRemoteDocker(r, instance.InstanceID, session, client)
	if r.watchTransport != nil {
		r.watchTransport(instance.InstanceID, session)
	}
	keepSession = true
	return instance, ""
}

func (r *RemoteDockerExecutor) attachRemoteDockerRecord(
	ctx context.Context,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	containerID string,
	session *remoteDockerSession,
) (*ExecutorInstance, *agentctl.Client, RecoveryCandidateOutcome) {
	if outcome := verifyLiveRemoteDockerContainer(ctx, session.dockerClient, containerID); outcome != "" {
		return nil, nil, outcome
	}
	control, outcome := r.remoteDockerControl(ctx, req, record.SessionID, containerID, session.endpoints)
	if outcome != "" {
		return nil, nil, outcome
	}
	defer control.Close()
	info, token, outcome := recoverRemoteDockerControlInstance(ctx, control, req, record)
	if outcome != "" || !validRemoteDockerInstance(info, req, record) {
		if outcome == "" {
			outcome = RecoveryOutcomeUnknown
		}
		return nil, nil, outcome
	}
	client, info, token, outcome := r.authenticateRemoteDockerInstance(ctx, session.endpoints, control, req, record, info, token)
	if outcome != "" {
		return nil, nil, outcome
	}
	if err := ctx.Err(); err != nil {
		client.Close()
		return nil, nil, RecoveryOutcomeUnknown
	}
	return buildRemoteDockerRecoveredInstance(r, req, containerID, info, token, client), client, ""
}

func verifyLiveRemoteDockerContainer(
	ctx context.Context,
	client *docker.Client,
	containerID string,
) RecoveryCandidateOutcome {
	containerInfo, err := client.GetContainerInfo(ctx, containerID)
	if err != nil {
		if errdefs.IsNotFound(err) {
			return RecoveryOutcomeNoMatchingInstance
		}
		return RecoveryOutcomeUnknown
	}
	if containerInfo == nil || containerInfo.ID != containerID {
		return RecoveryOutcomeUnknown
	}
	if containerInfo.State != containerStateRunning {
		return RecoveryOutcomeNoMatchingInstance
	}
	if ctx.Err() != nil {
		return RecoveryOutcomeUnknown
	}
	return ""
}

func (r *RemoteDockerExecutor) remoteDockerControl(
	ctx context.Context,
	req *ExecutorCreateRequest,
	sessionID string,
	containerID string,
	endpoints containerEndpointResolver,
) (*agentctl.ControlClient, RecoveryCandidateOutcome) {
	controlHost, controlPort, err := endpoints.Resolve(ctx, containerID, AgentCtlPort, "")
	if err != nil {
		r.logger.Debug("remote Docker recovery cannot reach saved agentctl control endpoint",
			zap.String("session_id", sessionID), zap.Error(err))
		return nil, RecoveryOutcomeUnknown
	}
	control := agentctl.NewControlClient(controlHost, controlPort, r.logger,
		agentctl.WithControlAuthToken(req.AuthToken))
	if err := control.Health(ctx); err != nil {
		control.Close()
		r.logger.Debug("remote Docker recovery control server is unavailable",
			zap.String("session_id", sessionID), zap.Error(err))
		return nil, RecoveryOutcomeUnknown
	}
	return control, ""
}

func (r *RemoteDockerExecutor) authenticateRemoteDockerInstance(
	ctx context.Context,
	endpoints containerEndpointResolver,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	info *agentctl.InstanceInfo,
	token string,
) (*agentctl.Client, *agentctl.InstanceInfo, string, RecoveryCandidateOutcome) {
	host, port, err := endpoints.Resolve(ctx, getMetadataString(req.Metadata, MetadataKeyContainerID), info.Port, "")
	if err != nil {
		return nil, nil, "", RecoveryOutcomeUnknown
	}
	client := newRemoteDockerRecoveryClient(r, req, host, port, token)
	if err := client.Health(ctx); err != nil {
		if !isAgentctlAuthError(err) || req.BootstrapNonce == "" {
			client.Close()
			return nil, nil, "", RecoveryOutcomeUnknown
		}
		newToken, newInfo, outcome := refreshRemoteDockerRecoveryIdentity(ctx, control, req, record, info)
		if outcome != "" {
			client.Close()
			return nil, nil, "", outcome
		}
		client.Close()
		info, token = newInfo, newToken
		client = newRemoteDockerRecoveryClient(r, req, host, port, token)
	}
	status, err := client.GetStatus(ctx)
	if err != nil && isAgentctlAuthError(err) && req.BootstrapNonce != "" && token == req.AuthToken {
		newToken, newInfo, outcome := refreshRemoteDockerRecoveryIdentity(ctx, control, req, record, info)
		if outcome != "" {
			client.Close()
			return nil, nil, "", outcome
		}
		client.Close()
		info, token = newInfo, newToken
		client = newRemoteDockerRecoveryClient(r, req, host, port, token)
		status, err = client.GetStatus(ctx)
	}
	if err != nil || status == nil {
		client.Close()
		return nil, nil, "", RecoveryOutcomeUnknown
	}
	if !status.IsAgentRunning() {
		client.Close()
		return nil, nil, "", RecoveryOutcomeNoMatchingInstance
	}
	return client, info, token, ""
}

func refreshRemoteDockerRecoveryIdentity(
	ctx context.Context,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
	current *agentctl.InstanceInfo,
) (string, *agentctl.InstanceInfo, RecoveryCandidateOutcome) {
	token, refreshed, err := refreshRemoteDockerRecoveryToken(ctx, control, req, record)
	if err != nil || !validRemoteDockerInstance(refreshed, req, record) || refreshed.Port != current.Port {
		return "", nil, RecoveryOutcomeUnknown
	}
	return token, refreshed, ""
}

func validRemoteDockerInstance(
	info *agentctl.InstanceInfo,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
) bool {
	return info != nil && info.Port > 0 && info.Port <= 65535 && sameRemoteDockerInstance(info, record, req)
}

func newRemoteDockerRecoveryClient(
	r *RemoteDockerExecutor,
	req *ExecutorCreateRequest,
	host string,
	port int,
	token string,
) *agentctl.Client {
	return agentctl.NewClient(host, port, r.logger,
		agentctl.WithExecutionID(req.InstanceID), agentctl.WithSessionID(req.SessionID), agentctl.WithAuthToken(token))
}

func buildRemoteDockerRecoveredInstance(
	r *RemoteDockerExecutor,
	req *ExecutorCreateRequest,
	containerID string,
	info *agentctl.InstanceInfo,
	token string,
	client *agentctl.Client,
) *ExecutorInstance {
	workspacePath := info.WorkspacePath
	if workspacePath == "" {
		workspacePath = getMetadataString(req.Metadata, MetadataKeyOriginalWorkspacePath)
	}
	return &ExecutorInstance{
		InstanceID: req.InstanceID, TaskID: req.TaskID, SessionID: req.SessionID,
		RuntimeName: r.Name(), Client: client, ContainerID: containerID, WorkspacePath: workspacePath,
		Metadata: cloneKubernetesMetadata(req.Metadata), Env: cloneStringMap(info.Env),
		WorkspaceSourceRoots: append([]string(nil), info.WorkspaceSourceRoots...),
		ProviderSessionID:    info.ProviderSessionID, AuthToken: token, BootstrapNonce: req.BootstrapNonce,
	}
}

func remoteDockerRecoveryRequest(record *models.ExecutorRunning) (*ExecutorCreateRequest, string, bool) {
	if record == nil || strings.TrimSpace(record.AgentExecutionID) == "" ||
		strings.TrimSpace(record.SessionID) == "" || strings.TrimSpace(record.TaskID) == "" {
		return nil, "", false
	}
	containerID := strings.TrimSpace(record.ContainerID)
	metadataContainerID := strings.TrimSpace(getMetadataString(record.Metadata, MetadataKeyContainerID))
	if containerID == "" {
		containerID = metadataContainerID
	}
	if containerID == "" || (metadataContainerID != "" && metadataContainerID != containerID) {
		return nil, "", false
	}
	metadata := cloneKubernetesMetadata(record.Metadata)
	metadata[MetadataKeyContainerID] = containerID
	return &ExecutorCreateRequest{
		InstanceID: record.AgentExecutionID, TaskID: record.TaskID, SessionID: record.SessionID,
		AuthToken: record.TransientAuthToken, BootstrapNonce: record.TransientBootstrapNonce,
		Metadata: metadata, Env: make(map[string]string),
	}, containerID, true
}

func recoverRemoteDockerControlInstance(
	ctx context.Context,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
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
	}
	control.SetAuthToken(token)
	info, err := control.GetInstance(ctx, req.InstanceID)
	if err != nil && isAgentctlAuthError(err) && req.BootstrapNonce != "" && token == req.AuthToken {
		token, err = control.Handshake(ctx, req.BootstrapNonce)
		if err == nil {
			control.SetAuthToken(token)
			info, err = control.GetInstance(ctx, req.InstanceID)
		}
	}
	if errors.Is(err, agentctl.ErrInstanceNotFound) {
		return nil, "", RecoveryOutcomeNoMatchingInstance
	}
	if err != nil {
		return nil, "", RecoveryOutcomeUnknown
	}
	if !sameRemoteDockerInstance(info, record, req) {
		return nil, "", RecoveryOutcomeUnknown
	}
	return info, token, ""
}

func refreshRemoteDockerRecoveryToken(
	ctx context.Context,
	control *agentctl.ControlClient,
	req *ExecutorCreateRequest,
	record *models.ExecutorRunning,
) (string, *agentctl.InstanceInfo, error) {
	token, err := control.Handshake(ctx, req.BootstrapNonce)
	if err != nil {
		return "", nil, err
	}
	control.SetAuthToken(token)
	info, err := control.GetInstance(ctx, req.InstanceID)
	if err != nil {
		return "", nil, err
	}
	if !sameRemoteDockerInstance(info, record, req) {
		return "", nil, errors.New("remote Docker agentctl identity changed during authentication refresh")
	}
	return token, info, nil
}

func sameRemoteDockerInstance(
	info *agentctl.InstanceInfo,
	record *models.ExecutorRunning,
	req *ExecutorCreateRequest,
) bool {
	return info != nil && info.ID == req.InstanceID && info.SessionID == record.SessionID && info.TaskID == record.TaskID
}

func (r *RemoteDockerExecutor) trackRecoveredRemoteDocker(
	req *ExecutorCreateRequest,
	session *remoteDockerSession,
	instance *ExecutorInstance,
) bool {
	target := make(map[string]interface{}, len(remoteDockerTargetKeys))
	for _, key := range remoteDockerTargetKeys {
		if value, ok := req.Metadata[key]; ok {
			target[key] = value
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sessions[instance.InstanceID] != nil {
		return false
	}
	r.sessions[instance.InstanceID] = session
	r.targets[instance.InstanceID] = target
	return true
}

func discardRecoveredRemoteDocker(
	r *RemoteDockerExecutor,
	instanceID string,
	session *remoteDockerSession,
	client *agentctl.Client,
) func() {
	var once sync.Once
	return func() {
		once.Do(func() {
			r.mu.Lock()
			if r.sessions[instanceID] == session {
				delete(r.sessions, instanceID)
				delete(r.targets, instanceID)
			}
			r.mu.Unlock()
			if trackedClient := session.takeAgentctlClient(); trackedClient != nil {
				trackedClient.Close()
			} else if client != nil {
				client.Close()
			}
			_ = session.close()
		})
	}
}

var _ DetailedRecoveryBackend = (*RemoteDockerExecutor)(nil)
