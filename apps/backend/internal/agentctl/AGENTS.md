# agentctl — HTTP server, adapters, ACP protocol

Scoped guidance for `apps/backend/internal/agentctl/`. Higher-level backend architecture is in `apps/backend/AGENTS.md`.

## API Groups

agentctl exposes these route groups (see `server/api/`):
- `/health`, `/info`, `/status` - Health and status
- `/instances/*` - Multi-instance management
- `/processes/*` - Agent subprocess management (start/stop)
- `/agent/configure`, `/agent/stream` - Agent configuration and event streaming
- `/git/*` - Git operations (status, commit, push, pull, rebase, stage, create PR, etc.)
- `/shell/*` - Shell session management
- `/workspace/*` - File operations, search, tree
- `/vscode/*` - VS Code integration proxy

## Pull request creation (`server/process/git_pr_providers.go`)

`GitOperator.CreatePR` picks a host CLI from `origin`:

| Remote host | CLI | Notes |
|-------------|-----|-------|
| `github.com`, `*.github.com` | `gh pr create` | Requires `gh` on `PATH` (included in the Kandev Docker image). |
| `gitlab.com`, configured self-managed host | `glab mr create` or REST v4 | `KANDEV_GITLAB_HOST` binds detection/auth to the workspace origin. `GITLAB_TOKEN` is used only for that exact host; the REST fallback resolves the project default branch and reuses an existing open source/target MR on retry. |
| `dev.azure.com`, `ssh.dev.azure.com`, `*.visualstudio.com` | `az repos pr create` | Requires `az` on `PATH`, `azure-devops` extension (both in the Kandev Docker image). Auth: `az login` or `AZURE_DEVOPS_EXT_PAT`. |
| Other hosts | — | Returns an unsupported-remote error. |

GitHub PR and GitLab MR URLs trigger backend association callbacks. Azure PR URLs are returned to the client but do not trigger TaskPR linkage. The web UI keeps a session-scoped pending URL so the changes panel hides the create action after any successful provider response.

## Adapter Model

Protocol adapters in `server/adapter/transport/` normalize different agent CLIs:
- `AgentAdapter` interface defines `Start()`, `Stop()`, `Prompt()`, `Cancel()`
- Transports: `acp` (Claude Code), `codex` (OpenAI Codex), `opencode`, `shared`, `streamjson`
- Top-level adapters: `CopilotAdapter` (GitHub Copilot SDK), `AmpAdapter` (Sourcegraph Amp)
- `process.Manager` owns subprocess, wires stdio to adapter
- Factory pattern in `server/adapter/factory.go` selects adapter by agent type

The `acp` transport is split by concern across `adapter_*.go` files: `adapter.go` (core/lifecycle), `adapter_session.go` (initialize/new/load/resume), `adapter_prompt.go` (prompt/cancel), `adapter_updates.go` (`session/update` notification fan-out), `adapter_tools.go` (`convertToolCallUpdate` / `convertToolCallResultUpdate` -> normalized payloads), `adapter_permissions.go`, and `adapter_helpers.go`. Agent-specific ACP extensions use the package-private `acpDialect` function table in `dialect.go`; keep observed wire translation in `dialect_<agent>.go`. Dialect hooks return normalized data or request descriptions and never receive `*Adapter` or execute RPCs. Shared capability normalization used by both live sessions and utility probes belongs in `internal/agentctl/acpcompat/`. Tool-call conversion lives in `adapter_tools.go`, not `adapter.go`. See ADR-0043.

### Grok ACP dialect (`dialect_grok.go`)

Grok exposes some model, config, and usage surfaces through implementation-specific ACP metadata. Its dialect normalizes them in the backend so shared ACP paths and the frontend stay provider-agnostic:

| Grok wire | Kandev shape |
|---|---|
| Legacy `models` catalog | model `ConfigOption` used by generic frontend selectors |
| Model `_meta.supportsReasoningEffort` / `reasoningEfforts` / `reasoningEffort` | gated `reasoning_effort` select with category `thought_level` |
| `SetConfigOption(model, …)` or `(reasoning_effort, …)` | Grok has **no** `session/set_config_option`. Rewrite both to `session/set_model` (effort keeps current modelId + `_meta.reasoningEffort`; model switch may carry prior effort in meta when still supported). Frontend always uses set_config_option when a model ConfigOption exists. |
| Mid-session `MODEL_SWITCH_INCOMPATIBLE_AGENT` | return Grok's actionable error; user starts a new session explicitly |
| Notification `_meta.totalTokens` + model `_meta.totalContextTokens` | `EventTypeContextWindow` (used/size/remaining). Compaction may decrease used; clear used on model switch. |
| Prompt `_meta.usage` | input/output/cache/total plus `reasoningTokens` as `PromptUsage.ThoughtTokens`; sibling flat fields are not whole-turn totals |

Grok ACP currently exposes neither per-turn cost nor subscription quota/reset values. Do not estimate them or treat the optional subscription tier label as usage data.

## ACP Protocol

