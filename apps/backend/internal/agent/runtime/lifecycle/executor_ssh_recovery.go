package lifecycle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"

	"go.uber.org/zap"
	"golang.org/x/crypto/ssh"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentruntime"
	"github.com/kandev/kandev/internal/task/models"
)

func (r *SSHExecutor) RecoverInstancesDetailed(
	ctx context.Context,
	records []*models.ExecutorRunning,
) ([]*ExecutorInstance, map[string]RecoveryCandidateOutcome, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	var instances []*ExecutorInstance
	outcomes := make(map[string]RecoveryCandidateOutcome)
	for _, record := range records {
		if record == nil || record.Runtime != agentruntime.RuntimeSSH || record.SessionID == "" {
			continue
		}
		instance, outcome := r.recoverSSHRecord(ctx, record)
		if instance != nil {
			delete(outcomes, record.SessionID)
			instances = append(instances, instance)
			continue
		}
		outcomes[record.SessionID] = outcome
	}
	return instances, outcomes, nil
}

func (r *SSHExecutor) recoverSSHRecord(
	ctx context.Context,
	record *models.ExecutorRunning,
) (*ExecutorInstance, RecoveryCandidateOutcome) {
	identity, ok := sshRecoveryIdentity(record)
	if !ok {
		return nil, RecoveryOutcomeUnknown
	}

	recoveryCtx, cancel := context.WithTimeout(ctx, sshAgentctlHealthTimeout)
	defer cancel()
	target, err := r.targetFromMetadata(record.Metadata)
	if err != nil {
		r.logger.Debug("ssh recovery cannot resolve saved target", zap.String("session_id", record.SessionID), zap.Error(err))
		return nil, RecoveryOutcomeUnknown
	}
	client, err := dialSSH(recoveryCtx, target)
	if err != nil {
		r.logger.Debug("ssh recovery cannot connect to saved target", zap.String("session_id", record.SessionID), zap.Error(err))
		return nil, RecoveryOutcomeUnknown
	}

	var forwarder *SSHPortForwarder
	var runtimeTunnel *sshRuntimeAPITunnel
	transferred := false
	defer func() {
		if transferred {
			return
		}
		if forwarder != nil {
			_ = forwarder.Close()
		}
		_ = runtimeTunnel.Close()
		_ = r.closeSSHClient(client)
	}()

	instanceInfo, controlPort, outcome := r.inspectSSHRecoveryInstance(recoveryCtx, client, record, identity)
	if outcome != "" {
		return nil, outcome
	}

	req := sshRecoveryRequest(record, identity)
	runtimeTunnel, _, err = openSSHRuntimeAPITunnelForRequest(client, req)
	if err != nil {
		r.logger.Debug("ssh recovery cannot attach saved runtime API tunnel", zap.String("session_id", record.SessionID), zap.Error(err))
		return nil, RecoveryOutcomeUnknown
	}
	forwarder, err = StartPortForward(client, identity.port, r.logger)
	if err != nil {
		r.logger.Debug("ssh recovery cannot open local agentctl forward", zap.String("session_id", record.SessionID), zap.Error(err))
		return nil, RecoveryOutcomeUnknown
	}
	if err := recoveryCtx.Err(); err != nil {
		return nil, RecoveryOutcomeUnknown
	}

	instance, registered := r.registerRecoveredSSHState(record, identity, target, client, forwarder, runtimeTunnel, req, instanceInfo, controlPort)
	if instance == nil {
		return nil, RecoveryOutcomeUnknown
	}
	transferred = registered
	return instance, ""
}

func (r *SSHExecutor) inspectSSHRecoveryInstance(
	ctx context.Context,
	client *ssh.Client,
	record *models.ExecutorRunning,
	identity sshRecoveryProcessIdentity,
) (*agentctl.InstanceInfo, int, RecoveryCandidateOutcome) {
	verify := r.verifyRemoteIdentity
	if verify == nil {
		verify = verifyRemoteAgentctlIdentity
	}
	ours, err := verify(ctx, client, identity.pid, identity.sessionDir, identity.taskDir)
	if err != nil {
		r.logger.Debug("ssh recovery cannot prove saved process identity", zap.String("session_id", record.SessionID), zap.Error(err))
		return nil, 0, RecoveryOutcomeUnknown
	}
	if !ours {
		return nil, 0, RecoveryOutcomeNoMatchingInstance
	}
	instanceInfo, controlPort, err := r.readSSHRecoveryInstanceInfo(ctx, client, record, identity)
	if err != nil {
		r.logger.Debug("ssh recovery cannot read exact live instance metadata", zap.String("session_id", record.SessionID), zap.Error(err))
		return nil, 0, RecoveryOutcomeUnknown
	}
	return instanceInfo, controlPort, ""
}

type sshRecoveryProcessIdentity struct {
	pid         int
	port        int
	controlPort int
	sessionDir  string
	taskDir     string
	nativeID    string
	backendID   string
	authToken   string
}

