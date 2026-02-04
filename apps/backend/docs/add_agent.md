# Adding New Agents to Kandev

This guide explains how to add support for new AI coding agents (like Gemini, OpenCode, Amp, etc.) to the Kandev backend.

## Architecture Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Agent Architecture                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  agents.json          Registry              Adapter Factory         │
│  ┌──────────┐        ┌──────────┐          ┌──────────────┐        │
│  │ Agent    │───────▶│ Registry │─────────▶│ NewAdapter() │        │
│  │ Config   │        │          │          │              │        │
│  └──────────┘        └──────────┘          └──────┬───────┘        │
│                                                    │                │
│                                                    ▼                │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │                    Protocol Adapters                         │   │
│  ├─────────────┬─────────────┬─────────────┬─────────────┐     │   │
│  │ ACP Adapter │Codex Adapter│Claude Adapter│ New Adapter │     │   │
│  │ (JSON-RPC)  │ (JSON-RPC)  │(stream-json) │  (???)      │     │   │
│  └─────────────┴─────────────┴─────────────┴─────────────┘     │   │
│                           │                                     │   │
│                           ▼                                     │   │
│                    AgentEvent (normalized)                      │   │
│                           │                                     │   │
│                           ▼                                     │   │
│                    Lifecycle Manager                            │   │
│                                                                 │   │
└─────────────────────────────────────────────────────────────────────┘
```

All agents communicate through a **protocol adapter** that normalizes their output into `AgentEvent` structs. This allows the rest of the system to work uniformly regardless of the underlying agent.

## Files to Modify/Create

### 1. Protocol Definition
**File:** `pkg/agent/protocol.go`

Add a new protocol constant:

```go
const (
    ProtocolACP       Protocol = "acp"
    ProtocolCodex     Protocol = "codex"
    ProtocolClaudeCode Protocol = "claude-code"
    ProtocolGemini    Protocol = "gemini"      // New
    ProtocolOpenCode  Protocol = "opencode"    // New
)

func (p Protocol) IsValid() bool {
    switch p {
    case ProtocolACP, ProtocolREST, ProtocolMCP, ProtocolCodex,
         ProtocolClaudeCode, ProtocolGemini, ProtocolOpenCode:
        return true
    }
    return false
}
```

### 2. Protocol Types (if needed)
**File:** `pkg/<agentname>/types.go`

If the agent uses a custom protocol, create a types package:

```go
package gemini

// Message types specific to this agent's protocol
type CLIMessage struct {
    Type    string `json:"type"`
    // ... agent-specific fields
}

// Request/response types for control flow
type ControlRequest struct {
    // ...
}
```

### 3. Protocol Client (if needed)
**File:** `pkg/<agentname>/client.go`

For agents with custom protocols over stdin/stdout:

```go
package gemini

type Client struct {
    stdin  io.Writer
    stdout io.Reader
    // ...
}

func NewClient(stdin io.Writer, stdout io.Reader) *Client
func (c *Client) Start(ctx context.Context)
func (c *Client) SendMessage(msg *Message) error
func (c *Client) SetMessageHandler(handler MessageHandler)
```

### 4. Protocol Adapter
**File:** `internal/agentctl/server/adapter/<agentname>_adapter.go`

Implement the `AgentAdapter` interface:

```go
package adapter

type GeminiAdapter struct {
    cfg       *Config
    logger    *logger.Logger
    stdin     io.Writer
    stdout    io.Reader
    client    *gemini.Client
    updatesCh chan AgentEvent
    // ...
}

