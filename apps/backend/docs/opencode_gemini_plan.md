# Multi-Agent Support Plan: OpenCode and Gemini

## Summary

Add support for two additional AI coding agents to kandev-2:
1. **OpenCode** - REST/SSE-based protocol with HTTP server
2. **Gemini** - ACP-based protocol (reuses existing ACP adapter)

## Architecture Overview

```
Protocol Layer              Adapter Layer              Normalized Events
-------------------        -------------------        -------------------
ACP (JSON-RPC)        -->  ACPAdapter            -->
Codex (JSON-RPC)      -->  CodexAdapter          -->  AgentEvent
Claude (stream-json)  -->  ClaudeCodeAdapter     -->  (protocol-agnostic)
OpenCode (REST/SSE)   -->  OpenCodeAdapter       -->  (NEW)
Gemini (ACP)          -->  ACPAdapter (reuse)    -->  (uses existing)
```

---

## Agent 1: OpenCode

### Protocol Overview

OpenCode uses a **REST API + Server-Sent Events (SSE)** pattern:

1. **Spawn HTTP Server**: Run `npx -y opencode-ai@<version> serve --hostname 127.0.0.1 --port 0`
2. **Wait for Server URL**: Parse stdout for `opencode server listening on <url>`
3. **Health Check**: Poll `GET /global/health` until healthy
4. **Session Management**:
   - Create: `POST /session?directory=<dir>`
   - Fork (resume): `POST /session/{id}/fork?directory=<dir>`
5. **Send Prompt**: `POST /session/{id}/message?directory=<dir>`
6. **Event Stream**: `GET /event?directory=<dir>` (SSE)
7. **Permission Handling**: `POST /permission/{id}/reply?directory=<dir>`
8. **Abort**: `POST /session/{id}/abort?directory=<dir>`

### Key Features

- **Password Auth**: Server uses `OPENCODE_SERVER_PASSWORD` env var for basic auth
- **Session Persistence**: Sessions can be forked for follow-up prompts
- **Permission System**: `permission.asked` events require `reply` with `once`/`reject`
- **Token Tracking**: Events include token usage info

### Event Types from SSE Stream

| Event Type | Description |
|------------|-------------|
| `message.updated` | Message metadata updated (tokens, model info) |
| `message.part.updated` | Text/reasoning/tool content streaming |
| `permission.asked` | Tool needs approval |
| `permission.replied` | Permission response sent |
| `session.idle` | Agent finished processing |
| `session.error` | Error occurred |
| `todo.updated` | Task list updated |

### Event Mapping: OpenCode --> AgentEvent

| OpenCode Event | AgentEvent Type | Field Mapping |
|----------------|-----------------|---------------|
| `session_start` | `session_status` | SessionID |
| `message.part.updated` (text) | `message_chunk` | Text |
| `message.part.updated` (reasoning) | `reasoning` | ReasoningText |
| `message.part.updated` (tool) | `tool_call` / `tool_update` | ToolCallID, ToolName, ToolStatus |
| `permission.asked` | `permission_request` | PendingID, PermissionTitle |
| `message.updated` (tokens) | `context_window` | ContextWindowUsed |
| `session.idle` + `done` | `complete` | - |
| `session.error` | `error` | Error |

---

## Agent 2: Gemini (Google)

### Protocol Overview

Gemini uses the **ACP (Agent Communication Protocol)** - same as Auggie:

1. **Spawn CLI**: `npx -y @google/gemini-cli@<version> --experimental-acp`
2. **ACP Initialization**: JSON-RPC over stdin/stdout
3. **Session Management**: Via ACP session/load protocol
4. **Prompts**: Via ACP prompt method

### Key Insight

Gemini CLI supports `--experimental-acp` flag, making it fully compatible with our existing `ACPAdapter`. No new adapter needed - just configuration.

### Command Flags

```bash
npx -y @google/gemini-cli@0.23.0 \
  --model <model> \
  --experimental-acp \
  [--yolo]  # Auto-approve (optional)
```

---

## Implementation Phases

### Phase 1: Add Protocol Constants

**File: `pkg/agent/protocol.go`**