JSON-RPC 2.0 over stdin/stdout between agentctl and agent process. Requests: `initialize`, `session/new`, `session/load`, `session/prompt`, `session/cancel`. Notifications: `session/update` with types `message_chunk`, `tool_call`, `tool_update`, `complete`, `error`, `permission_request`, `context_window`.

### ACP frame debug logging (`adapter/transport/shared/acplog.go`)

When `KANDEV_DEBUG_AGENT_MESSAGES=true` (on by default in the **dev** profile), the ACP adapter dumps every raw + normalized frame to **per-session** JSONL files:

- Files: `raw-{protocol}-{agentID}-{sessionID}.jsonl` and `normalized-{protocol}-{agentID}-{sessionID}.jsonl` (the `raw-`/`normalized-` prefix + `.jsonl` suffix is a contract with the reader in `internal/debug`).
- Dir resolution: `KANDEV_DEBUG_LOG_DIR` (explicit override) → `<KANDEV_HOME_DIR>/logs/acp` (honors dev/e2e isolation — `KANDEV_HOME_DIR` is already the Kandev root, so no extra `.kandev` segment) → `~/.kandev/logs/acp/` → process CWD.
- One kept-open buffered writer + dedicated mutex per session (no global lock on the hot path); rotates on a per-file byte cap.
- A `shared.Janitor` (owned by `cmd/agentctl/main.go run()`, `Start`/`Stop`) flushes periodically and prunes oldest-by-mtime first, enforcing a total-file cap and an age cap so an always-on dev session can't fill the disk. It also closes idle writers so handles don't leak.
- Dev-only live tail of a stuck session from an in-memory ring buffer (zero disk growth): `GET /api/v1/debug/acp/{session}?n=200`.

Env knobs (all optional): `KANDEV_DEBUG_ACP_MAX_FILES` (default 200), `KANDEV_DEBUG_ACP_RETENTION_HOURS` (default 48), `KANDEV_DEBUG_ACP_MAX_FILE_BYTES` (default 8 MiB).

**PRIVACY / PERF:** frames carry the full prompt, file, and tool-call content. Keep this strictly behind the debug flag and local-dev-scoped — never enable in a shared/production deployment. When agentctl runs inside a Docker executor the files land *inside the container*, so this is meant for standalone/dev.

### Recognizing agent-specific ACP meta-tagged tools

`claude-agent-acp` tags certain tool calls with `_meta.claudeCode.toolName` (e.g. `Monitor`, `ScheduleWakeup`, `Agent` for subagents) and may carry results in `_meta.claudeCode.toolResponse`. `codex-acp` reports collaboration and subagent lifecycle through `_meta.codex`. These are normalized into typed `streams.NormalizedPayload` kinds in `server/adapter/transport/acp/`. The established pattern is a defensive untyped-map recognizer, typed normalized data, and sibling tests. Shared recognized tools live in files such as `monitor.go`, `wakeup.go`, and `subagent.go`; agent-scoped wire translation belongs in `dialect_<agent>.go` (for example `dialect_codex.go`). `convertToolCallUpdate` stashes `title`/`Meta` into the normalizer args; result enrichment happens in `convertToolCallResultUpdate`. Don't inline detection in `adapter.go`. Detection can also be cross-agent: common subagent recognition keys off Claude's `_meta`, OpenCode's tool `title`, Cursor's `rawInput._toolName`, and Auggie's title prefix.

### Subagent tool-call nesting: what each agent emits

Kandev renders subagents (the `Task` tool) as cards and *wants* to nest each subagent's internal tool calls under its card (via `parent_tool_call_id`, see `tool-subagent-message.tsx`). Reality differs per agent — verified from captured `~/.kandev/logs/acp/` frames of a "spawn 3 subagents that each run `sleep 30`" prompt:

| Agent | Subagent-internal tool calls | Nestable? |
|---|---|---|
| **Claude** | emitted on the parent session, each tagged with **`_meta.claudeCode.parentToolUseId`** = the parent Task tool_call's id | **Yes** — `parentToolUseID()` reads it in `adapter_tools.go` and sets `AgentEvent.ParentToolCallID`, which becomes the message's `parent_tool_call_id` and nests under the card |
| **Codex** | **not emitted over ACP**; `codex-acp` emits top-level collaboration and lifecycle tool calls with `_meta.codex.collaboration` / `_meta.codex.subagent` | No — spawn/start signals render through `subagent_task` and preserve the child thread ID, but there are no child internal tool calls to attach |
| **Cursor** | **not emitted over ACP at all** (Task `tool_call_update` carries only `{durationMs, isBackground}`) | No — no child data exists |
| **OpenCode** | emitted into a **separate child ACP session** (the `metadata.sessionId` we store as `SubagentTaskPayload.ChildSessionID`) | Not yet — they never reach the parent stream; would require merging that child session via the stored `child_session_id` |