func sshRecoveryIdentity(record *models.ExecutorRunning) (sshRecoveryProcessIdentity, bool) {
	if !hasCanonicalSSHRecoveryRecordIdentity(record) {
		return sshRecoveryProcessIdentity{}, false
	}
	identity := sshRecoveryProcessIdentity{
		backendID: strings.TrimSpace(record.AgentExecutionID),
		authToken: strings.TrimSpace(record.TransientAuthToken),
		taskDir:   strings.TrimSpace(getMetadataString(record.Metadata, MetadataKeySSHRemoteTaskDir)),
		nativeID:  strings.TrimSpace(getMetadataString(record.Metadata, MetadataKeySSHAgentctlInstanceID)),
	}
	controlPort, ok := sshRecoveryControlPort(record.Metadata)
	if !ok {
		return sshRecoveryProcessIdentity{}, false
	}
	identity.controlPort = controlPort
	metadataPID, sessionDir, port, ok := sshRecoveryAgentctlTarget(record)
	if !ok {
		return sshRecoveryProcessIdentity{}, false
	}
	identity.pid = metadataPID
	identity.port = port
	identity.sessionDir = sessionDir
	if !hasCompleteSSHRecoveryCredentials(identity) {
		return sshRecoveryProcessIdentity{}, false
	}
	return identity, true
}

func hasCanonicalSSHRecoveryRecordIdentity(record *models.ExecutorRunning) bool {
	if record == nil {
		return false
	}
	return strings.TrimSpace(record.SessionID) != "" && record.SessionID == strings.TrimSpace(record.SessionID) &&
		strings.TrimSpace(record.TaskID) != "" && record.TaskID == strings.TrimSpace(record.TaskID) &&
		record.AgentExecutionID == strings.TrimSpace(record.AgentExecutionID)
}

func sshRecoveryControlPort(metadata map[string]interface{}) (int, bool) {
	rawPort := strings.TrimSpace(getMetadataString(metadata, MetadataKeySSHRemoteControlPort))
	if rawPort == "" {
		return 0, true
	}
	port, err := strconv.Atoi(rawPort)
	return port, err == nil && port >= 1 && port <= 65535
}

func sshRecoveryAgentctlTarget(record *models.ExecutorRunning) (int, string, int, bool) {
	metadataPID, sessionDir, ok := persistedSSHAgentctlTarget(record.Metadata)
	port, err := strconv.Atoi(strings.TrimSpace(getMetadataString(record.Metadata, MetadataKeySSHRemoteAgentctlPort)))
	if !ok || err != nil || port < 1 || port > 65535 || record.PID != metadataPID {
		return 0, "", 0, false
	}
	return metadataPID, sessionDir, port, true
}

func hasCompleteSSHRecoveryCredentials(identity sshRecoveryProcessIdentity) bool {
	return identity.backendID != "" && identity.nativeID != "" && identity.authToken != "" && identity.taskDir != ""
}

func parseSSHControlPortMetadata(metadata map[string]interface{}) int {
	port, ok := sshRecoveryControlPort(metadata)
	if !ok {
		return 0
	}
	return port
}

func (r *SSHExecutor) readSSHRecoveryInstanceInfo(
	ctx context.Context,
	client *ssh.Client,
	record *models.ExecutorRunning,
	identity sshRecoveryProcessIdentity,
) (*agentctl.InstanceInfo, int, error) {
	controlPort := identity.controlPort
	if controlPort == 0 {
		logPath := shellQuote(identity.sessionDir + "/agentctl.log")
		command := "head -c 32768 " + logPath + " 2>/dev/null; printf '\\n'; tail -c 32768 " + logPath + " 2>/dev/null"
		output, _, err := runSSHCommand(ctx, client, command)
		if err != nil {
			return nil, 0, fmt.Errorf("read exact agentctl session log: %w", err)
		}
		controlPort, err = latestSSHAgentctlControlPort(output)
		if err != nil {
			return nil, 0, err
		}
	}
	forwarder, err := StartPortForward(client, controlPort, r.logger)
	if err != nil {
		return nil, 0, fmt.Errorf("forward exact agentctl control listener: %w", err)
	}
	defer func() { _ = forwarder.Close() }()
	control := agentctl.NewControlClient(sshAgentctlLoopbackHost, forwarder.LocalPort(), r.logger,
		agentctl.WithControlAuthToken(identity.authToken))
	defer control.Close()
	info, err := control.GetInstance(ctx, identity.nativeID)
	if err != nil {
		return nil, 0, fmt.Errorf("read exact agentctl instance: %w", err)
	}
	if info == nil || info.ID != identity.nativeID || info.SessionID != record.SessionID ||
		info.TaskID != record.TaskID || info.Port != identity.port || info.WorkspacePath != identity.taskDir {
		return nil, 0, errors.New("live agentctl metadata does not match the persisted SSH instance")
	}
	return info, controlPort, nil
}

