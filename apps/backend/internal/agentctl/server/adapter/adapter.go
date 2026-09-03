// Package adapter provides protocol adapters for agent communication.
// Only ACP is supported; non-ACP variants were removed in the ACP-first migration.
//
// Architecture:
//   - Transport layer (transport/*): Handles protocol-level communication (ACP is the only transport)
//   - Factory: Creates the ACP transport adapter
//
// Agent configuration (commands, discovery, models) is handled by the agent/registry package
// using agents.json as the source of truth. This package only handles protocol communication.
package adapter

import (
	"context"
	"io"

	"github.com/kandev/kandev/internal/agentctl/server/adapter/transport/shared"
	"github.com/kandev/kandev/internal/agentctl/types"
	"github.com/kandev/kandev/internal/agentctl/types/streams"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// Re-export permission types from the shared types package for convenience.
// This allows users of the adapter package to access these types without
// importing the types package directly.
type (
	PermissionRequest  = types.PermissionRequest
	PermissionResponse = types.PermissionResponse
	PermissionOption   = streams.PermissionOption
	PermissionHandler  = types.PermissionHandler
)

// Re-export stream types for convenience.
type (
	AgentEvent = streams.AgentEvent
	PlanEntry  = streams.PlanEntry
)

// Re-export agent event type constants from streams package.
const (
	EventTypeMessageChunk        = streams.EventTypeMessageChunk
	EventTypeReasoning           = streams.EventTypeReasoning
	EventTypeToolCall            = streams.EventTypeToolCall
	EventTypeToolUpdate          = streams.EventTypeToolUpdate
	EventTypePlan                = streams.EventTypePlan
	EventTypeComplete            = streams.EventTypeComplete
	EventTypeError               = streams.EventTypeError
	EventTypePermissionRequest   = streams.EventTypePermissionRequest
	EventTypePermissionCancelled = streams.EventTypePermissionCancelled
	EventTypeContextWindow       = streams.EventTypeContextWindow
	EventTypeSessionStatus       = streams.EventTypeSessionStatus
	EventTypeRateLimit           = streams.EventTypeRateLimit
)

// OneShotAdapter is an optional interface implemented by adapters that spawn
// a new process per prompt. When an adapter is one-shot, the process manager
// skips subprocess creation and the adapter manages its own subprocess
// lifecycle internally. No production adapter implements this today; Amp is
// now an ACP agent and goes through the standard subprocess path.
type OneShotAdapter interface {
	IsOneShot() bool
}

// OneShotConfig holds command configuration for one-shot adapters that manage
// their own subprocess lifecycle. The adapter spawns a new process per prompt.
type OneShotConfig struct {
	// InitialArgs is the command for the first prompt in a new session.
	InitialArgs []string
	// ContinueArgs is the command for follow-up prompts.
	// The thread/session ID is appended at runtime.
	ContinueArgs []string
	// Env is the environment variables for the subprocess.
	Env []string
	// WorkDir is the working directory for the subprocess.
	WorkDir string
}

// StderrProvider provides access to recent stderr output for error context.
// This is used by adapters to include stderr in error events when the agent
// reports an error without a detailed message (e.g., rate limit errors).
type StderrProvider interface {
	// GetRecentStderr returns the most recent stderr lines from the agent process.
	GetRecentStderr() []string
}

// StderrLineConsumer receives cleaned stderr lines as they are emitted by the
// managed agent process. Implementations must return promptly; a consumer is
// an optional diagnostic signal and must never apply backpressure to the child
// process's stderr pipe.
type StderrLineConsumer interface {
	ConsumeStderrLine(line string)
}

// StderrLineSanitizer projects stderr lines before they are logged, buffered,
// or included in process-exit events. Returning keep=false excludes a line
// from those generic diagnostics while allowing a protocol-specific consumer
// to inspect the original line in memory.
type StderrLineSanitizer interface {
	SanitizeStderrLine(line string) (safeLine string, keep bool)
}

// SessionCapabilityProvider exposes capabilities negotiated by ACP initialize.
// It is optional so adapters and focused tests that do not expose capability
// details retain the existing AgentAdapter contract.
type SessionCapabilityProvider interface {
	SupportsSessionResume() bool
	SupportsSessionLoad() bool
}

// StderrProviderSetter is an optional interface implemented by adapters that can use
// stderr output for error context. The process manager checks for this interface
// and calls SetStderrProvider if available.
type StderrProviderSetter interface {
	SetStderrProvider(provider StderrProvider)
}

// ModeSettableAdapter is an optional interface implemented by adapters that
// support changing the session mode (e.g., ACP adapters with session/set_mode).
type ModeSettableAdapter interface {
	SetMode(ctx context.Context, modeID string) error
}

// ModelSettableAdapter is an optional interface implemented by adapters that
// support changing the session model (e.g., ACP adapters with session/set_model).
type ModelSettableAdapter interface {
	SetModel(ctx context.Context, modelID string) error
}

// SessionModelStateProvider exposes the model catalog returned by the most
// recent session creation or load. The agent stream still emits the same
// session_models event, but this synchronous snapshot prevents lifecycle
// decisions from racing that event's downstream dispatch.
type SessionModelStateProvider interface {
	GetSessionModelState() *streams.SessionModelState
}

// AuthenticatableAdapter is an optional interface implemented by adapters that
// support ACP authentication (session/authenticate).
type AuthenticatableAdapter interface {
	Authenticate(ctx context.Context, methodID string) error
}

// ConfigOptionSettableAdapter is an optional interface implemented by adapters
// that support setting an arbitrary session config option (ACP
// session/set_config_option). Useful for agent-specific runtime knobs that
// aren't covered by mode/model.
type ConfigOptionSettableAdapter interface {
	SetConfigOption(ctx context.Context, key, value string) error
}

// SessionResettableAdapter is an optional interface implemented by adapters that
// can reset context by creating a new session on the same connection, without
// restarting the agent subprocess. Only ACP adapters support this.
type SessionResettableAdapter interface {
	ResetSession(ctx context.Context, mcpServers []types.McpServer) (string, error)
}

// AgentInfo contains information about the connected agent.
type AgentInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// AgentAdapter defines the interface for protocol adapters.
// Each adapter translates ACP, the only supported protocol, into the
// normalized AgentEvent format that agentctl exposes via its HTTP API.
//
// Lifecycle:
//  1. Create adapter with New*Adapter(cfg, logger)
//  2. Call PrepareEnvironment() before starting the agent process
//  3. Call PrepareCommandArgs() to get extra command-line args
//  4. Start the agent process and obtain stdin/stdout pipes
//  5. Call Connect(stdin, stdout) to wire up communication
//  6. Call Initialize() to establish protocol handshake
//  7. Use NewSession/LoadSession/Prompt/Cancel for agent interactions
//  8. Call Close() when done
type AgentAdapter interface {
	// PrepareEnvironment performs protocol-specific setup before the agent process starts.
	// The ACP adapter is a no-op (MCP servers are passed through the protocol).
	// Must be called before the agent subprocess is started.
	// Returns a map of environment variables to add to the subprocess environment.
	PrepareEnvironment() (map[string]string, error)

	// PrepareCommandArgs returns extra command-line arguments for the agent process.
	// The ACP adapter returns nil (no extra args needed).
	// Must be called after PrepareEnvironment and before starting the subprocess.
	PrepareCommandArgs() []string

	// Connect wires up the stdin/stdout pipes from the running agent subprocess.
	// Must be called after the subprocess is started and before Initialize.
	Connect(stdin io.Writer, stdout io.Reader) error

	// Initialize establishes the connection with the agent and exchanges capabilities.
	// Sends the ACP initialize request over the subprocess's stdin/stdout.
	Initialize(ctx context.Context) error

	// GetAgentInfo returns information about the connected agent.
	// Returns nil if Initialize has not been called yet.
	GetAgentInfo() *AgentInfo

	// NewSession creates a new agent session and returns the session ID.
	NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error)

	// LoadSession resumes an existing session by ID.
	// mcpServers contains the MCP servers to configure for the resumed session.
	// Agents that receive MCP configs via the protocol (e.g. ACP with AssumeMcpSse)
	// need these to reconnect to MCP servers on a new agentctl instance.
	LoadSession(ctx context.Context, sessionID string, mcpServers []types.McpServer) error

	// Prompt sends a prompt to the agent.
	// The agent's responses are streamed via the Updates channel.
	// Attachments (images) are passed to the agent if provided.
	Prompt(ctx context.Context, message string, attachments []v1.MessageAttachment, promptGeneration uint64) error

	// Cancel cancels the current operation.
	Cancel(ctx context.Context) error

	// Updates returns a channel that receives agent events.
	// The channel is closed when the adapter is closed.
	Updates() <-chan AgentEvent

	// GetSessionID returns the current session ID.
	GetSessionID() string

	// GetOperationID returns the current operation/turn ID.
	// Returns empty string if no operation is in progress, or if the connected
	// ACP agent does not report a turn ID.
	GetOperationID() string

	// SetPermissionHandler sets the handler for permission requests.
	SetPermissionHandler(handler PermissionHandler)

	// Close releases resources held by the adapter.
	Close() error

	// RequiresProcessKill returns true if the adapter's subprocess needs to be
	// explicitly killed during shutdown. Most ACP agents return false because
	// closing stdin causes the subprocess to exit. Some agents (e.g. opencode
	// acp) keep an internal HTTP server and MCP child processes alive after
	// stdin closes, so their config sets this true to force an immediate
	// process-group kill instead of waiting on a graceful stdin close.
	RequiresProcessKill() bool
}