Claude is the one that supports nested child tools today: `claude-agent-acp` (since PR #859) sets `_meta.claudeCode.parentToolUseId` on a subagent's internal calls, and its value already equals the parent Task tool_call id — so it maps straight onto our `parent_tool_call_id`. Codex reports child lifecycle but not child tools. Cursor exposes nothing to nest. OpenCode needs a Kandev-side child-session merge. (Top-level `parentToolCallId` is NOT in the ACP schema — `_meta` is the spec-compliant carrier.)

## Process-group cleanup and `RequiresProcessKill`

All ACP agents are launched in their own process group, and shutdown must not
report success while any process in that group is still alive. Some agents —
notably `opencode acp` — keep their HTTP server and an MCP child tree alive after
stdin EOF. For those agents, closing stdin on shutdown is not useful; the process
group has to be killed immediately so the MCP children don't re-parent to init
and leak (GH issue #1247).

The "skip graceful wait" signal flows agent → adapter → process manager via a
single bool:

1. `agents.RuntimeConfig.RequiresProcessKill` (set `true` on `OpenCodeACP`).
2. The lifecycle executors copy it into `agentctl.CreateInstanceRequest.RequiresProcessKill`.
3. `instance.Manager` stores it on `config.InstanceConfig.RequiresProcessKill`.
4. `process.Manager.buildAdapterConfig` forwards it into `adapter.Config` → `shared.Config`.
5. `acp.Adapter.RequiresProcessKill()` returns `cfg.RequiresProcessKill`.
6. On shutdown, `process.Manager.killProcessGroupIfRequired` consults the adapter and, when true, sends SIGKILL to the whole pgid via `killProcessGroup` (`syscall.Kill(-pid, SIGKILL)` on Unix).

`RequiresProcessKill: false` does **not** mean children may be left running. It
only allows the normal graceful path first (stdin close, adapter close, process
wait). After the command leader exits, `waitForProcessExit` still checks the
agent process group and sends SIGTERM/SIGKILL if descendants remain. If `Stop(ctx)`
times out, it also re-runs the pgid SIGKILL fallback.

To add another agent that needs immediate kill instead of graceful stdin close:
set `RequiresProcessKill: true` in its `Runtime()` config.

## Env stripping for credential-mode agents (`StripEnv`)

Some agents check environment variables to decide their credential mode. For example, `devin acp` checks `ACP_BACKEND`: when set (any value, including empty), it requires protocol-level `authenticate` and refuses local credentials; when unset, it falls back to reading `~/.local/share/devin/credentials.toml` directly. Kandev (which may inherit `ACP_BACKEND` from Windsurf Next) needs the fall-back path, so the variable must be **absent** from the child environment — not just empty.

The strip list flows agent → instance config → process manager via a single slice, mirroring `RequiresProcessKill`:

1. `agents.RuntimeConfig.StripEnv` (set `[]string{"ACP_BACKEND"}` on `DevinACP`).
2. The lifecycle executors copy it into `agentctl.CreateInstanceRequest.StripEnv`.
3. `instance.Manager` stores it on `config.InstanceConfig.StripEnv`.
4. `process.Manager.buildAdapterConfig` iterates the list and calls `utility.RemoveEnvEntry` on `m.cfg.AgentEnv` before that env is used to spawn child processes.

For the one-shot probe/inference path, the strip list is derived from `Runtime().StripEnv` via the shared `agents.StripEnvFor` helper — it is not an independent field on `InferenceConfig`. The derived value is propagated through `InferenceConfigDTO.StripEnv` and applied by `utility.sanitizeEnvForAgent` before spawning the ephemeral subprocess.

To add another agent that needs env vars stripped: set `StripEnv: []string{"VAR_NAME"}` in its `Runtime()` — that's all.

## Idle-instance reaper (`KANDEV_ACP_IDLE_TIMEOUT`)

`instance.Manager` runs a background goroutine that reaps instances whose owning kandev backend appears to be gone. An instance is "idle" when it has zero in-flight HTTP requests *and* its most recent observed activity is older than the configured timeout. Activity is tracked by a middleware on the per-instance handler that bumps a counter on every request entry/exit, so a long-lived `/agent/stream` WebSocket keeps the instance "active" for as long as the stream is open.

Env knobs:
- `KANDEV_ACP_IDLE_TIMEOUT` (default `1h`, `0` to disable)
- `KANDEV_ACP_IDLE_REAPER_INTERVAL` (default `1m`)

Both accept any value `time.ParseDuration` accepts (`30m`, `2h`, `500ms`, …). Disabled mode is intended for tests and edge cases — production should keep the reaper on.

## MCP stdio command validation

`instance.Manager.buildMcpServerConfigs` calls `exec.LookPath` on every stdio MCP `Command` before passing the list to the agent. Entries whose command can't be resolved are dropped with a warn log (URL-transport MCPs always pass through). This prevents the `/snap/bin/brave` repro in GH issue #1247 — an MCP whose binary was uninstalled after config save no longer causes a permanently broken child process to be spawned every session.

## Further scoped notes

- `server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`) and iframe-blocking header stripping.
