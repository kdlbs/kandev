# Adding a New Agent CLI Integration

This guide explains how to add support for a new coding agent CLI to Kandev.

Agents are defined entirely in Go. The backend discovers them through a registry, and the frontend picks them up automatically via the API - **no frontend changes are needed**.

For a clean reference implementation, see [`gemini.go`](../apps/backend/internal/agent/agents/gemini.go) (ACP protocol). For a CLI-only TUI agent, see the `TUIAgent` declarative type in [`tui_agent.go`](../apps/backend/internal/agent/agents/tui_agent.go).

---

## CLI-Only Agent (Passthrough Only) - Recommended for TUI tools

If an agent has a CLI but no structured protocol (no JSON-RPC, no streaming API), use the **`TUIAgent` declarative type**. The agent runs in a terminal session and input/output passes through a WebSocket - users interact with the full agent TUI directly.

This is the simplest path. A complete agent definition is ~20 lines.

### Step 1: Create the agent file

Create `apps/backend/internal/agent/agents/{agent_id}.go`:

```go
package agents

func NewClaude() *TUIAgent {
    return NewTUIAgent(TUIAgentConfig{
        AgentID:   "claude",
        AgentName: "Claude",
        Command:   "claude",
        Desc:      "Claude Code - an agentic coding tool by Anthropic.",

        // Optional: let users select a model in profile settings.
        // The {model} placeholder is replaced with the user's chosen model at launch.
        // Omit this if the CLI doesn't accept a model flag.
        ModelFlag: NewParam("--model", "{model}"),
    })
}
```

Logos are optional - the UI shows a blank placeholder when none is provided. To add them, embed SVGs and pass `LogoLight` / `LogoDark`:

```go
package agents

import _ "embed"

//go:embed logos/claude_light.svg
var claudeLogoLight []byte

//go:embed logos/claude_dark.svg
var claudeLogoDark []byte

func NewClaude() *TUIAgent {
    return NewTUIAgent(TUIAgentConfig{
        AgentID:   "claude",
        AgentName: "Claude",
        Command:   "claude",
        Desc:      "Claude Code - an agentic coding tool by Anthropic.",
        ModelFlag: NewParam("--model", "{model}"),
        LogoLight: claudeLogoLight,
        LogoDark:  claudeLogoDark,
    })
}
```

`TUIAgentConfig` fields (all optional fields have sensible defaults):

| Field | Required | Description |
|-------|----------|-------------|
| `AgentID` | Yes | Unique lowercase identifier (e.g. `"claude"`) |
| `AgentName` | Yes | Display name (e.g. `"Claude"`) |
| `Command` | Yes | Binary name to run (e.g. `"claude"`) |
| `Desc` | Yes | Description shown in the UI |
| `ModelFlag` | No | Model CLI flag, e.g. `NewParam("--model", "{model}")`. Lets users pick a model in profile settings. Omit if the CLI doesn't support model selection. |
| `CommandArgs` | No | Extra args appended after `Command` (e.g. `[]string{"--debug"}`) |
| `WaitForTerm` | No | When `true`, delays process start until the terminal WebSocket connects and sends its first resize event. Use this for TUI apps that need accurate terminal dimensions on startup (e.g. ncurses-based UIs). Most agents don't need this. Defaults to `false`. |
| `LogoLight` / `LogoDark` | No | Embedded SVG bytes for light/dark themes. The UI shows a blank placeholder if omitted. |
| `DetectOpts` | No | Custom detection options. Defaults to `WithCommand(Command)` which checks if the binary is in `$PATH`. |

### Step 2: Register the agent

Add your agent to `LoadDefaults()` in [`apps/backend/internal/agent/registry/registry.go`](../apps/backend/internal/agent/registry/registry.go):

```go
func (r *Registry) LoadDefaults() {
    all := []agents.Agent{
        // ... existing agents ...
        agents.NewMyAgent(), // add here
    }
    // ...
}
```

### Step 3: Build and test

```bash
make -C apps/backend build && make -C apps/backend test && make -C apps/backend lint
```

Note: passthrough-only agents provide terminal interaction only. There are no structured chat messages, tool call visibility, or progress tracking in the chat UI.

---

## Adding a New Agent (Full Integration)

For agents with a structured protocol (JSON-RPC, streaming JSON, etc.), implement the full [`Agent`](../apps/backend/internal/agent/agents/agent.go) interface.

### Step 1: Create the agent definition file

Create `apps/backend/internal/agent/agents/{agent_id}.go`.

This file implements the [`Agent`](../apps/backend/internal/agent/agents/agent.go) interface. See the interface definition in `agent.go` for the full contract.

Embed `StandardPassthrough` to get CLI passthrough support for free:

```go
type MyAgent struct {
    StandardPassthrough
}

func NewMyAgent() *MyAgent {
    return &MyAgent{
        StandardPassthrough: StandardPassthrough{
            PermSettings: myAgentPermSettings,
            Cfg: PassthroughConfig{
                Supported:      true,
                Label:          "CLI Passthrough",
                Description:    "Show terminal directly instead of chat interface",
                PassthroughCmd: NewCommand("myagent"),
                ModelFlag:      NewParam("--model", "{model}"),
                PromptFlag:     NewParam("--prompt", "{prompt}"),
                IdleTimeout:    3 * time.Second,
                BufferMaxBytes: DefaultBufferMaxBytes,
            },
        },
    }
}
```

