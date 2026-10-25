package process

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/agentctl/types"
)

const passthroughInputWritten = "written"

var ErrPassthroughConflict = errors.New("remote passthrough ownership conflict")
var ErrPassthroughNotFound = errors.New("remote passthrough process not found")

type passthroughStartResult struct {
	digest   [32]byte
	snapshot types.PassthroughSnapshot
	err      error
}

type passthroughInputResult struct {
	digest   [32]byte
	done     chan struct{}
	outcome  string
	sequence uint64
}

// PassthroughOwner serializes one instance's PTY admission against ACP startup.
type PassthroughOwner struct {
	manager         *Manager
	runner          *InteractiveRunner
	mu              sync.Mutex
	current         types.PassthroughSnapshot
	starts          map[string]passthroughStartResult
	inputs          map[string]*passthroughInputResult
	inputFloor      string
	events          []types.PassthroughEvent
	wake            chan struct{}
	limit           int
	terminalOutputs map[string]chan []byte
	terminalCols    int
	terminalRows    int
}

func newPassthroughOwner(m *Manager) *PassthroughOwner {
	limit := m.cfg.DetachedEventLimit
	if limit <= 0 || limit > 4096 {
		limit = 512
	}
	o := &PassthroughOwner{
		manager: m, runner: NewInteractiveRunner(nil, m.logger, m.cfg.ProcessBufferMaxBytes),
		starts: make(map[string]passthroughStartResult), inputs: make(map[string]*passthroughInputResult),
		wake: make(chan struct{}), limit: limit,
		terminalOutputs: make(map[string]chan []byte),
	}
	o.runner.SetStatusCallback(o.onStatus)
	o.runner.SetTurnCompleteCallback(o.onIdle)
	return o
}

func (m *Manager) Passthrough() *PassthroughOwner { return m.passthrough }

func (o *PassthroughOwner) snapshotLocked() types.PassthroughSnapshot {
	snapshot := o.current
	cfg := o.manager.cfg
	snapshot.Version = types.PassthroughProtocolVersion
	snapshot.InstanceID, snapshot.SessionID = cfg.InstanceID, cfg.SessionID
	snapshot.WorkspacePath = cfg.WorkDir
	snapshot.ConfigPath = filepath.Join(cfg.WorkDir, ".kandev", "sessions", cfg.SessionID, "passthrough", "mcp-"+strconv.FormatUint(snapshot.Generation+1, 10)+".json")
	snapshot.MCPURL = fmt.Sprintf("http://127.0.0.1:%d/mcp", cfg.Port)
	if info, ok := o.runner.Get(snapshot.ProcessID, false); ok {
		snapshot.OSPID = info.OSPID
	}
	snapshot.HasTerminal = len(o.terminalOutputs) > 0
	return snapshot
}

func (o *PassthroughOwner) Snapshot() types.PassthroughSnapshot {
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.snapshotLocked()
}

func (o *PassthroughOwner) liveLocked() bool {
	return o.current.ProcessID != "" && o.current.Status == types.ProcessStatusRunning && o.runner.IsProcessReadyOrPending(o.current.ProcessID)
}

