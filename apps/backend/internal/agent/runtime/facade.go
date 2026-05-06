package runtime

import (
	"context"
	"fmt"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
)

// New returns a Runtime backed by the supplied Backend (typically a
// *lifecycle.Manager).
func New(backend Backend) Runtime {
	return &facade{backend: backend}
}

// facade is the default Runtime implementation: a thin adapter over
// the lifecycle Manager (or any Backend).
type facade struct {
	backend Backend
}

// Launch translates a LaunchSpec into a lifecycle.LaunchRequest and
// dispatches to the backend.
//
// If `spec.Metadata["launch_request"]` is a `*lifecycle.LaunchRequest`,
// it is used as the base request and LaunchSpec's typed fields are
// applied on top — this is how richer launch parameters (worktrees,
// multi-repo specs, attachments) reach the runtime in Phase 1 without
// canonicalising them onto LaunchSpec yet.
func (f *facade) Launch(ctx context.Context, spec LaunchSpec) (ExecutionRef, error) {
	req := launchRequestFromSpec(spec)
	exec, err := f.backend.Launch(ctx, req)
	if err != nil {
		return ExecutionRef{}, err
	}
	return executionRefFromAgentExecution(exec), nil
}

// Resume sends a follow-up prompt to an existing execution. Attachments
// and dispatchOnly are not surfaced on Runtime.Resume in Phase 1; callers
// needing them should use the lifecycle Manager directly until Phase 3
// canonicalises richer prompt semantics.
func (f *facade) Resume(ctx context.Context, executionID string, prompt string) error {
	if executionID == "" {
		return fmt.Errorf("runtime: executionID is required")
	}
	_, err := f.backend.PromptAgent(ctx, executionID, prompt, nil, false)
	return err
}

// Stop terminates an execution.
func (f *facade) Stop(ctx context.Context, executionID string, reason string) error {
	if executionID == "" {
		return fmt.Errorf("runtime: executionID is required")
	}
	return f.backend.StopAgentWithReason(ctx, executionID, reason, false)
}

// GetExecution returns a snapshot view of an execution.
func (f *facade) GetExecution(_ context.Context, executionID string) (*Execution, error) {
	if executionID == "" {
		return nil, fmt.Errorf("runtime: executionID is required")
	}
	exec, ok := f.backend.GetExecution(executionID)
	if !ok || exec == nil {
		return nil, ErrNotFound
	}
	return executionFromAgentExecution(exec), nil
}

// SubscribeEvents is best-effort in Phase 1: the lifecycle manager
// publishes events through the process-wide event bus rather than
// per-execution channels, so we can't synthesize a raw event stream
// without coupling Runtime to the bus subject conventions. Callers that
// need raw streams should use the agentctl client; Phase 2/3 may add a
// per-execution channel in the lifecycle manager and wire it through here.
func (f *facade) SubscribeEvents(_ context.Context, _ string) (<-chan Event, error) {
	return nil, ErrUnsupported
}

// SetMcpMode delegates to the backend.
func (f *facade) SetMcpMode(ctx context.Context, executionID string, mode string) error {
	if executionID == "" {
		return fmt.Errorf("runtime: executionID is required")
	}
	return f.backend.SetMcpMode(ctx, executionID, mode)
}

// launchRequestFromSpec builds the lifecycle.LaunchRequest the backend
// expects. If the caller supplies a pre-built *lifecycle.LaunchRequest
// in Metadata["launch_request"], we use it as the base and overlay the
// typed LaunchSpec fields on top.
func launchRequestFromSpec(spec LaunchSpec) *lifecycle.LaunchRequest {
	req := &lifecycle.LaunchRequest{}
	if base, ok := spec.Metadata["launch_request"].(*lifecycle.LaunchRequest); ok && base != nil {
		copy := *base
		req = &copy
	}
	if spec.AgentProfileID != "" {
		req.AgentProfileID = spec.AgentProfileID
	}
	if spec.ExecutorID != "" {
		req.ExecutorType = spec.ExecutorID
	}
	if spec.Workspace.Path != "" {
		req.WorkspacePath = spec.Workspace.Path
	}
	if spec.Workspace.RepositoryID != "" {
		req.RepositoryID = spec.Workspace.RepositoryID
	}
	if spec.Workspace.IsEphemeral {
		req.IsEphemeral = true
	}
	if spec.Prompt != "" {
		req.TaskDescription = spec.Prompt
	}
	if spec.PriorACPSession != "" {
		req.ACPSessionID = spec.PriorACPSession
	}
	if spec.McpMode != "" {
		req.McpMode = spec.McpMode
	}
	if len(spec.Metadata) > 0 {
		if req.Metadata == nil {
			req.Metadata = map[string]interface{}{}
		}
		for k, v := range spec.Metadata {
			if k == "launch_request" {
				continue
			}
			req.Metadata[k] = v
		}
	}
	return req
}

func executionRefFromAgentExecution(exec *lifecycle.AgentExecution) ExecutionRef {
	if exec == nil {
		return ExecutionRef{}
	}
	return ExecutionRef{
		ID:          exec.ID,
		SessionID:   exec.SessionID,
		AgentctlURL: exec.AgentctlURL(),
		StartedAt:   exec.StartedAt,
	}
}

func executionFromAgentExecution(exec *lifecycle.AgentExecution) *Execution {
	if exec == nil {
		return nil
	}
	out := &Execution{
		ID:             exec.ID,
		SessionID:      exec.SessionID,
		TaskID:         exec.TaskID,
		AgentProfileID: exec.AgentProfileID,
		WorkspacePath:  exec.WorkspacePath,
		AgentctlURL:    exec.AgentctlURL(),
		Status:         exec.Status,
		StartedAt:      exec.StartedAt,
		FinishedAt:     exec.FinishedAt,
		ExitCode:       exec.ExitCode,
		ErrorMessage:   exec.ErrorMessage,
		ACPSessionID:   exec.ACPSessionID,
		Metadata:       exec.Metadata,
	}
	return out
}

// Compile-time check: facade satisfies Runtime.
var _ Runtime = (*facade)(nil)
