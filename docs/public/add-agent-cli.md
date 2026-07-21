---
title: "Add an Agent CLI"
description: "Register a local TUI agent or ship a tested built-in passthrough or ACP agent integration."
---

# Add an Agent CLI

Choose the smallest integration that matches the CLI:

| Need | Path |
|---|---|
| Use a local interactive CLI without changing Kandev source | Add a custom TUI agent in Settings |
| Ship a passthrough-only CLI as a built-in | Add a declarative `TUIAgent` and registry entry |
| Show structured chat, tool calls, modes, and resume | Implement a built-in ACP agent |

ACP, REST, and MCP are different boundaries. ACP is the only structured agent protocol accepted by the current agentctl adapter factory. REST/WebSocket control agentctl and Kandev. MCP supplies tools to an agent; it is not a runtime adapter.

## Register a local TUI agent

Open **Settings → Agents**, choose **Add TUI Agent**, and provide:

- a display name;
- an optional model/profile label;
- a command available on the executor's `PATH`.

Use `{{model}}` in the command to insert the optional model value. Kandev derives a stable slug from the display name, persists the definition, registers it at startup, and creates a default passthrough profile.

This path is terminal passthrough only. It does not provide structured chat messages, tool-call visibility, ACP capability probing, or Kandev MCP injection. The command is split into an executable and arguments on whitespace; it is not evaluated by a shell, and shell quoting/expansion is not supported.

Use a source integration when the agent must ship for every installation, needs logos/discovery/auth/permissions, needs safe structured argument building, or supports ACP.

## Ship a built-in passthrough agent

Definitions live in `apps/backend/internal/agent/agents/`. The declarative `TUIAgent` implements `agents.Agent` plus `agents.PassthroughAgent`, but Kandev classifies it as passthrough-only.

Create `apps/backend/internal/agent/agents/my_agent.go`:

```go
package agents

func NewMyAgent() *TUIAgent {
    return NewTUIAgent(TUIAgentConfig{
        AgentID:   "my-agent",
        AgentName: "My Agent",
        Command:   "my-agent",
        Desc:      "Run My Agent in terminal passthrough mode.",
        ModelFlag: NewParam("--model", "{model}"),
        WaitForTerm: true,
    })
}
```

Only set `ModelFlag` when the CLI accepts that flag. `WaitForTerm` is useful for a full-screen TUI that needs an initial terminal resize before process start.

The exact `TUIAgentConfig` contract is:

| Fields | Required | Meaning |
|---|---|---|
| `AgentID`, `AgentName`, `Command`, `Desc` | Yes | Stable lowercase ID, names, executable, and description |
| `Display`, `Order` | No | Alternate display name and list order; defaults are name and 99 |
| `LogoLight`, `LogoDark` | No | Embedded SVG bytes; the UI otherwise shows a placeholder |
| `IdleTimeout`, `BufferMax`, `WaitForTerm` | No | Passthrough buffering and terminal-start behavior |
| `ModelFlag`, `CommandArgs` | No | Typed model parameter and fixed argument tokens |
| `DetectOpts` | No | Installation detection; defaults to `WithCommand(Command)` |
| `Protocol` | No | Runtime metadata, default ACP; it does not turn `TUIAgent` into structured chat |

Do not shell-concatenate user values. `CommandArgs` is a token slice and `ModelFlag` uses the typed parameter builder. Detection should inspect known commands/files without executing untrusted content.

`TUIAgentConfig` does not expose a passthrough MCP strategy. If this CLI needs Kandev/profile MCP servers, implement a full agent with an explicit `PassthroughConfig.MCPStrategy`.

## Ship a structured ACP integration

Use [`gemini.go`](../../apps/backend/internal/agent/agents/gemini.go) as a current ACP example and [`agent.go`](../../apps/backend/internal/agent/agents/agent.go) as the contract. A full integration implements `agents.Agent`:

- identity: `ID`, `Name`, `DisplayName`, `Description`, `Enabled`, `DisplayOrder`;
- assets and discovery: `Logo`, `IsInstalled`;
- execution: `BuildCommand`, `Runtime`;
- policy: `PermissionSettings`, `BillingType`, `RemoteAuth`, `InstallScript`.

Optional interfaces add specific capabilities:

| Interface | Capability |
|---|---|
| `InferenceAgent` | One-shot inference through the host utility |
| `PassthroughAgent` | Optional direct terminal mode |
| `NativeBinaryAgent` | Prefer an installed native binary over a package launch |
| `LoginAgent` | Interactive PTY-backed authentication |

### Build the runtime command

`BuildCommand(CommandOptions)` must return a tokenized command that launches an ACP-speaking process or bridge. Use `Cmd`, `Command`, and `Param` builders for model, session, permission, prompt, and configured CLI flags. Never interpolate them into a shell string.