```go
const (
    // ... existing protocols
    // ProtocolOpenCode is the OpenCode CLI protocol (REST/SSE over HTTP).
    ProtocolOpenCode Protocol = "opencode"
)

func (p Protocol) IsValid() bool {
    switch p {
    case ProtocolACP, ProtocolREST, ProtocolMCP, ProtocolCodex,
         ProtocolClaudeCode, ProtocolOpenCode:
        return true
    }
    return false
}
```

Note: Gemini uses `ProtocolACP` (existing).

---

### Phase 2: OpenCode Protocol Types

**New File: `pkg/opencode/types.go`**

```go
package opencode

// ExecutorEvent represents events written to stdout by the adapter
type ExecutorEvent struct {
    Type string `json:"type"`
    // Variants based on type
}

type SessionStartEvent struct {
    Type      string `json:"type"` // "session_start"
    SessionID string `json:"session_id"`
}

type SDKEvent struct {
    Type  string          `json:"type"` // "sdk_event"
    Event json.RawMessage `json:"event"`
}

type TokenUsageEvent struct {
    Type               string `json:"type"` // "token_usage"
    TotalTokens        int    `json:"total_tokens"`
    ModelContextWindow int    `json:"model_context_window"`
}

type ApprovalResponseEvent struct {
    Type       string `json:"type"` // "approval_response"
    ToolCallID string `json:"tool_call_id"`
    Status     string `json:"status"` // "approved", "denied", etc.
}

type DoneEvent struct {
    Type string `json:"type"` // "done"
}

type ErrorEvent struct {
    Type    string `json:"type"` // "error"
    Message string `json:"message"`
}

// SDK Event types (from /event SSE stream)
type MessageUpdatedEvent struct {
    Type       string      `json:"type"` // "message.updated"
    Properties MessageInfo `json:"properties"`
}

type MessagePartUpdatedEvent struct {
    Type       string         `json:"type"` // "message.part.updated"
    Properties MessagePartInfo `json:"properties"`
    Delta      string         `json:"delta,omitempty"` // For streaming text
}

type MessageInfo struct {
    ID        string           `json:"id"`
    SessionID string           `json:"sessionID"`
    Role      string           `json:"role"` // "user", "assistant"
    Model     *MessageModelInfo `json:"model,omitempty"`
    Tokens    *MessageTokens   `json:"tokens,omitempty"`
}

type MessagePartInfo struct {
    Type      string                 `json:"type"` // "text", "reasoning", "tool"
    MessageID string                 `json:"messageID"`
    SessionID string                 `json:"sessionID"`
    Text      string                 `json:"text,omitempty"`
    CallID    string                 `json:"callID,omitempty"` // For tool
    Tool      string                 `json:"tool,omitempty"`
    State     *ToolStateUpdate       `json:"state,omitempty"`
}

type ToolStateUpdate struct {
    Status   string          `json:"status"` // "pending", "running", "completed", "error"
    Input    json.RawMessage `json:"input,omitempty"`
    Output   string          `json:"output,omitempty"`
    Title    string          `json:"title,omitempty"`
    Error    string          `json:"error,omitempty"`
    Metadata json.RawMessage `json:"metadata,omitempty"`
}

type PermissionAskedEvent struct {
    Type       string `json:"type"` // "permission.asked"
    Properties struct {
        ID         string                 `json:"id"`
        SessionID  string                 `json:"sessionID"`
        Permission string                 `json:"permission"`
        Tool       *PermissionToolInfo    `json:"tool,omitempty"`
        Metadata   map[string]interface{} `json:"metadata,omitempty"`
    } `json:"properties"`
}

type PermissionToolInfo struct {
    CallID string `json:"callID"`
}

type SessionIdleEvent struct {
    Type       string `json:"type"` // "session.idle"
    Properties struct {
        SessionID string `json:"sessionID"`
    } `json:"properties"`
}

type SessionErrorEvent struct {
    Type       string `json:"type"` // "session.error"
    Properties struct {
        SessionID string      `json:"sessionID"`
        Error     interface{} `json:"error"`
    } `json:"properties"`
}
```

---

### Phase 3: OpenCode HTTP Client

**New File: `pkg/opencode/client.go`**

