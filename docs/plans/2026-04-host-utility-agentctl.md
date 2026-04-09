# Host Utility Agentctl — Boot Probes, Capability API, Sessionless Utility Prompts

**Date:** 2026-04-08
**Status:** implemented
**PR:** TBD
**Decision:** ADR-0002

## Problem

Utility agent calls (enhance-prompt, commit-message, etc.) required an active task session because `lifecycle.Manager.ExecuteInferencePrompt` looked up the session's agentctl via `GetOrEnsureExecution` and spawned a one-shot ACP subprocess inside it. Three features could not fit that mold:

1. **Boot-time probes.** List models/modes/auth for each installed agent so the settings UI can render them and detect misconfiguration early.
2. **Settings refresh.** User clicks "refresh" to re-check auth/models after installing or re-authing a CLI.
3. **Sessionless enhance-prompt.** Enhance button in the new-task modal — no task or workspace exists yet.

Built-in utility prompts are self-contained (context is injected as template vars), so running them in a neutral tmp dir is semantically fine.

## Design

### Architecture

```
Backend boot
    ↓
waitForAgentctlControlHealthy
    ↓
HostUtilityManager.Start(ctx)  [goroutine]
    ├─ mktemp  kandev-host-utility-<pid>-*/
    ├─ filter registry → ACP-native inference agents
    └─ errgroup (parallel):
         for each agent type:
           ├─ IsInstalled? (skip → not_installed)
           ├─ ControlClient.CreateInstance(workdir=tmp/<type>)
           ├─ waitForClientHealthy
           └─ Probe() → cache
```

Per-call path (both probe and sessionless prompt):

```
caller → HostUtilityManager.getInstance(type)
       → per-type warm Client (one-shot ACP subprocess)
       → agentctl utility.{Probe|Execute}
       → initialize → session/new → [set_mode?] → [prompt?] → kill
```

### Packages and files

**New**
- `internal/agent/hostutility/` — `Manager`, `cache`, public API (`GetAll`, `Get`, `Refresh`, `ExecutePrompt`), types (`AgentCapabilities`, `Model`, `Mode`, `AuthMethod`, `PromptResult`, `Status` enum).
- `internal/agent/capabilities/handlers/` — HTTP routes:
  - `GET  /api/v1/agents/capabilities`
  - `GET  /api/v1/agents/:type/capabilities`
  - `POST /api/v1/agents/:type/probe`
  - `POST /api/v1/agents/:type/prompt` (raw one-off)

**Extended**
- `internal/agentctl/server/utility/types.go` — `ProbeRequest`/`ProbeResponse`, `Mode` field on `PromptRequest`, `ProbeAuthMethod`/`ProbeModel`/`ProbeMode`/`ProbePromptCapabilities`.
- `internal/agentctl/server/utility/acp_executor.go` — `Probe()` method, `executeACPSession` honors `mode` via `conn.SetSessionMode`. Parsing split into `buildInitProbeFields` + `applySessionProbeFields` helpers.
- `internal/agentctl/server/utility/handler.go` — `POST /api/v1/inference/probe` route.
- `internal/agentctl/client/utility.go` — factored `doLongRunningJSON` helper shared by `InferencePrompt` and new `Probe`.
- `internal/utility/template/engine.go` — `ResolveWithOptions` + `ResolveOptions{MissingAsEmpty bool}`.
- `internal/utility/service/service.go` — `PreparePromptRequest` takes a `sessionless bool` flag.
- `internal/utility/controller/controller.go` — threads the flag through.
- `internal/utility/handlers/handlers.go` — `session_id` optional; sessionless branch calls `HostUtilityExecutor.ExecutePrompt`; new `HostUtilityExecutor` interface.
- `cmd/kandev/main.go` — constructs `HostUtilityManager` after `waitForAgentctlControlHealthy`, starts it in a goroutine, registers `Stop()` cleanup via `addCleanup` with a 10s timeout. Threaded through `buildHTTPServer` → `routeParams`.
- `cmd/kandev/helpers.go` — `routeParams.hostUtilityMgr`; registers both the new capabilities routes and the updated utility routes.

### Config shapes (internal)

```go
// ProbeConfig — implicit, built per-call by the manager from registry + workdir.
// No Model — probes must omit --model so session/new returns the agent default
// plus the full availableModels list.
//
// PromptConfig — uses the existing InferenceConfigDTO; Model is required,
// Mode is optional and when set triggers session/set_mode after session/new.
```

### Caller responsibilities

| Param   | Probe | Sessionless prompt | Task-scoped prompt |
|---------|-------|--------------------|---------------------|
| WorkDir | host tmp dir (manager-owned) | host tmp dir | task workspace (unchanged) |
| Model   | **omitted** | required (utility record → user default → cached currentModelId) | from utility record / user default |
| Mode    | unused | optional | optional |

### Scope (v1)

- ACP-native agents only (`Runtime().Protocol == ProtocolACP`): claude-acp, codex-acp, copilot-acp, amp-acp, opencode-acp, auggie.
- Non-ACP protocols (streamjson/codex/copilot/amp/opencode adapters) excluded — later iteration.
- Cache is in-memory, populated at boot and on explicit `/probe` refresh. No TTL.
- Tmp dirs process-scoped (`kandev-host-utility-<pid>-*`); cleanup only removes dirs owned by this process.

## Implementation Notes

- **Lazy instance recreation.** `Manager.getInstance` recreates a warm instance if it's missing (never bootstrapped or crashed). No separate health loop.
- **Failure isolation.** Per-agent bootstrap failures land in the cache as `Status = not_installed | auth_required | failed`. `isAuthError` is a coarse string-match heuristic on the ACP SDK error — the SDK doesn't expose a distinct auth error code.
- **Env inheritance.** The agentctl instance is workspace-only and inherits the backend's env. The ephemeral ACP subprocess spawned by `Probe`/`Execute` also inherits (no `cmd.Env` override in `acp_executor.go`), so `ANTHROPIC_API_KEY`, `OPENAI_API_KEY`, etc. flow through naturally for v1.
- **`ExecutionStore` bypass deferred.** The original plan called for a `host-utility:` ID prefix branch in `manager_execution.go`. Once it became clear that `ControlClient.CreateInstance` + a regular `Client` does everything we need without touching the lifecycle manager's execution store, that branch was removed from the plan — host utility sits alongside the lifecycle manager rather than through it.
- **Worktree access.** Sessionless utility calls cannot read workspace files by design. All current built-ins are self-contained; a `requires_workspace` flag on `UtilityAgent` is the documented future escape hatch for custom templates that need file access.
- **Lint wrinkle.** `acp_executor.go` has one `//nolint:unconvert` on `string(m.Id)` where `m.Id` is `acp.AuthMethodId` (a named string type). The linter flags it as unnecessary, but Go requires the explicit conversion.
- **Frontend wiring is explicitly deferred.** Settings page, new-task modal enhance button, and startup capabilities fetch land in a follow-up PR. The API surface was designed to support all three.

### Verification

- `make fmt` clean
- `go build ./...` success
- `make test` all packages green
- `make lint` 0 issues
