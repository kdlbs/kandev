package lifecycle

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agentctl/server/process"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentruntime"
)

type executionPassthroughRunner interface {
	Start(context.Context, process.InteractiveStartRequest) (*process.InteractiveProcessInfo, error)
	Stop(context.Context, string) error
	WriteStdin(string, string) error
	ResizeBySession(string, uint16, uint16) error
	GetBuffer(string) ([]process.ProcessOutputChunk, bool)
	WaitForFirstIdle(context.Context, string) error
	IsProcessReadyOrPending(string) bool
	ConnectSessionWebSocket(string) bool
	HasActiveWebSocketBySession(string) bool
	WriteToDirectOutputBySession(string, []byte) error
}

type remotePassthroughState struct {
	snapshot           types.PassthroughSnapshot
	preparedGeneration uint64
	observerCancel     context.CancelFunc
	observerTarget     types.PassthroughTarget
	observerGeneration uint64
}

func isSSHPassthrough(execution *AgentExecution) bool {
	return execution != nil && (execution.ExecutorType == executorTypeSSH || execution.RuntimeName == agentruntime.RuntimeSSH)
}

func (m *Manager) passthroughRunnerForExecution(execution *AgentExecution) executionPassthroughRunner {
	if m.passthroughRunnerOverride != nil {
		return m.passthroughRunnerOverride
	}
	if isSSHPassthrough(execution) {
		return &remotePassthroughRunner{execution: execution, manager: m}
	}
	if execution != nil && execution.RuntimeName != "" && execution.RuntimeName != agentruntime.RuntimeStandalone {
		return nil
	}
	runner := m.GetInteractiveRunner()
	if runner == nil {
		return nil
	}
	return runner
}

type remotePassthroughRunner struct {
	execution *AgentExecution
	manager   *Manager
}

func (r *remotePassthroughRunner) client() (*agentctl.Client, func(), error) {
	client, release := r.execution.AcquireAgentCtlClient()
	if client == nil {
		release()
		return nil, func() {}, errors.New("remote passthrough controller is unavailable")
	}
	return client, release, nil
}

func (r *remotePassthroughRunner) snapshot(ctx context.Context) (*types.PassthroughSnapshot, error) {
	client, release, err := r.client()
	if err != nil {
		return nil, err
	}
	defer release()
	snapshot, err := client.GetPassthrough(ctx)
	if err != nil {
		return nil, err
	}
	if snapshot.SessionID != r.execution.SessionID {
		return nil, agentctl.ErrPassthroughConflict
	}
	expectedID := r.execution.metadataString(MetadataKeySSHAgentctlInstanceID)
	if expectedID == "" {
		expectedID = r.execution.ID
	}
	if snapshot.InstanceID != expectedID {
		return nil, agentctl.ErrPassthroughConflict
	}
	r.execution.remotePassthroughMu.Lock()
	r.execution.remotePassthrough.snapshot = *snapshot
	r.execution.remotePassthroughMu.Unlock()
	return snapshot, nil
}

func (r *remotePassthroughRunner) target(processID string) (types.PassthroughTarget, error) {
	r.execution.remotePassthroughMu.Lock()
	defer r.execution.remotePassthroughMu.Unlock()
	target := r.execution.remotePassthrough.snapshot.PassthroughTarget
	if target.ProcessID != processID || processID == "" {
		return target, agentctl.ErrPassthroughConflict
	}
	return target, nil
}

func (r *remotePassthroughRunner) Start(ctx context.Context, request process.InteractiveStartRequest) (*process.InteractiveProcessInfo, error) {
	if !request.LaunchWrapper.IsZero() {
		return nil, errors.New("remote terminal transport does not support launch wrappers")
	}
	snapshot, err := r.snapshot(ctx)
	if err != nil {
		return nil, err
	}
	client, release, err := r.client()
	if err != nil {
		return nil, err
	}
	defer release()
	remote := types.PassthroughStartRequest{
		SessionID: request.SessionID, OperationID: uuid.NewString(), ExpectedGeneration: snapshot.Generation,
		Command: request.Command, Env: request.Env, StripEnv: request.StripEnv, PromptPattern: request.PromptPattern,
		IdleTimeout: request.IdleTimeout, StatusDetector: request.StatusDetector, CheckInterval: request.CheckInterval,
		StabilityWindow: request.StabilityWindow, Cols: request.DefaultCols, Rows: request.DefaultRows,
	}
	if err := r.recordStart(ctx, snapshot, remote.OperationID, request.InitialPromptChunks); err != nil {
		return nil, err
	}
	started, err := client.StartPassthrough(ctx, remote)
	if err != nil {
		probeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		observed, probeErr := client.LookupPassthroughStart(probeCtx, remote.OperationID)
		if probeErr != nil {
			return nil, err
		}
		started = observed
	}
	if started.InstanceID != snapshot.InstanceID || started.SessionID != snapshot.SessionID || started.OperationID != remote.OperationID {
		return nil, agentctl.ErrPassthroughConflict
	}
	if err := r.recordStart(ctx, started, remote.OperationID, request.InitialPromptChunks); err != nil {
		return nil, err
	}
	r.execution.remotePassthroughMu.Lock()
	r.execution.remotePassthrough.snapshot = *started
	r.execution.remotePassthroughMu.Unlock()
	return remotePassthroughInfo(started), nil
}