```go
package opencode

import (
    "context"
    "encoding/base64"
    "fmt"
    "io"
    "net/http"
    "sync"
    "time"

    "github.com/kandev/kandev/internal/common/logger"
)

// Client manages HTTP communication with OpenCode server
type Client struct {
    baseURL     string
    directory   string
    password    string
    httpClient  *http.Client
    logger      *logger.Logger

    // Event handling
    eventHandler  EventHandler
    controlCh     chan ControlEvent

    // Session state
    sessionID   string
    mu          sync.RWMutex

    ctx         context.Context
    cancel      context.CancelFunc
}

type EventHandler func(event *SDKEvent)

type ControlEvent struct {
    Type    string // "idle", "auth_required", "session_error", "disconnected"
    Message string
}

func NewClient(baseURL, directory, password string, log *logger.Logger) *Client

// Core operations
func (c *Client) WaitForHealth(ctx context.Context) error
func (c *Client) CreateSession(ctx context.Context) (string, error)
func (c *Client) ForkSession(ctx context.Context, sessionID string) (string, error)
func (c *Client) SendPrompt(ctx context.Context, sessionID, prompt string, model *ModelSpec) error
func (c *Client) Abort(ctx context.Context, sessionID string) error
func (c *Client) ReplyPermission(ctx context.Context, requestID, reply string, message *string) error

// Event stream
func (c *Client) StartEventStream(ctx context.Context) error
func (c *Client) SetEventHandler(handler EventHandler)
func (c *Client) ControlChannel() <-chan ControlEvent

// Helpers
func (c *Client) buildAuthHeader() string
```

---

### Phase 4: OpenCode Adapter

**New File: `internal/agentctl/server/adapter/opencode_adapter.go`**

```go
package adapter

import (
    "context"
    "io"
    "os/exec"
    "sync"

    "github.com/kandev/kandev/internal/common/logger"
    "github.com/kandev/kandev/pkg/opencode"
)

type OpenCodeAdapter struct {
    cfg       *Config
    logger    *logger.Logger

    // Process management
    cmd       *exec.Cmd
    stdout    io.ReadCloser

    // HTTP client (created after server starts)
    client    *opencode.Client

    // Session state
    sessionID string
    password  string

    // Event channel
    updatesCh chan AgentEvent

    // Permission handling
    permissionHandler PermissionHandler

    mu     sync.RWMutex
    closed bool

    ctx    context.Context
    cancel context.CancelFunc
}

func NewOpenCodeAdapter(cfg *Config, log *logger.Logger) *OpenCodeAdapter

// AgentAdapter interface implementation
func (a *OpenCodeAdapter) PrepareEnvironment() error
func (a *OpenCodeAdapter) Connect(stdin io.Writer, stdout io.Reader) error
func (a *OpenCodeAdapter) Initialize(ctx context.Context) error
func (a *OpenCodeAdapter) Prompt(ctx context.Context, message string) error
func (a *OpenCodeAdapter) Cancel(ctx context.Context) error
func (a *OpenCodeAdapter) NewSession(ctx context.Context, mcpServers []types.McpServer) (string, error)
func (a *OpenCodeAdapter) LoadSession(ctx context.Context, sessionID string) error
func (a *OpenCodeAdapter) Updates() <-chan AgentEvent
func (a *OpenCodeAdapter) Close() error
func (a *OpenCodeAdapter) GetAgentInfo() *AgentInfo
func (a *OpenCodeAdapter) SetPermissionHandler(handler PermissionHandler)
func (a *OpenCodeAdapter) RespondToPermission(ctx context.Context, requestID string, response *PermissionResponse) error

// Internal methods
func (a *OpenCodeAdapter) waitForServerURL(stdout io.Reader) (string, error)
func (a *OpenCodeAdapter) handleSDKEvent(event *opencode.SDKEvent)
func (a *OpenCodeAdapter) parseAndEmitEvent(eventType string, data map[string]interface{})
func (a *OpenCodeAdapter) sendUpdate(event AgentEvent)
```

### Key Implementation Details

**Server Startup Flow:**
1. Spawn `npx -y opencode-ai@<version> serve --hostname 127.0.0.1 --port 0`
2. Parse stdout for `opencode server listening on <url>`
3. Create HTTP client with parsed URL
4. Call `WaitForHealth()`
5. Create session via `CreateSession()` or `ForkSession()`
6. Start SSE event stream in background goroutine