`Runtime()` describes working directory, environment, resource limits, required/stripped variables, mounts, session recovery, and the command used outside the host. Set `RuntimeConfig.Protocol` to `agent.ProtocolACP`. The factory in [`internal/agentctl/server/adapter/factory.go`](../../apps/backend/internal/agentctl/server/adapter/factory.go) rejects other structured protocols.

If the upstream CLI has no ACP server, add or reuse a bridge that speaks ACP, or keep the integration passthrough-only. Adding a protocol constant alone does not create an adapter.

### Detect installation and authentication

`IsInstalled` returns a `DiscoveryResult` with availability, matched path, MCP support/paths, installation paths, and resume/shell/workspace capabilities. Use `Detect` with bounded `DetectOption` checks for known commands or files.

Declare remote credential methods through `RemoteAuth`, required environment through `RuntimeConfig.RequiredEnv`, and variables that must never reach the child through `StripEnv`. `InstallScript` runs in remote environments; keep it deterministic, pinned where possible, and free of embedded secrets.

Permission settings must map to actual CLI or agentctl behavior. Test supervised, autonomous, and plan-shaped policies when supported. Do not advertise a permission toggle that only changes the UI.

### Discover models and modes

Do not maintain a static model list in the agent definition. `internal/agent/hostutility/` probes the ACP server and caches models, modes, and capabilities learned during `session/new`. Test empty, changed, and failed capability responses.

### Configure MCP

In structured mode, resolved MCP servers normally travel in ACP `session/new`. Set `RuntimeConfig.ProjectMCPStrategy` only when an adapter cannot forward those servers and the CLI requires a project file.

For optional passthrough mode, embed or implement `PassthroughAgent` and set `PassthroughConfig.MCPStrategy`. Existing strategies in [`internal/agent/mcpconfig/passthrough.go`](../../apps/backend/internal/agent/mcpconfig/passthrough.go) cover Claude, Codex, Cursor, Pi, and OpenCode configuration shapes.

An MCP strategy must:

- materialize only resolved, scoped servers;
- avoid modifying a user's global CLI configuration;
- pass arguments/environment as tokens;
- clean temporary or project files on normal and failed exits;
- avoid logging tokens, headers, or generated configuration.

### Add assets and registration

Optional logos live at:

```text
apps/backend/internal/agent/agents/logos/my-agent_light.svg
apps/backend/internal/agent/agents/logos/my-agent_dark.svg
```

Embed them with `//go:embed`. Keep the `AgentID` stable and independent from the display name; persisted profiles and API data refer to it.

Register every shipped agent in `LoadDefaults()` in [`internal/agent/registry/registry.go`](../../apps/backend/internal/agent/registry/registry.go):

```go
all := []agents.Agent{
    // Existing agents...
    agents.NewMyAgent(),
}
```

Ordinary identity, availability, profiles, models, and modes reach the UI through the registry API. A new capability, setting, or wire field can still require web types/components and mandatory Playwright coverage.

## Test the integration

Start with package tests:

```bash
cd apps/backend
CGO_ENABLED=1 go test -tags fts5 ./internal/agent/agents ./internal/agent/registry
CGO_ENABLED=1 go test -tags fts5 ./internal/agent/runtime/lifecycle \
  ./internal/agentctl/server/adapter/...
cd ../..
make lint-backend
```

Cover:

- unique ID and deterministic registry order;
- installed, missing, and ambiguous discovery;
- every model/session/permission argument as exact tokens;
- required and stripped environment variables;
- ACP initialize, `session/new`, prompt, cancellation, resume, and shutdown;
- capability probe changes and failures;
- passthrough terminal sizing, buffering, cleanup, and MCP materialization when offered;
- local/worktree plus each declared container or remote runtime;
- missing login, missing credential, and interrupted installation paths.

Then run `make test-backend` and start `make dev`. Verify discovery, profile selection, start/prompt/resume, model/mode refresh, permissions, and passthrough if supported.

The real adapter suite is separate:

```bash
make -C apps/backend test-e2e
```

It may launch installed third-party agents, require login, and incur paid usage. Run only after reading the target tests. Root `make test-e2e` is the browser Playwright suite, not this real-agent suite.

## Update an existing agent

Trace the behavior instead of assuming one file owns it:

| Change | Inspect |
|---|---|
| Identity, command, image, auth, permissions, resume | Agent definition under `internal/agent/agents/` |
| Models, modes, structured capabilities | ACP server/bridge and host-utility probe |
| Passthrough MCP format | Agent `PassthroughConfig` and `internal/agent/mcpconfig/` |
| Structured MCP gap | `RuntimeConfig.ProjectMCPStrategy` and adapter behavior |
| Registry/default availability | `internal/agent/registry/` and settings reconciler |
| Packaged native binary | Runtime bundle matrices, npm runtime packages, desktop resources, release tests |
| New user-visible field | Backend DTO/API, web types/UI, Playwright, and public docs |