func latestSSHAgentctlControlPort(output string) (int, error) {
	const message = "HTTP server bound successfully"
	lines := strings.Split(output, "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		line := lines[index]
		if !strings.Contains(line, message) {
			continue
		}
		address, err := sshAgentctlBoundAddress(line)
		if err != nil {
			return 0, err
		}
		host, portValue, err := net.SplitHostPort(address)
		if err != nil {
			return 0, errors.New("latest agentctl control listener has an invalid address")
		}
		parsedHost := net.ParseIP(strings.Trim(host, "[]"))
		if !strings.EqualFold(host, "localhost") && (parsedHost == nil || !parsedHost.IsLoopback()) {
			return 0, errors.New("agentctl control listener is not bound to loopback")
		}
		port, err := strconv.Atoi(portValue)
		if err != nil || port < 1 || port > 65535 {
			return 0, errors.New("latest agentctl control listener has an invalid port")
		}
		return port, nil
	}
	return 0, errors.New("agentctl session log does not contain a valid control listener")
}

func sshAgentctlBoundAddress(line string) (string, error) {
	var fields struct {
		Message string `json:"msg"`
		Address string `json:"address"`
	}
	if err := json.Unmarshal([]byte(line), &fields); err == nil && fields.Message == "HTTP server bound successfully" {
		if fields.Address == "" {
			return "", errors.New("agentctl control listener log entry has no address")
		}
		return fields.Address, nil
	}
	if position := strings.Index(line, "address="); position >= 0 {
		value := strings.TrimSpace(line[position+len("address="):])
		if fields := strings.Fields(value); len(fields) > 0 {
			value = strings.Trim(fields[0], `"',;`)
		}
		if value != "" {
			return value, nil
		}
	}
	return "", errors.New("agentctl control listener log entry has no parseable address")
}

func sshRecoveryRequest(record *models.ExecutorRunning, identity sshRecoveryProcessIdentity) *ExecutorCreateRequest {
	return &ExecutorCreateRequest{
		InstanceID: identity.backendID,
		TaskID:     record.TaskID,
		SessionID:  record.SessionID,
		AuthToken:  identity.authToken,
		Metadata:   cloneSSHMetadata(record.Metadata),
		Env:        make(map[string]string),
	}
}

func (r *SSHExecutor) registerRecoveredSSHState(
	record *models.ExecutorRunning,
	identity sshRecoveryProcessIdentity,
	target *SSHTarget,
	client *ssh.Client,
	forwarder *SSHPortForwarder,
	runtimeTunnel *sshRuntimeAPITunnel,
	req *ExecutorCreateRequest,
	instanceInfo *agentctl.InstanceInfo,
	controlPort int,
) (*ExecutorInstance, bool) {
	metadata := cloneSSHMetadata(req.Metadata)
	metadata[MetadataKeySSHLocalForwardPort] = strconv.Itoa(forwarder.LocalPort())
	metadata[MetadataKeySSHRemoteControlPort] = strconv.Itoa(controlPort)
	metadata[MetadataKeyReuseExistingProcess] = true
	metadata[MetadataKeyIsRemote] = true
	state := &sshSessionState{
		target:    target,
		client:    client,
		forwarder: forwarder,
		agentctlClient: agentctl.NewClient(sshAgentctlLoopbackHost, forwarder.LocalPort(), r.logger,
			agentctl.WithExecutionID(identity.nativeID),
			agentctl.WithSessionID(record.SessionID),
			agentctl.WithAuthToken(identity.authToken)),
		pid:              identity.pid,
		remoteDir:        identity.sessionDir,
		remoteTaskDir:    identity.taskDir,
		authToken:        identity.authToken,
		reusingProcess:   true,
		metadata:         metadata,
		prepareEnv:       make(map[string]string),
		runtimeAPITunnel: runtimeTunnel,
		port:             identity.port,
		controlPort:      controlPort,
		workdirRoot:      r.workdirRoot(metadata),
	}

	r.mu.Lock()
	if r.sessions[identity.backendID] != nil {
		r.mu.Unlock()
		return nil, false
	}
	r.sessions[identity.backendID] = state
	r.startWatchdogLocked(identity.backendID, state)
	r.mu.Unlock()

	instance := &ExecutorInstance{
		InstanceID:           identity.backendID,
		TaskID:               record.TaskID,
		SessionID:            record.SessionID,
		RuntimeName:          r.Name(),
		Client:               state.agentctlClient,
		AuthToken:            identity.authToken,
		WorkspacePath:        identity.taskDir,
		Env:                  cloneStringMap(instanceInfo.Env),
		WorkspaceSourceRoots: append([]string(nil), instanceInfo.WorkspaceSourceRoots...),
		Metadata:             metadata,
	}
	instance.DiscardRecovery = func() {
		r.discardRecoveredSSHState(identity.backendID, state)
	}
	return instance, true
}

func (r *SSHExecutor) discardRecoveredSSHState(instanceID string, state *sshSessionState) {
	r.mu.Lock()
	if r.sessions[instanceID] == state {
		delete(r.sessions, instanceID)
	}
	r.mu.Unlock()

	r.stopWatchdogAndWait(state)
	_ = r.closeForwarderOnce(state)
	_ = state.runtimeAPITunnel.Close()
	_ = r.closeClientOnce(state)
	r.awaitProbeExit(state)
}