func remotePassthroughInfo(s *types.PassthroughSnapshot) *process.InteractiveProcessInfo {
	return &process.InteractiveProcessInfo{ID: s.ProcessID, SessionID: s.SessionID, WorkingDir: s.WorkspacePath,
		OSPID: s.OSPID, Status: s.Status, StartedAt: s.StartedAt, ExitCode: s.ExitCode}
}

func (r *remotePassthroughRunner) Stop(ctx context.Context, processID string) error {
	target, err := r.target(processID)
	if err != nil {
		return err
	}
	client, release, err := r.client()
	if err != nil {
		return err
	}
	defer release()
	return client.StopPassthrough(ctx, target)
}

func (r *remotePassthroughRunner) WriteStdin(processID, data string) error {
	return r.writeStdinWithID(processID, data, uuid.Must(uuid.NewV7()).String())
}

func (r *remotePassthroughRunner) writeStdinWithID(processID, data, submissionID string) error {
	_, err := r.writeStdinReceipt(processID, data, submissionID)
	return err
}

func (r *remotePassthroughRunner) writeStdinReceipt(processID, data, submissionID string) (types.PassthroughInputResult, error) {
	target, err := r.target(processID)
	if err != nil {
		return types.PassthroughInputResult{}, err
	}
	client, release, err := r.client()
	if err != nil {
		return types.PassthroughInputResult{}, err
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	result, err := client.WritePassthrough(ctx, types.PassthroughInputRequest{PassthroughTarget: target, SubmissionID: submissionID, Data: data})
	if err != nil {
		receipt, receiptErr := client.PassthroughInputReceipt(ctx, target, submissionID)
		if receiptErr != nil {
			return types.PassthroughInputResult{}, err
		}
		result = receipt
	}
	if result.Outcome != remotePromptWritten {
		return types.PassthroughInputResult{}, errors.New("remote prompt delivery is uncertain. Reconnect before retrying")
	}
	return *result, nil
}

func (r *remotePassthroughRunner) ResizeBySession(sessionID string, cols, rows uint16) error {
	if sessionID != r.execution.SessionID {
		return agentctl.ErrPassthroughConflict
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	target, err := r.target(r.execution.PassthroughProcessID)
	if err != nil {
		return err
	}
	client, release, err := r.client()
	if err != nil {
		return err
	}
	defer release()
	return client.ResizePassthrough(ctx, types.PassthroughResizeRequest{PassthroughTarget: target, Cols: cols, Rows: rows})
}

func (r *remotePassthroughRunner) GetBuffer(processID string) ([]process.ProcessOutputChunk, bool) {
	target, err := r.target(processID)
	if err != nil {
		return nil, false
	}
	client, release, err := r.client()
	if err != nil {
		return nil, false
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	data, err := client.PassthroughBuffer(ctx, target)
	return []process.ProcessOutputChunk{{Data: data}}, err == nil
}

func (r *remotePassthroughRunner) WaitForFirstIdle(ctx context.Context, processID string) error {
	target, err := r.target(processID)
	if err != nil {
		return err
	}
	client, release, err := r.client()
	if err != nil {
		return err
	}
	defer release()
	return client.WaitForPassthroughIdle(ctx, target)
}

func (r *remotePassthroughRunner) IsProcessReadyOrPending(processID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, err := r.snapshot(ctx)
	if err != nil {
		// An unavailable controller cannot prove that its process has stopped.
		return true
	}
	return snapshot.ProcessID == processID && snapshot.Status == types.ProcessStatusRunning
}

func (r *remotePassthroughRunner) HasActiveWebSocketBySession(sessionID string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snapshot, err := r.snapshot(ctx)
	return err == nil && snapshot.SessionID == sessionID && snapshot.HasTerminal
}

func (r *remotePassthroughRunner) ConnectSessionWebSocket(processID string) bool {
	return r.IsProcessReadyOrPending(processID)
}

func (r *remotePassthroughRunner) WriteToDirectOutputBySession(sessionID string, data []byte) error {
	if sessionID != r.execution.SessionID {
		return agentctl.ErrPassthroughConflict
	}
	r.execution.remotePassthroughMu.Lock()
	target := r.execution.remotePassthrough.snapshot.PassthroughTarget
	r.execution.remotePassthroughMu.Unlock()
	client, release, err := r.client()
	if err != nil {
		return err
	}
	defer release()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return client.PassthroughNotice(ctx, target, string(data))
}

func (m *Manager) StreamRemotePassthrough(ctx context.Context, execution *AgentExecution) (*websocket.Conn, error) {
	if !isSSHPassthrough(execution) {
		return nil, errors.New("execution does not use SSH passthrough")
	}
	runner := &remotePassthroughRunner{execution: execution}
	target, err := runner.target(execution.PassthroughProcessID)
	if err != nil {
		return nil, err
	}
	client, release, err := runner.client()
	if err != nil {
		return nil, err
	}
	defer release()
	return client.StreamPassthrough(ctx, target)
}