// McpServerConfig holds configuration for an MCP server.
type McpServerConfig struct {
	// Name is the human-readable name of the MCP server
	Name string `json:"name"`
	// URL is the URL for HTTP/SSE transport
	URL string `json:"url,omitempty"`
	// Type is the transport type: "stdio", "sse", "http", or "streamable_http"
	Type string `json:"type,omitempty"`
	// Command is the command for stdio transport
	Command string `json:"command,omitempty"`
	// Args are the arguments for stdio transport
	Args []string `json:"args,omitempty"`
	// Env holds environment variables for stdio transport
	Env map[string]string `json:"env,omitempty"`
	// Headers holds HTTP headers for SSE/HTTP transport
	Headers map[string]string `json:"headers,omitempty"`
	// DefinitionID, DefinitionRevision, and Origins are sanitized backend
	// provenance carried through adapter construction for attachment evidence.
	DefinitionID       string                  `json:"definition_id,omitempty"`
	DefinitionRevision int64                   `json:"definition_revision,omitempty"`
	Origins            []types.McpServerOrigin `json:"origins,omitempty"`
}

// Config holds configuration for creating adapters
type Config struct {
	// WorkDir is the working directory for the agent
	WorkDir string

	// AutoApprove automatically approves permission requests
	AutoApprove bool

	// McpServers is a list of MCP servers to configure for the agent
	McpServers []McpServerConfig

	// AgentID is the agent identifier from the registry (e.g., "auggie", "amp", "claude-code").
	// Used for logging and debug capture. Adapters should use this instead of hardcoded names.
	AgentID string

	// AgentName is the human-readable agent name (e.g., "Auggie", "AMP", "Claude Code").
	// Used for display purposes.
	AgentName string

	// BaseURL, AuthHeader, AuthValue, and Headers are unused by any adapter
	// today (no HTTP-based transport exists). Config.ToSharedConfig still
	// propagates them to shared.Config, but no transport reads them.
	BaseURL    string            // Base URL of the agent's HTTP API
	AuthHeader string            // Optional auth header name
	AuthValue  string            // Optional auth header value
	Headers    map[string]string // Additional headers

	// Protocol-specific configuration
	Extra map[string]string

	// OneShotConfig is set for one-shot adapters that manage their own subprocess.
	// When non-nil, the process manager skips subprocess creation.
	OneShotConfig *OneShotConfig

	// AssumeMcpSse overrides MCP capability filtering to assume SSE support.
	AssumeMcpSse bool

	// AssumeMcpHttp overrides MCP capability filtering to assume HTTP support.
	AssumeMcpHttp bool

	// RequiresProcessKill is read by the transport adapter's RequiresProcessKill()
	// method. Set true for agents whose subprocess does not exit on stdin close
	// (opencode acp). The process manager uses the adapter's return value to
	// decide whether to kill the entire process group on shutdown.
	RequiresProcessKill bool

	// NotificationQueueCapacity is the server-resolved ACP inbound queue size.
	NotificationQueueCapacity int
}

