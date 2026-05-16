// Package runtime is the single seam between higher-level coordinators
// (the workflow engine, cron-driven trigger handlers, the orchestrator
// for user-initiated kanban launches) and the agent execution layer
// (lifecycle manager, executor backends, agentctl client).
//
// The runtime knows nothing about tasks, workflows, or office stages.
// It launches agents, resumes them with prompts, stops them, and exposes
// execution state. Anything task-shaped lives a layer above.
//
// Phase 1 of task-model-unification (ADR 0004) introduces this package.
// Sub-packages — `lifecycle` and `agentctl` — are the moved implementations.
// The `Runtime` interface here is the only surface external callers should
// reach for once migration completes; existing callers continue to use the
// sub-packages directly until later phases route them through this seam.
package runtime

import (
	"context"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/agent/runtime/lifecycle"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Runtime is the public surface for launching, resuming, stopping, and
// observing agent executions.
//
// Implementations must be safe for concurrent use.
type Runtime interface {
	// Launch starts a new agent execution against the supplied spec and
	// returns a reference to the execution.
	Launch(ctx context.Context, spec LaunchSpec) (ExecutionRef, error)

	// Resume sends a follow-up prompt to an existing execution.
	Resume(ctx context.Context, executionID string, prompt string) error

	// Stop terminates an execution and records the supplied reason.
	Stop(ctx context.Context, executionID string, reason string) error

	// GetExecution returns a snapshot view of an execution by ID.
	GetExecution(ctx context.Context, executionID string) (*Execution, error)

	// SubscribeEvents returns a channel of stream events for an
	// execution. The channel closes when the supplied context is cancelled
	// or the execution finishes.
	SubscribeEvents(ctx context.Context, executionID string) (<-chan Event, error)

	// SetMcpMode swaps the MCP tool mode for a running execution.
	SetMcpMode(ctx context.Context, executionID string, mode string) error
}

// LaunchSpec carries everything the runtime needs to start an agent.
//
// The minimum useful surface: profile, executor, workspace, prompt, prior
// ACP session id (for resume), MCP mode, plus a freeform metadata map.
// Richer launch parameters (worktree configuration, multi-repo specs,
// attachments, executor config) flow through `Metadata["launch_request"]`
// as a `*lifecycle.LaunchRequest` until later phases canonicalise them
// onto LaunchSpec directly.
type LaunchSpec struct {
	// AgentProfileID identifies the agent profile to run (e.g. "claude-acp-default").
	AgentProfileID string

	// ExecutorID identifies the executor backend to dispatch on
	// (e.g. "local_pc", "local_docker", "sprites").
	ExecutorID string

	// Workspace describes the workspace the agent operates in.
	Workspace WorkspaceRef

	// Prompt is the initial prompt sent on session start.
	Prompt string

	// PriorACPSession is the ACP session id to resume, if any.
	PriorACPSession string

	// McpMode selects the MCP tool surface (e.g. "config" for config-mode tools).
	McpMode string

	// Metadata carries integration-specific fields. Phase 1 supports
	// the key "launch_request" with value `*lifecycle.LaunchRequest` to
	// pass through fields not yet on LaunchSpec.
	Metadata map[string]any
}

// WorkspaceRef identifies the workspace and (optionally) the worktree
// the agent should operate in.
type WorkspaceRef struct {
	// Path is the host path to the workspace (or worktree root).
	Path string
	// RepositoryID, when non-empty, identifies the repository this workspace belongs to.
	RepositoryID string
	// IsEphemeral marks workspaces that may receive a fallback directory
	// when no repository is configured.
	IsEphemeral bool
}

// ExecutionRef is a lightweight handle to a launched execution.
type ExecutionRef struct {
	// ID is the runtime-assigned execution id.
	ID string
	// SessionID is the task session this execution belongs to (if any).
	SessionID string
	// AgentctlURL is the host:port the agentctl HTTP server is reachable on.
	AgentctlURL string
	// StartedAt is when the runtime registered the execution.
	StartedAt time.Time
}

// Execution is a snapshot view of a running execution.
type Execution struct {
	ID             string
	SessionID      string
	TaskID         string
	AgentProfileID string
	WorkspacePath  string
	AgentctlURL    string // base URL of the agentctl HTTP server (empty when not yet wired)
	Status         v1.AgentStatus
	StartedAt      time.Time
	FinishedAt     *time.Time
	ExitCode       *int
	ErrorMessage   string
	ACPSessionID   string
	Metadata       map[string]interface{}
}

// Event is the protocol-agnostic event shape produced by an agent
// process. Re-exported from the agentctl streams types for ergonomics:
// callers don't need to import a sibling package for a tiny struct.
type Event = streams.AgentEvent

// ErrNotFound is returned by GetExecution when no execution matches.
var ErrNotFound = errors.New("runtime: execution not found")

// ErrUnsupported is returned by methods that have not yet been wired up
// in the current implementation. Phase 1 leaves SubscribeEvents
// best-effort because the lifecycle manager publishes events via the
// global event bus rather than per-execution channels; callers that
// want raw streams should still use the agentctl client today.
var ErrUnsupported = errors.New("runtime: operation not supported")

// Backend is the lower-level dependency the default Runtime delegates to.
// Defining it as an interface lets tests substitute a fake without
// constructing a full lifecycle Manager.
type Backend interface {
	Launch(ctx context.Context, req *lifecycle.LaunchRequest) (*lifecycle.AgentExecution, error)
	PromptAgent(ctx context.Context, executionID string, prompt string, attachments []v1.MessageAttachment, dispatchOnly bool) (*lifecycle.PromptResult, error)
	StopAgentWithReason(ctx context.Context, executionID string, reason string, force bool) error
	GetExecution(executionID string) (*lifecycle.AgentExecution, bool)
	SetMcpMode(ctx context.Context, executionID string, mode string) error
}

// Compile-time check: the lifecycle Manager satisfies Backend.
var _ Backend = (*lifecycle.Manager)(nil)