func (o *PassthroughOwner) Live() bool {
	if o == nil {
		return false
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	return o.liveLocked()
}

func (o *PassthroughOwner) Start(ctx context.Context, req types.PassthroughStartRequest) (types.PassthroughSnapshot, error) {
	m := o.manager
	m.startMu.Lock()
	defer m.startMu.Unlock()
	ctx, release, err := m.BeginOwnedOperation(ctx)
	if err != nil {
		return types.PassthroughSnapshot{}, err
	}
	defer release()
	if err := ctx.Err(); err != nil {
		return types.PassthroughSnapshot{}, err
	}
	if req.SessionID == "" || req.SessionID != m.cfg.SessionID {
		return types.PassthroughSnapshot{}, ErrPassthroughConflict
	}
	if req.OperationID == "" || len(req.OperationID) > 128 || len(req.Command) == 0 {
		return types.PassthroughSnapshot{}, errors.New("operation_id and command are required")
	}
	encoded, err := json.Marshal(req)
	if err != nil {
		return types.PassthroughSnapshot{}, err
	}
	digest := sha256.Sum256(encoded)
	id, previous, err := o.reserveStart(req, digest)
	if err != nil {
		return types.PassthroughSnapshot{}, err
	}
	if previous != nil {
		return previous.snapshot, previous.err
	}
	startRequest := o.runnerRequest(req, id)
	startRequest.Env, err = mergeAgentEnvIntoShellConfigWithError(m.cfg.AgentEnv, startRequest.Env)
	if err != nil {
		return o.finishStart(req.OperationID, digest, err)
	}
	_, startErr := o.runner.Start(ctx, startRequest)
	return o.finishStart(req.OperationID, digest, startErr)
}

func (o *PassthroughOwner) runnerRequest(req types.PassthroughStartRequest, id string) InteractiveStartRequest {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.terminalCols > 0 && o.terminalRows > 0 {
		req.Cols, req.Rows = o.terminalCols, o.terminalRows
	}
	return passthroughRunnerRequest(req, id, o.manager.cfg.WorkDir)
}

func (o *PassthroughOwner) reserveStart(req types.PassthroughStartRequest, digest [32]byte) (string, *passthroughStartResult, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if previous, ok := o.starts[req.OperationID]; ok {
		if previous.digest != digest {
			return "", nil, ErrPassthroughConflict
		}
		return "", &previous, nil
	}
	if o.liveLocked() || o.manager.Status() == StatusRunning || o.manager.Status() == StatusStarting || req.ExpectedGeneration != o.current.Generation {
		return "", nil, ErrPassthroughConflict
	}
	o.starts = make(map[string]passthroughStartResult)
	o.inputs = make(map[string]*passthroughInputResult)
	o.inputFloor = ""
	o.current = types.PassthroughSnapshot{
		PassthroughTarget: types.PassthroughTarget{ProcessID: uuid.NewString(), Generation: o.current.Generation + 1},
		OperationID:       req.OperationID, StartedAt: time.Now().UTC(), StartSequence: o.current.Sequence,
		Sequence: o.current.Sequence, Status: types.ProcessStatusRunning,
	}
	return o.current.ProcessID, nil, nil
}

func (o *PassthroughOwner) finishStart(operationID string, digest [32]byte, startErr error) (types.PassthroughSnapshot, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if startErr != nil {
		o.current.Status = types.ProcessStatusFailed
		startErr = errors.New("remote CLI could not start. Check that the selected CLI is installed and executable")
	}
	result := o.snapshotLocked()
	o.emitLocked("started", nil)
	o.starts[operationID] = passthroughStartResult{digest: digest, snapshot: result, err: startErr}
	return result, startErr
}

func passthroughRunnerRequest(req types.PassthroughStartRequest, id, workdir string) InteractiveStartRequest {
	return InteractiveStartRequest{
		ProcessID: id, SessionID: req.SessionID, Command: req.Command,
		LogCommand: []string{"remote CLI", "<arguments omitted>"}, WorkingDir: workdir,
		Env: req.Env, StripEnv: req.StripEnv, PromptPattern: req.PromptPattern,
		IdleTimeout: req.IdleTimeout, StatusDetector: req.StatusDetector,
		CheckInterval: req.CheckInterval, StabilityWindow: req.StabilityWindow,
		ImmediateStart: true, DefaultCols: req.Cols, DefaultRows: req.Rows, OwnProcessGroup: true,
	}
}

func (o *PassthroughOwner) Lookup(operationID string) (types.PassthroughSnapshot, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	result, ok := o.starts[operationID]
	if !ok {
		return types.PassthroughSnapshot{}, ErrPassthroughNotFound
	}
	if result.snapshot.ProcessID == o.current.ProcessID {
		return o.snapshotLocked(), result.err
	}
	return result.snapshot, result.err
}