**Event Processing:**
```go
func (a *OpenCodeAdapter) handleSDKEvent(event *opencode.SDKEvent) {
    switch event.Type {
    case "message.part.updated":
        part := parseMessagePart(event)
        switch part.Type {
        case "text":
            a.sendUpdate(AgentEvent{
                Type: EventTypeMessageChunk,
                Text: part.Delta,
            })
        case "reasoning":
            a.sendUpdate(AgentEvent{
                Type:          EventTypeReasoning,
                ReasoningText: part.Delta,
            })
        case "tool":
            a.handleToolUpdate(part)
        }
    case "permission.asked":
        a.handlePermissionRequest(event)
    case "session.idle":
        a.sendUpdate(AgentEvent{
            Type: EventTypeComplete,
        })
    case "session.error":
        a.sendUpdate(AgentEvent{
            Type:  EventTypeError,
            Error: extractErrorMessage(event),
        })
    }
}
```

---

### Phase 5: Update Adapter Factory

**File: `internal/agentctl/server/adapter/factory.go`**

```go
func NewAdapter(protocol agent.Protocol, cfg *Config, log *logger.Logger) (AgentAdapter, error) {
    switch protocol {
    case agent.ProtocolACP:
        return NewACPAdapter(cfg, log), nil
    case agent.ProtocolCodex:
        return NewCodexAdapter(cfg, log), nil
    case agent.ProtocolClaudeCode:
        return NewClaudeCodeAdapter(cfg, log), nil
    case agent.ProtocolOpenCode:
        return NewOpenCodeAdapter(cfg, log), nil
    default:
        return nil, fmt.Errorf("unsupported protocol: %s", protocol)
    }
}
```

---

### Phase 6: Update Registry Protocol Parser

**File: `internal/agent/registry/registry.go`**

```go
func parseProtocol(p string) agent.Protocol {
    switch p {
    case "acp":
        return agent.ProtocolACP
    case "codex":
        return agent.ProtocolCodex
    case "claude-code":
        return agent.ProtocolClaudeCode
    case "opencode":
        return agent.ProtocolOpenCode
    default:
        return agent.ProtocolACP
    }
}
```

---

### Phase 7: Agent Configurations

**File: `internal/agent/registry/agents.json`**

Add these entries to the `agents` array:

```json
{
  "id": "opencode",
  "name": "OpenCode AI Agent",
  "display_name": "OpenCode",
  "description": "OpenCode CLI-powered autonomous coding agent using REST/SSE protocol.",
  "enabled": true,
  "image": "",
  "tag": "",
  "cmd": [
    "npx",
    "-y",
    "opencode-ai@1.1.25",
    "serve",
    "--hostname",
    "127.0.0.1",
    "--port",
    "0"
  ],
  "working_dir": "{workspace}",
  "required_env": [],
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
    "refactoring",
    "testing",
    "shell_execution"
  ],
  "protocol": "opencode",
  "protocol_config": {},
  "model_flag": "",
  "workspace_flag": "",
  "session_config": {
    "native_session_resume": false,
    "resume_flag": "",
    "can_recover": true,
    "session_dir_template": "{home}/.opencode",
    "session_dir_target": "",
    "reports_status_via_stream": true
  },
  "permission_config": {
    "permission_flag": "",
    "tools_requiring_permission": ["edit", "bash", "webfetch"]
  },
  "permission_settings": {
    "auto_approve": {
      "supported": true,
      "default": true,
      "label": "Auto-approve",
      "description": "Automatically approve tool calls",
      "apply_method": "env",
      "cli_flag": ""
    }
  },
  "discovery": {
    "supports_mcp": true,
    "mcp_config_path": {
      "linux": ["~/.config/opencode/opencode.json", "~/.config/opencode/opencode.jsonc"],
      "windows": [],
      "macos": ["~/Library/Application Support/opencode/opencode.json"]
    },
    "installation_path": {
      "linux": ["~/.opencode", "~/.config/opencode"],
      "windows": [],
      "macos": ["~/.opencode", "~/Library/Application Support/ai.opencode.desktop"]
    },
    "discovery_capabilities": {
      "supports_session_resume": true,
      "supports_shell": false,
      "supports_workspace_only": false
    }
  },
  "model_config": {
    "default_model": "anthropic/claude-sonnet-4-20250514",
    "available_models": [
      {
        "id": "anthropic/claude-sonnet-4-20250514",
        "name": "Claude Sonnet 4",
        "description": "Anthropic Claude Sonnet 4",
        "provider": "anthropic",
        "context_window": 200000,
        "is_default": true
      },
      {
        "id": "anthropic/claude-opus-4-20250514",
        "name": "Claude Opus 4",
        "description": "Anthropic Claude Opus 4",
        "provider": "anthropic",
        "context_window": 200000,
        "is_default": false
      },
      {
        "id": "openai/gpt-5.2",
        "name": "GPT-5.2",
        "description": "OpenAI GPT-5.2",
        "provider": "openai",
        "context_window": 200000,
        "is_default": false
      },
      {
        "id": "google/gemini-2.5-pro",
        "name": "Gemini 2.5 Pro",
        "description": "Google Gemini 2.5 Pro",
        "provider": "google",
        "context_window": 2000000,
        "is_default": false
      }
    ]
  }
},
{
  "id": "gemini",
  "name": "Google Gemini CLI Agent",
  "display_name": "Gemini",
  "description": "Google Gemini CLI-powered autonomous coding agent using ACP protocol.",
  "enabled": true,
  "image": "",
  "tag": "",
  "cmd": [
    "npx",
    "-y",
    "@google/gemini-cli@0.23.0",
    "--experimental-acp"
  ],
  "working_dir": "{workspace}",
  "required_env": [],
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
    "refactoring",
    "testing",
    "shell_execution"
  ],
  "protocol": "acp",
  "protocol_config": {},
  "model_flag": "--model {model}",
  "workspace_flag": "",
  "session_config": {
    "native_session_resume": true,
    "resume_flag": "",
    "can_recover": true,
    "session_dir_template": "{home}/.gemini",
    "session_dir_target": "",
    "reports_status_via_stream": false
  },
  "permission_config": {
    "permission_flag": "",
    "tools_requiring_permission": ["run_shell_command", "write_file", "edit_file"]
  },
  "permission_settings": {
    "auto_approve": {
      "supported": true,
      "default": false,
      "label": "Auto-approve (YOLO mode)",
      "description": "Automatically approve all tool calls",
      "apply_method": "cli_flag",
      "cli_flag": "--yolo --allowed-tools run_shell_command"
    }
  },
  "discovery": {
    "supports_mcp": true,
    "mcp_config_path": {
      "linux": ["~/.gemini/settings.json"],
      "windows": [],
      "macos": ["~/.gemini/settings.json"]
    },
    "installation_path": {
      "linux": ["~/.gemini/oauth_creds.json", "~/.gemini/installation_id"],
      "windows": [],
      "macos": ["~/.gemini/oauth_creds.json", "~/.gemini/installation_id"]
    },
    "discovery_capabilities": {
      "supports_session_resume": true,
      "supports_shell": false,
      "supports_workspace_only": false
    }
  },
  "model_config": {
    "default_model": "gemini-2.5-flash",
    "available_models": [
      {
        "id": "gemini-2.5-flash",
        "name": "Gemini 2.5 Flash",
        "description": "Fast and efficient model",
        "provider": "google",
        "context_window": 1000000,
        "is_default": true
      },
      {
        "id": "gemini-2.5-pro",
        "name": "Gemini 2.5 Pro",
        "description": "Most capable model with 2M context",
        "provider": "google",
        "context_window": 2000000,
        "is_default": false
      }
    ]
  }
}
```

---

## Files to Create

| File | Description |
|------|-------------|
| `pkg/opencode/types.go` | OpenCode protocol type definitions |
| `pkg/opencode/client.go` | HTTP/SSE client for OpenCode server |
| `pkg/opencode/client_test.go` | Unit tests for client |
| `internal/agentctl/server/adapter/opencode_adapter.go` | Adapter implementation |
| `internal/agentctl/server/adapter/opencode_adapter_test.go` | Adapter tests |

## Files to Modify

| File | Change |
|------|--------|
| `pkg/agent/protocol.go` | Add `ProtocolOpenCode` constant |
| `internal/agent/registry/registry.go` | Add `"opencode"` case to `parseProtocol()` |
| `internal/agent/registry/agents.json` | Add opencode and gemini agent configs |
| `internal/agentctl/server/adapter/factory.go` | Add OpenCode case to factory |