// Required interface methods:
func (a *GeminiAdapter) PrepareEnvironment() error
func (a *GeminiAdapter) Connect(stdin io.Writer, stdout io.Reader) error
func (a *GeminiAdapter) Initialize(ctx context.Context) error
func (a *GeminiAdapter) Prompt(ctx context.Context, message string) error
func (a *GeminiAdapter) Cancel(ctx context.Context) error
func (a *GeminiAdapter) NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error)
func (a *GeminiAdapter) LoadSession(ctx context.Context, sessionID string) error
func (a *GeminiAdapter) Updates() <-chan AgentEvent
func (a *GeminiAdapter) Close() error
func (a *GeminiAdapter) GetAgentInfo() *AgentInfo
func (a *GeminiAdapter) SetPermissionHandler(handler PermissionHandler)
func (a *GeminiAdapter) RespondToPermission(ctx context.Context, requestID string, response *PermissionResponse) error
```

### 5. Adapter Factory
**File:** `internal/agentctl/server/adapter/factory.go`

Register the new adapter:

```go
func NewAdapter(protocol agent.Protocol, cfg *Config, log *logger.Logger) (AgentAdapter, error) {
    switch protocol {
    case agent.ProtocolACP:
        return NewACPAdapter(cfg, log), nil
    case agent.ProtocolCodex:
        return NewCodexAdapter(cfg, log), nil
    case agent.ProtocolClaudeCode:
        return NewClaudeCodeAdapter(cfg, log), nil
    case agent.ProtocolGemini:
        return NewGeminiAdapter(cfg, log), nil  // New
    default:
        return nil, fmt.Errorf("unsupported protocol: %s", protocol)
    }
}
```

### 6. Registry Protocol Parser
**File:** `internal/agent/registry/registry.go`

Add protocol string parsing:

```go
func parseProtocol(p string) agent.Protocol {
    switch p {
    case "acp":
        return agent.ProtocolACP
    case "codex":
        return agent.ProtocolCodex
    case "claude-code":
        return agent.ProtocolClaudeCode
    case "gemini":
        return agent.ProtocolGemini  // New
    default:
        return agent.ProtocolACP
    }
}
```

### 7. Agent Configuration
**File:** `internal/agent/registry/agents.json`

Add the agent definition:

```json
{
  "id": "gemini",
  "name": "Google Gemini Agent",
  "display_name": "Gemini",
  "description": "Google Gemini CLI-powered coding agent.",
  "enabled": true,
  "image": "",
  "tag": "",
  "cmd": [
    "gemini-cli",
    "--output-format=json",
    "--non-interactive"
  ],
  "working_dir": "{workspace}",
  "required_env": ["GOOGLE_API_KEY"],
  "env": {},
  "mounts": [],
  "resource_limits": {
    "memory_mb": 4096,
    "cpu_cores": 2.0,
    "timeout_seconds": 3600
  },
  "capabilities": [
    "code_generation",
    "code_review",
    "shell_execution"
  ],
  "protocol": "gemini",
  "protocol_config": {},
  "model_flag": "--model {model}",
  "workspace_flag": "--workspace",
  "session_config": {
    "native_session_resume": false,
    "resume_flag": "--resume",
    "can_recover": true,
    "session_dir_template": "{home}/.gemini/sessions",
    "session_dir_target": ""
  },
  "permission_config": {
    "permission_flag": "",
    "tools_requiring_permission": ["shell", "write_file", "edit_file"]
  },
  "permission_settings": {
    "auto_approve": {
      "supported": true,
      "default": false,
      "label": "Auto-approve",
      "description": "Automatically approve tool calls",
      "apply_method": "cli_flag",
      "cli_flag": "--auto-approve"
    }
  },
  "discovery": {
    "supports_mcp": false,
    "mcp_config_path": { "linux": [], "windows": [], "macos": [] },
    "installation_path": {
      "linux": ["~/.gemini/config.json"],
      "windows": ["~/.gemini/config.json"],
      "macos": ["~/.gemini/config.json"]
    },
    "discovery_capabilities": {
      "supports_session_resume": true,
      "supports_shell": false,
      "supports_workspace_only": false
    }
  },
  "model_config": {
    "default_model": "gemini-2.0-flash",
    "available_models": [
      {
        "id": "gemini-2.0-flash",
        "name": "Gemini 2.0 Flash",
        "description": "Fast and efficient model",
        "provider": "google",
        "context_window": 1000000,
        "is_default": true
      },
      {
        "id": "gemini-2.0-pro",
        "name": "Gemini 2.0 Pro",
        "description": "Most capable model",
        "provider": "google",
        "context_window": 2000000,
        "is_default": false
      }
    ]
  }
}
```

## Key Considerations

### Execution Modes

Agents can run in two modes:

1. **Docker-based**: Set `image` and `tag` fields. The agent runs in a container.
2. **Standalone**: Set `cmd` array, leave `image` empty. The agent runs directly on the host.

The validation in `registry.go` requires either an `image` OR a `cmd`:
```go
if config.Image == "" && len(config.Cmd) == 0 {
    return fmt.Errorf("agent type requires either image (Docker) or cmd (standalone)")
}
```

### Event Types

All adapters must emit normalized `AgentEvent` structs. Key event types:

| Event Type | Purpose |
|------------|---------|
| `message_chunk` | Streaming text from the agent |
| `reasoning` | Chain-of-thought/thinking content |
| `tool_call` | Agent is invoking a tool |
| `tool_update` | Tool execution result |
| `complete` | Agent turn finished (triggers state change to REVIEW) |
| `error` | Error occurred |
| `permission_request` | Agent needs permission for an action |
| `session_status` | Session state update (NOT turn completion) |
| `context_window` | Token usage information |

**Important:** Only emit `complete` when the agent truly finishes a turn. Don't emit it for initialization messages.

### Tool Call Events

When emitting tool calls, set both `ToolName` and `ToolTitle`:

```go
a.sendUpdate(AgentEvent{
    Type:        EventTypeToolCall,
    SessionID:   sessionID,
    OperationID: operationID,
    ToolCallID:  toolID,
    ToolName:    "Bash",           // Machine-readable name
    ToolTitle:   "ls -la /tmp",    // Human-readable title for UI
    ToolArgs:    args,
    ToolStatus:  "running",
})
```

### Permission Handling

If the agent requires permission for certain actions:

1. Set `tools_requiring_permission` in the agent config
2. Implement `SetPermissionHandler` and `RespondToPermission` in the adapter
3. Emit `permission_request` events when the agent needs approval:

```go
a.sendUpdate(AgentEvent{
    Type:              EventTypePermissionRequest,
    SessionID:         sessionID,
    PendingID:         requestID,
    ToolCallID:        toolID,
    PermissionTitle:   "Execute: rm -rf /tmp/test",
    PermissionOptions: []PermissionOption{
        {ID: "allow", Label: "Allow"},
        {ID: "deny", Label: "Deny"},
    },
    ActionType:    "shell",
    ActionDetails: map[string]any{"command": "rm -rf /tmp/test"},
})
```

### Session Management

Configure session handling in `session_config`:

- `native_session_resume`: If true, sessions resume via the adapter's native session loading (works across protocols). If false, use CLI flag or context injection.
- `resume_flag`: CLI flag for resuming (e.g., `--resume`)
- `can_recover`: Whether sessions can survive backend restarts
- `session_dir_template`: Where session data is stored on host
- `session_dir_target`: Mount path inside container (if Docker-based)

**Session ID Tracking for Stream-Based Protocols:**

For agents that report their session ID via the stream (like Claude Code), you must:

1. Capture the session ID from the protocol message (e.g., `system` message in Claude Code)
2. Emit a `session_status` event with the session ID in `SessionID` field
3. The orchestrator will automatically store this as the resume token

Example from Claude Code adapter:
```go
func (a *ClaudeCodeAdapter) handleSystemMessage(msg *claudecode.CLIMessage) {
    // Update session ID if provided
    if msg.SessionID != "" {
        a.mu.Lock()
        a.sessionID = msg.SessionID
        a.mu.Unlock()
    }

    // Send session status event - this triggers resume token storage
    a.sendUpdate(AgentEvent{
        Type:      EventTypeSessionStatus,
        SessionID: msg.SessionID,  // Agent's native session ID
        Data: map[string]any{
            "session_status": msg.SessionStatus,
            "init":           true,
        },
    })
}
```

The `SessionID` field in the event is mapped to `ACPSessionID` in the event payload, which the orchestrator uses to store the resume token.

### Model Configuration

The `model_flag` field defines how to pass the model to the CLI:

```json
"model_flag": "--model {model}"       // Results in: --model gemini-2.0-flash
"model_flag": "-c model=\"{model}\""  // Results in: -c model="gemini-2.0-flash"
```

The `{model}` placeholder is replaced with the selected model ID.

### Discovery

The discovery system detects installed agents by checking for files:

```json
"installation_path": {
  "linux": ["~/.gemini/config.json"],
  "windows": ["~/.gemini/config.json"],
  "macos": ["~/.gemini/config.json"]
}
```

When a file exists at any of these paths, the agent is considered "available" and a default profile is auto-created.

### Context Window Tracking

Track token usage and emit context window events:

```go
a.sendUpdate(AgentEvent{
    Type:                   EventTypeContextWindow,
    SessionID:              sessionID,
    ContextWindowSize:      200000,
    ContextWindowUsed:      15000,
    ContextWindowRemaining: 185000,
    ContextEfficiency:      7.5, // percentage
})
```

## Testing Checklist

Before releasing a new agent integration:

- [ ] Agent appears in settings when installed (discovery works)
- [ ] Default profile created with correct model
- [ ] Agent starts successfully with a simple prompt
- [ ] Streaming text appears in real-time
- [ ] Tool calls display with proper titles
- [ ] Tool results are shown correctly
- [ ] Permission requests work (if applicable)
- [ ] Session stays IN_PROGRESS while agent is working
- [ ] Session moves to REVIEW when agent finishes
- [ ] Follow-up prompts work in same session
- [ ] Session resume works after stopping agent
- [ ] Context window tracking shows accurate usage
- [ ] Errors are properly displayed to user
- [ ] Model switching works

## Example: Existing Adapters

### ACP Adapter (`acp_adapter.go`)
- Used by: Auggie
- Protocol: JSON-RPC over stdin/stdout
- Features: Full ACP protocol support, MCP server integration

### Codex Adapter (`codex_adapter.go`)
- Used by: OpenAI Codex
- Protocol: Custom JSON-RPC (codex app-server)
- Features: Auto-approval via ACP, session resume

### Claude Code Adapter (`claude_code_adapter.go`)
- Used by: Claude Code CLI
- Protocol: stream-json over stdin/stdout
- Features: Permission prompts via stdio, thinking/reasoning support

## Troubleshooting

### Agent not appearing in settings
- Check `installation_path` paths exist on system
- Verify `enabled: true` in agents.json
- Check backend logs for discovery errors

### Protocol not recognized (falls back to ACP)
- Add protocol to `parseProtocol()` in `registry.go`
- Add to `Protocol.IsValid()` in `pkg/agent/protocol.go`

### Session moves to REVIEW too quickly
- Don't emit `EventTypeComplete` for initialization messages
- Only emit `complete` when agent truly finishes turn

### Tool titles not showing
- Set both `ToolName` and `ToolTitle` in tool_call events
- `ToolTitle` is the human-readable display text

### Model flag not working
- Ensure `model_flag` includes `{model}` placeholder
- Check profile has model set (not empty)

### Session resume not working
- For stream-based protocols: ensure adapter emits `session_status` event with agent's native session ID
- Check that `resume_flag` is set in agent config (e.g., `"--resume"`)
- Verify `native_session_resume` is `false` for CLI-based resume
- Check `executor_running` table has the correct `resume_token` value
- For Claude Code: the session ID from `system` message should be stored as resume token