// ToSharedConfig converts this Config to the shared.Config used by transport adapters.
func (c *Config) ToSharedConfig() *shared.Config {
	mcpServers := make([]shared.McpServerConfig, len(c.McpServers))
	for i, srv := range c.McpServers {
		mcpServers[i] = shared.McpServerConfig{
			Name:         srv.Name,
			URL:          srv.URL,
			Type:         srv.Type,
			Command:      srv.Command,
			Args:         srv.Args,
			Env:          srv.Env,
			Headers:      srv.Headers,
			DefinitionID: srv.DefinitionID, DefinitionRevision: srv.DefinitionRevision,
			Origins: append([]types.McpServerOrigin(nil), srv.Origins...),
		}
	}
	return &shared.Config{
		WorkDir:                   c.WorkDir,
		AutoApprove:               c.AutoApprove,
		McpServers:                mcpServers,
		AgentID:                   c.AgentID,
		AgentName:                 c.AgentName,
		BaseURL:                   c.BaseURL,
		AuthHeader:                c.AuthHeader,
		AuthValue:                 c.AuthValue,
		Headers:                   c.Headers,
		Extra:                     c.Extra,
		AssumeMcpSse:              c.AssumeMcpSse,
		AssumeMcpHttp:             c.AssumeMcpHttp,
		RequiresProcessKill:       c.RequiresProcessKill,
		NotificationQueueCapacity: c.NotificationQueueCapacity,
	}
}

// SteerablePrompter is implemented by an adapter that can deliver a prompt into
// a turn that is still generating, instead of holding it until the turn ends.
//
// It is an optional interface rather than a method on AgentAdapter so transports
// that cannot steer need no change — the same shape the process manager uses for
// RequiresProcessKill. Callers must type-assert and must also check
// SupportsSteering(), which reflects the connected agent's negotiated
// advertisement and is therefore only known after initialize.
//
// Delivery is opportunistic: whether the agent folds the prompt into the running
// turn or runs it as the next turn is the agent's decision and is not advertised
// over the protocol. See docs/specs/platform/requirements/mid-turn-steering.md.
type SteerablePrompter interface {
	SupportsSteering() bool
	PromptSteer(
		ctx context.Context,
		message string,
		attachments []v1.MessageAttachment,
		promptGeneration uint64,
	) error
}