---

## Key Differences Between Agents

| Aspect | OpenCode | Gemini |
|--------|----------|--------|
| Protocol | REST/SSE over HTTP | ACP (JSON-RPC over stdin/stdout) |
| Server Model | Spawns HTTP server | Uses ACP directly |
| Session Resume | HTTP fork endpoint | ACP session/load |
| Permission Handling | HTTP reply endpoint | ACP callback |
| New Adapter Needed | Yes (OpenCodeAdapter) | No (reuses ACPAdapter) |
| Auth | Password-based Basic Auth | OAuth (stored in ~/.gemini) |

---

## Permission Handling

### OpenCode
```go
// In opencode_adapter.go
func (a *OpenCodeAdapter) handlePermissionRequest(event *opencode.PermissionAskedEvent) {
    requestID := event.Properties.ID
    toolCallID := event.Properties.Tool.CallID
    permission := event.Properties.Permission

    a.sendUpdate(AgentEvent{
        Type:            EventTypePermissionRequest,
        SessionID:       a.sessionID,
        PendingID:       requestID,
        ToolCallID:      toolCallID,
        PermissionTitle: formatPermissionTitle(permission, event.Properties.Metadata),
        ActionType:      permission,
        ActionDetails:   event.Properties.Metadata,
    })
}

func (a *OpenCodeAdapter) RespondToPermission(ctx context.Context, requestID string, response *PermissionResponse) error {
    reply := "reject"
    var message *string

    if response.Approved {
        reply = "once"
    } else {
        msg := response.Message
        if msg == "" {
            msg = "User denied this tool use request"
        }
        message = &msg
    }

    return a.client.ReplyPermission(ctx, requestID, reply, message)
}
```

### Gemini (uses existing ACPAdapter permission handling)

---

## Session Management

### OpenCode

```go
func (a *OpenCodeAdapter) NewSession(ctx context.Context, _ []types.McpServer) (string, error) {
    sessionID, err := a.client.CreateSession(ctx)
    if err != nil {
        return "", err
    }

    a.mu.Lock()
    a.sessionID = sessionID
    a.mu.Unlock()

    // Emit session status event for resume token storage
    a.sendUpdate(AgentEvent{
        Type:      EventTypeSessionStatus,
        SessionID: sessionID,
        Data: map[string]interface{}{
            "session_status": "active",
            "init":           true,
        },
    })

    return sessionID, nil
}

func (a *OpenCodeAdapter) LoadSession(ctx context.Context, sessionID string) error {
    // Fork existing session (creates new session with history)
    newSessionID, err := a.client.ForkSession(ctx, sessionID)
    if err != nil {
        return err
    }

    a.mu.Lock()
    a.sessionID = newSessionID
    a.mu.Unlock()

    return nil
}
```

### Gemini (uses existing ACPAdapter session handling via ACP protocol)

---

## Testing Checklist

### OpenCode
- [ ] Server starts and health check passes
- [ ] Session creation works
- [ ] Prompt sends and response streams
- [ ] Text content displays correctly
- [ ] Reasoning content displays correctly
- [ ] Tool calls display with status updates
- [ ] Permission requests trigger UI
- [ ] Permission responses (approve/deny) work
- [ ] Session resume (fork) works
- [ ] Token usage tracked
- [ ] Abort works
- [ ] Error handling works

### Gemini
- [ ] Agent appears when oauth_creds.json exists
- [ ] ACP initialization works
- [ ] Prompt sends and response streams
- [ ] Tool calls work
- [ ] Permission requests work (without --yolo)
- [ ] --yolo mode auto-approves
- [ ] Session resume works via ACP
- [ ] Model selection works (--model flag)

---

## Implementation Priority

1. **Gemini First** - Simpler, reuses ACPAdapter, just needs config
2. **OpenCode Second** - Requires new adapter with HTTP/SSE handling

### Gemini Implementation Steps
1. Add agent config to agents.json
2. Test with existing ACPAdapter
3. Verify model flag and auto-approve work

### OpenCode Implementation Steps
1. Add protocol constant
2. Create types package
3. Create HTTP/SSE client
4. Create adapter
5. Add to factory
6. Add agent config
7. Test end-to-end
