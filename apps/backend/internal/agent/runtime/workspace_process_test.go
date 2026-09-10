package runtime

import (
	"context"
	"testing"
	"time"

	agentctl "github.com/kandev/kandev/internal/agent/runtime/agentctl"
	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

type workspaceProcessBackendStub struct {
	start func(context.Context, lifecycle.WorkspaceProcessRequest) (*agentctl.ProcessInfo, error)
}

func (s workspaceProcessBackendStub) StartWorkspaceProcess(ctx context.Context, req lifecycle.WorkspaceProcessRequest) (*agentctl.ProcessInfo, error) {
	return s.start(ctx, req)
}

func (s workspaceProcessBackendStub) GetWorkspaceProcess(context.Context, string, string, bool) (*agentctl.ProcessInfo, error) {
	return nil, nil
}

func (s workspaceProcessBackendStub) StopWorkspaceProcess(context.Context, string, string) error {
	return nil
}

func TestWorkspaceProcessRunnerAdaptsTheLifecycleBackend(t *testing.T) {
	called := false
	runner := NewWorkspaceProcessRunner(workspaceProcessBackendStub{
		start: func(_ context.Context, req lifecycle.WorkspaceProcessRequest) (*agentctl.ProcessInfo, error) {
			called = true
			if req.RunID != "run-1" || req.Timeout != time.Second {
				t.Fatalf("request = %+v", req)
			}
			return &agentctl.ProcessInfo{ID: "process-1"}, nil
		},
	})
	info, err := runner.Start(context.Background(), WorkspaceProcessRequest{RunID: "run-1", Timeout: time.Second})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !called || info.ID != "process-1" {
		t.Fatalf("runner result = %+v called=%v", info, called)
	}
}
