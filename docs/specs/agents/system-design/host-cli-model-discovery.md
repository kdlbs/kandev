---
status: current
system: agents
requirements:
  - REQ-AGENTS-HOST-CLI-001
  - REQ-AGENTS-HOST-CLI-002
  - REQ-AGENTS-HOST-CLI-003
  - REQ-AGENTS-HOST-CLI-004
---

# Host CLI Model Discovery System Design

## Purpose and boundaries

The agent system owns agent availability and provider model options, so it
owns this design. The design adds one trusted description of the vendor CLI
per agent type, reads that CLI for its version and, where a documented
interface exists, its model catalogue, and keeps both caches honest around
Kandev's existing agent install action.

The design uses, but does not change, four adjacent contracts:

- The managed ACP bridge remains the session runtime and the source of the
  bridge list. See [runtime updates part 1](runtime-updates-01.md).
- The host utility manager remains the owner of the ACP capability probe and
  its cache (`internal/agent/hostutility`).
- The discovery registry remains the owner of agent availability
  (`internal/agent/discovery`).
- **The agent install action remains the only way Kandev installs or upgrades
  a vendor CLI.** It is unchanged by this design; see
  [Install and upgrade ownership](#install-and-upgrade-ownership).

Session launches never use the host CLI. A model selected from the CLI list or
typed as a custom ID is passed to the bridge exactly as any other profile
model. Whether the bridge accepts an identifier it does not advertise is the
bridge's own behavior; the Claude bridge resolves aliases and family names,
and rejects unrelated identifiers at session start with its existing error.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-AGENTS-HOST-CLI-001` | [Version detection](#version-detection), [Discovery projection](#discovery-projection) |
| `REQ-AGENTS-HOST-CLI-002` | [Model discovery](#model-discovery), [Merged model response](#merged-model-response), [Profile model selector](#profile-model-selector) |
| `REQ-AGENTS-HOST-CLI-003` | [Install and upgrade ownership](#install-and-upgrade-ownership), [Refresh triggers](#refresh-triggers) |
| `REQ-AGENTS-HOST-CLI-004` | [Profile model selector](#profile-model-selector) |

## Install and upgrade ownership

Kandev already installs and upgrades vendor CLIs. This design adds no second
mechanism; it hangs rediscovery off the existing one.

| Concern | Owner | Location |
| --- | --- | --- |
| Install script per agent type | `agents` package | `internal/agent/agents/claude_acp.go` (`npm install -g @anthropic-ai/claude-code …`), `internal/agent/agents/codex_acp.go` (`npm install -g @openai/codex …`) |
| Job enqueue | settings controller | `internal/agent/settings/controller/agent_install.go` (`EnqueueInstall`) |
| Streaming execution, retention, completion hook | settings controller | `internal/agent/settings/controller/install_job.go` (`JobStore`, its `onSuccess` callback) |
| Cross-action exclusivity | settings controller | `internal/agent/settings/controller/maintenance_jobs.go` |
| HTTP | settings handlers | `POST /api/v1/agent-install/:agentName`, `GET /api/v1/agent-install/jobs`, `GET /api/v1/agent-install/jobs/:id` |
| WebSocket | gateway | `agent.install.started`, `agent.install.output`, `agent.install.finished` |
| UI | web | `apps/web/components/settings/agents/agent-install-catalog.tsx`, `apps/web/components/settings/install-agent-card.tsx`, route `/settings/agents/browse` |

Because each install script is an `npm install -g` of the vendor package, a
successful install is also how an operator upgrades a CLI in place. The
separate managed runtime update (`/api/v1/agent-update`,
`agent-runtime-update-control.tsx`) upgrades the ACP bridge package, not the
vendor CLI, and is likewise unchanged.

## Components and responsibilities

- `internal/agent/hostcli` (new) owns the trusted `Spec` type, version
  parsing, and the Codex app-server model listing client. It has no dependency
  on the settings controller.
- `agents.HostCLIAgent` (new optional interface in `internal/agent/agents`)
  returns the `hostcli.Spec` for an agent type. `ClaudeACP`, `CodexACP`, and
  the E2E `MockAgent` implement it. Every other agent type does not, and keeps
  its current behavior.
- `internal/agent/discovery.Registry` runs the version command for available
  host-CLI agents during detection and carries the result on `Availability`.
- `internal/agent/settings/controller` owns the host-CLI model cache, the
  merged model projection, and the refresh triggers.
- `apps/web` adds the version badge on the agent card, the model source note,
  and the custom model entry in the shared selector.

## Data and contracts

### Host CLI specification

```go
type Spec struct {
    DisplayName string      // "Claude Code", "Codex CLI"
    Executable  string      // "claude", "codex"
    VersionArgs []string    // {"--version"}
    ModelSource ModelSource // ModelSourceNone | ModelSourceCodexAppServer
}
```

The spec is compiled metadata. Requests never carry an executable, argument,
or model source. The Claude spec declares `ModelSourceNone` because the
installed Claude Code CLI has no documented model listing: the Agent SDK's
`supportedModels()` is a Node API, and the CLI rejects the corresponding
control request in print mode on the versions verified during design. The
Codex spec declares `ModelSourceCodexAppServer`, the documented
`codex app-server` JSON-RPC protocol.

### Version detection

Discovery runs `<matched path> --version` with a five-second timeout and
extracts the first `MAJOR.MINOR.PATCH` token, so `2.1.220 (Claude Code)` and
`codex-cli 0.155.1` both resolve. A non-zero exit that still printed a version
is accepted; anything else becomes a one-line reason. The result is cached per
executable path inside the discovery registry with a ten-minute freshness
window and is invalidated together with the detection cache.

`Availability` gains two fields:

```go
CLIVersion      string // "2.1.220"
CLIVersionError string // short reason when empty
```

### Discovery projection

`AgentDiscoveryDTO` gains `cli_version` and `cli_version_error`. Both are
omitted for agent types without a host CLI, so existing consumers are
unchanged.

### Model discovery

`hostcli.ListCodexModels` spawns `<codex> app-server`, starts reading, then
sends `initialize`, `initialized`, and `model/list` (`{"limit": 200}`) with a
fifteen-second overall timeout, then terminates the process. Reading starts
before writing because the peer answers `initialize` before the client
finishes writing `model/list`; a reader started afterwards deadlocks on a full
pipe. Each entry maps `model`/`id`, `displayName`, `description`, `isDefault`,
and keeps `supportedReasoningEfforts` and `defaultReasoningEffort` in `Meta`.
Hidden models are excluded. A JSON-RPC error mentioning authentication maps to
`not_logged_in`; a missing executable maps to `not_installed`; a deadline maps
to `timeout`; anything else maps to `failed`.

The controller keeps one in-memory entry per agent type with the discovered
models, a status, an error message, and a check timestamp. Agent types with
`ModelSourceNone` resolve to `skipped` immediately, without a process.

### Merged model response

`DynamicModelsResponse` and `ModelConfigDTO` gain:

```json
{
  "discovery": {
    "source": "cli_command",
    "executable": "codex",
    "cli_version": "0.155.1",
    "status": "ok",
    "error": "",
    "checked_at": "2026-09-25T12:00:00Z",
    "allows_custom_model": true
  }
}
```

`models` is ordered CLI list first, then bridge entries whose `id` is not
already present. Every entry carries `source: "cli"` or `source: "acp"`.
`current_model_id` remains the bridge's current model. `status` is `ok`,
`skipped`, `pending` (nothing discovered yet), or a failure. The block is
omitted for agent types without a host CLI, and `allows_custom_model` is true
only for host-CLI agent types.

## Refresh triggers

Discovery must never run on a listing request's critical path, and must never
outlive the binary it described.

| Trigger | Entry point | Behavior |
| --- | --- | --- |
| Backend finished loading agents | `backendapp/main.go`, after `SetHostUtility` | `WarmHostCLIModels` in a goroutine: discovers every stale or missing catalogue |
| Agents settings page load or **Rescan** | `GET /api/v1/agents/discovery` handler | Invalidates the version cache (existing behavior), responds, then warms stale catalogues in a bounded goroutine |
| Existing agent install succeeded | `JobStore` `onSuccess` in `SetJobBroadcaster` | Invalidates the discovery cache (and with it the version cache), drops that agent's catalogue, and rediscovers it |
| Profile editor capability refresh | `GET /api/v1/agent-models/:agentName?refresh=true` | Re-reads the CLI for that agent synchronously with the probe refresh |
| Any other read | `/agents/available`, `/agent-models/:agentName` | Serves the cached catalogue; spawns nothing |

The ten-minute freshness window keeps repeated page loads from re-reading
every CLI, while the install hook and the explicit refresh both ignore it.

## Profile model selector

`ModelPicker` receives the discovery block. When `allows_custom_model` is
true the shared selector always shows its filter input and, while the typed
text does not exactly match an option, renders one extra row that selects the
typed text verbatim. A stored model absent from the list is rendered as an
enabled custom entry for these agent types; other agent types keep the
disabled "unavailable" row. A short note under the selector states the source
and version, or the fallback for a CLI that publishes no list, plus the last
discovery error with the existing retry action.

Desktop uses the existing popover; phones use the same popover contained in
the viewport with 44 px rows, as the selector already does. The custom row is
part of the same list, so keyboard and touch flows are unchanged.

## Agents settings card

The identity block shows `<DisplayName> <version>` after the detected path, or
a muted unknown-version line with its reason. No new action is added to the
card: installing or upgrading a CLI remains the existing install flow, and
updating the ACP bridge remains the existing runtime update control.

## Failure and recovery

- Version detection failures never mark the agent unavailable and never block
  discovery.
- Model discovery failures keep the bridge list and expose `status` and
  `error`; the profile editor shows the reason and the existing retry action.
- A failed or skipped discovery never rewrites saved profile data.
- A vendor process that never answers is killed at the deadline.

## Persistence

No database change. Versions and model entries are process-local and are
rebuilt after a backend restart. Profile rows keep storing the selected model
string.

## Security

- The executable and its arguments come from compiled agent metadata. No
  request field reaches a process argument.
- Discovery runs as the Kandev process user and relies on the CLI's own stored
  login; it never sends credentials.
- Discovery is read-only: it starts no session, writes no file, and changes no
  installed package.

## Observability

Structured logs: info on a successful listing (`agent`, `models`), debug on a
failed version check or listing (`agent`, `executable`, `status`, error).

## Related decisions

- [ADR-2026-09-25-host-cli-as-model-and-version-source](../../../decisions/2026-09-25-host-cli-as-model-and-version-source.md)
- [ADR-2026-08-12-validated-managed-runtime-version-selection](../../../decisions/2026-08-12-validated-managed-runtime-version-selection.md)
