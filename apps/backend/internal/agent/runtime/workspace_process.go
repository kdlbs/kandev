package runtime

import (
	"context"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// WorkspaceProcessRequest describes one command that must run inside a bound
// execution workspace. RunID is the stable agentctl idempotency identity.
type WorkspaceProcessRequest struct {
	RunID          string
	SessionID      string
	ExecutionID    string
	Command        string
	WorkingDir     string
	Timeout        time.Duration
	Kind           string
	BufferMaxBytes int64
}

// WorkspaceProcessInfo is the runtime-owned view of a managed workspace
// process. The concrete wire type remains private to the runtime seam.
type WorkspaceProcessInfo = agentctl.ProcessInfo

// WorkspaceProcessOutputChunk is one buffered workspace-process output chunk.
type WorkspaceProcessOutputChunk = agentctl.ProcessOutputChunk

// WorkspaceProcessStatus is the lifecycle status of a workspace process.
type WorkspaceProcessStatus = agentctl.ProcessStatus

// WorkspaceProcessRunner is the narrow runtime seam for workflow and other
// task-owned commands. It does not expose lifecycle internals or agent launch.
type WorkspaceProcessRunner interface {
	Start(ctx context.Context, request WorkspaceProcessRequest) (*WorkspaceProcessInfo, error)
	Get(ctx context.Context, executionID, processID string, includeOutput bool) (*WorkspaceProcessInfo, error)
	Stop(ctx context.Context, executionID, processID string) error
}

type workspaceProcessBackend interface {
	StartWorkspaceProcess(context.Context, lifecycle.WorkspaceProcessRequest) (*WorkspaceProcessInfo, error)
	GetWorkspaceProcess(context.Context, string, string, bool) (*WorkspaceProcessInfo, error)
	StopWorkspaceProcess(context.Context, string, string) error
}

type workspaceProcessFacade struct {
	backend workspaceProcessBackend
}

// NewWorkspaceProcessRunner adapts the lifecycle manager to the narrow
// runtime-owned workspace process interface.
func NewWorkspaceProcessRunner(backend workspaceProcessBackend) WorkspaceProcessRunner {
	return &workspaceProcessFacade{backend: backend}
}

func (f *workspaceProcessFacade) Start(ctx context.Context, request WorkspaceProcessRequest) (*WorkspaceProcessInfo, error) {
	return f.backend.StartWorkspaceProcess(ctx, lifecycle.WorkspaceProcessRequest{
		RunID: request.RunID, SessionID: request.SessionID, ExecutionID: request.ExecutionID,
		Command: request.Command, WorkingDir: request.WorkingDir, Timeout: request.Timeout,
		Kind: request.Kind, BufferMaxBytes: request.BufferMaxBytes,
	})
}

func (f *workspaceProcessFacade) Get(ctx context.Context, executionID, processID string, includeOutput bool) (*WorkspaceProcessInfo, error) {
	return f.backend.GetWorkspaceProcess(ctx, executionID, processID, includeOutput)
}

func (f *workspaceProcessFacade) Stop(ctx context.Context, executionID, processID string) error {
	return f.backend.StopWorkspaceProcess(ctx, executionID, processID)
}