Key implementation notes:

- **`BuildCommand()`** returns the CLI command for structured protocol mode (the agent communicates via stdin/stdout with a protocol adapter). Include the protocol flag here (e.g. `--experimental-acp`, `--stream-json`).
- **`Runtime()`** returns configuration used when running the agent in Docker or as a standalone process. Set `Protocol` to tell the adapter factory which transport to use.
- **`IsInstalled()`** uses the `Detect()` helper with `WithFileExists()` to check for known installation paths. Set `SupportsMCP` and `MCPConfigPaths` on the result.
- **`ListModels()`** can return a static list or dynamically query the CLI. Use `execAndParse()` from `helpers.go` for dynamic model discovery.
- Use `Cmd()` builder for constructing commands fluently: `Cmd("myagent", "--flag").Model(...).Settings(...).Build()`.

### Step 2: Add logos (optional)

Logos are optional - the UI shows a blank placeholder when none is provided. To add them, place SVGs in:

```
apps/backend/internal/agent/agents/logos/{agent_id}_light.svg
apps/backend/internal/agent/agents/logos/{agent_id}_dark.svg
```

Embed them in your agent file:

```go
//go:embed logos/myagent_light.svg
var myagentLogoLight []byte

//go:embed logos/myagent_dark.svg
var myagentLogoDark []byte
```

### Step 3: Register the agent

Add your agent to `LoadDefaults()` in [`apps/backend/internal/agent/registry/registry.go`](../apps/backend/internal/agent/registry/registry.go):

```go
func (r *Registry) LoadDefaults() {
    all := []agents.Agent{
        // ... existing agents ...
        agents.NewMyAgent(), // add here
    }
    // ...
}
```

That's it for registration. The frontend discovers agents through the API automatically.

### Step 4: Choose a protocol

Check [`apps/backend/pkg/agent/protocol.go`](../apps/backend/pkg/agent/protocol.go) for available protocols:

| Protocol | Constant | Transport | Used By |
|----------|----------|-----------|---------|
| ACP | `ProtocolACP` | JSON-RPC 2.0 over stdin/stdout | Gemini, Auggie |
| stream-json | `ProtocolClaudeCode` | Streaming JSON over stdin/stdout | Claude Code |
| Codex | `ProtocolCodex` | JSON-RPC variant over stdin/stdout | Codex |
| OpenCode | `ProtocolOpenCode` | REST/SSE over HTTP | OpenCode |
| Copilot | `ProtocolCopilot` | Go SDK (JSON-RPC internally) | GitHub Copilot |
| Amp | `ProtocolAmp` | Streaming JSON over stdin/stdout | Amp |

**If an existing protocol fits your agent**, reuse it - set `Protocol` in `Runtime()`. The adapter factory in `factory.go` selects the right transport automatically based on this value.

**If you need a new protocol**, you'll need to:

1. Add a new `Protocol` constant in `apps/backend/pkg/agent/protocol.go`
2. Create a transport adapter in `apps/backend/internal/agentctl/server/adapter/transport/{protocol}/`
3. Add the case to the factory switch in [`apps/backend/internal/agentctl/server/adapter/factory.go`](../apps/backend/internal/agentctl/server/adapter/factory.go)

### Step 5: Build and test

```bash
make -C apps/backend build && make -C apps/backend test && make -C apps/backend lint
```

Start the dev server and verify:
- The agent appears in the agent selector in the UI
- Agent execution works (start a task, send a prompt)
- Passthrough mode works (toggle to CLI passthrough in the session)
- Model listing works

---

## Updating an Existing Agent

| Change | Where |
|--------|-------|
| Update CLI version | `BuildCommand()` and `Runtime().Cmd` in the agent file |
| Add/remove models | `ListModels()` - update static list or dynamic command |
| Update permissions | `PermissionSettings()` variable and `PassthroughConfig` |
| Update Docker image | `Runtime()` - change `Image`/`Tag` fields |
| Change protocol | `Runtime().Protocol` in the agent file |

---

## File Reference

| File | When to modify |
|------|---------------|
| `apps/backend/internal/agent/agents/{agent_id}.go` | Always - agent definition |
| `apps/backend/internal/agent/agents/logos/{agent_id}_{light,dark}.svg` | Optional - agent logos (UI shows blank placeholder if omitted) |
| `apps/backend/internal/agent/registry/registry.go` | Always - register in `LoadDefaults()` |
| `apps/backend/internal/agent/agents/tui_agent.go` | Never - provides `TUIAgent` declarative type (read-only reference) |
| `apps/backend/pkg/agent/protocol.go` | Only if adding a new protocol |
| `apps/backend/internal/agentctl/server/adapter/factory.go` | Only if adding a new protocol |
| `apps/backend/internal/agentctl/server/adapter/transport/{proto}/` | Only if adding a new protocol |
| `apps/backend/internal/agent/agents/passthrough.go` | Never - provides `StandardPassthrough` (read-only reference) |
| `apps/backend/internal/agent/agents/agent.go` | Never - defines the `Agent` interface (read-only reference) |
