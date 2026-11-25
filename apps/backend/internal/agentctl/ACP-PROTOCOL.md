## ACP Protocol

JSON-RPC 2.0 over stdin/stdout between agentctl and agent process. Requests: `initialize`, `session/new`, `session/load`, `session/resume`, `session/prompt`, `session/cancel`, `session/close`. Notifications: `session/update` with types `message_chunk`, `tool_call`, `tool_update`, `complete`, `error`, `permission_request`, `context_window`.

The adapter prefers advertised `session/resume` for any agent to restore the saved conversation without replaying its history. Both resume and load responses preserve typed configuration and legacy model state. If an agent advertises resume but returns method-not-found, the adapter uses `session/load` only when advertised and the context is still active. Other errors preserve the saved identity. Restore traces contain separate `session.resume` and `session.load` spans for the actual requests.

OpenCode v1 and v2 share the `opencode-acp` identity and can access the same session database. For OpenCode only, a failed restore never falls back to `session/new`, including a recognized missing-session response. Preserve the native session ID and return the load error to the existing Kandev session.

### Native prompt suggestions (Claude)

`supports_prompt_suggestions` is effective enablement; the optional
`native_prompt_suggestions_supported` is support alone (nil = unknown) and must
never enable the composer. `dialect_claude_prompt_suggestions.go` accepts
literal `_meta.claudeCode.promptSuggestions: true`, or, when absent, only the
exact official bridge `@agentclientprotocol/claude-agent-acp` `0.81.2` or
`0.100.0` with `promptQueueing: true`; false or malformed denies support, and names,
namespaces, queueing, and models never qualify. Supported sessions configure
new/load/resume with `claudeCode.options.promptSuggestions` plus the requested
raw `prompt_suggestion` filter; `adapter_prompt_suggestion.go` owns window and
origin handling. Failed or same-ID transitions keep the prior window ambiguous.
Design: `docs/specs/agents/system-design/native-prompt-suggestion-capability.md`.

### ACP permission identity and injected MCP approval

The ACP client preserves `ToolCall.Name` and `ToolCall.Meta` on the internal
permission request. The Claude dialect reads `_meta.claudeCode.toolName` only
for Claude frames and uses an exact qualified title fallback only when both
identity fields are absent and the ACP kind is `other`. Malformed or
conflicting identity data stays in the normal permission path. Other provider
dialects must not inherit this title fallback without a tested wire contract.

The process manager may automatically select an offered allow-once, then
allow-always option for any qualified tool on the host-injected Kandev MCP
server when blanket approval is off. It requires the internal construction
provenance marker and the exact current-port HTTP or SSE server entry. This
server-wide provider-layer rule includes destructive Kandev tools; MCP
authentication, task/session authorization, questions, and workflow gates stay
separate. Shell, file, third-party, ambiguous, malformed, and unqualified
requests keep the pending permission flow. Internal identity fields are not
part of permission snapshots or stream events.

The flattened `mcp__server__tool` parser accepts only one unambiguous `__`
separator. It rejects delimiter collisions, adjacent underscores at the
boundary, and additional `__` sequences in the tool suffix. This prevents a
server such as `kandev__external` from being read as the reserved `kandev`
server. Tool names with these ambiguous forms keep the pending flow.

### ACP frame debug logging (`adapter/transport/shared/acplog.go`)

When `KANDEV_DEBUG_AGENT_MESSAGES=true` (on by default in the **dev** profile), the ACP adapter dumps every raw + normalized frame to **per-session** JSONL files:

- Files: `raw-{protocol}-{agentID}-{sessionID}.jsonl` and `normalized-{protocol}-{agentID}-{sessionID}.jsonl` (the `raw-`/`normalized-` prefix + `.jsonl` suffix is a contract with the reader in `internal/debug`).
- Dir resolution: `KANDEV_DEBUG_LOG_DIR` (explicit override) → `<KANDEV_HOME_DIR>/logs/acp` (honors dev/e2e isolation — `KANDEV_HOME_DIR` is already the Kandev root, so no extra `.kandev` segment) → `~/.kandev/logs/acp/` → process CWD.
- One kept-open buffered writer + dedicated mutex per session (no global lock on the hot path); rotates on a per-file byte cap.
- A `shared.Janitor` (owned by `cmd/agentctl/main.go run()`, `Start`/`Stop`) flushes periodically and prunes oldest-by-mtime first, enforcing a total-file cap and an age cap so an always-on dev session can't fill the disk. It also closes idle writers so handles don't leak.
- Age and count pruning check the active writer registry and unlink under the same manager lock used by writer registration. This protects only active segments held by this agentctl process; rotated segments and files open in another agentctl sharing the directory are not protected. Active segments still count toward the count limit, so the directory can exceed it only when those open segments alone exceed the limit.
- Agent subprocesses never inherit these settings. `launchwrapper.IsAgentDebugSetting` defines the set (`KANDEV_DEBUG_AGENT_MESSAGES`, `KANDEV_DEBUG_LOG_DIR`, `KANDEV_DEBUG_ACP_*`), and agent config, utility probes, and ACP `terminal/create` remove it, including explicit per-instance and terminal overrides. The owning agentctl reads its own process environment, so its logging is unaffected. The `acp` test package's `TestMain` points the registry at a temporary directory and exits before tests run if that directory cannot be created.
- Dev-only live tail of a stuck session from an in-memory ring buffer (zero disk growth): `GET /api/v1/debug/acp/{session}?n=200`.

Env knobs (all optional): `KANDEV_DEBUG_ACP_MAX_FILES` (default 200), `KANDEV_DEBUG_ACP_RETENTION_HOURS` (default 48), `KANDEV_DEBUG_ACP_MAX_FILE_BYTES` (default 8 MiB).

**PRIVACY / PERF:** frames carry the full prompt, file, and tool-call content. Keep this strictly behind the debug flag and local-dev-scoped — never enable in a shared/production deployment. When agentctl runs inside a Docker executor the files land *inside the container*, so this is meant for standalone/dev.

### Recognizing agent-specific ACP meta-tagged tools

`claude-agent-acp` tags certain tool calls with `_meta.claudeCode.toolName` (e.g. `Monitor`, `ScheduleWakeup`, `Agent` for subagents) and may carry results in `_meta.claudeCode.toolResponse`. `codex-acp` reports collaboration and subagent lifecycle through legacy `_meta.codex` or the 2.x structured `rawInput` envelope. Keep normalization, correlation, and capacity evidence on the shared Codex wire interpreter; explicit legacy metadata remains authoritative. Spawn fallback requires the exact `spawnAgent` title plus the structured sender/receiver/state fields, never a title alone. These are normalized into typed `streams.NormalizedPayload` kinds in `server/adapter/transport/acp/`. The established pattern is a defensive untyped-map recognizer, typed normalized data, and sibling tests. Shared recognized tools live in files such as `monitor.go`, `wakeup.go`, and `subagent.go`; agent-scoped wire translation belongs in `dialect_<agent>.go` (for example `dialect_codex.go`). `convertToolCallUpdate` stashes `title`/`Meta` into the normalizer args; result enrichment happens in `convertToolCallResultUpdate`. Don't inline detection in `adapter.go`. Detection can also be cross-agent: common subagent recognition keys off Claude's `_meta`, OpenCode's tool `title`, Cursor's `rawInput._toolName`, and Auggie's title prefix.

### Subagent tool-call nesting: what each agent emits

Kandev renders subagents (the `Task` tool) as cards and *wants* to nest each subagent's internal tool calls under its card (via `parent_tool_call_id`, see `tool-subagent-message.tsx`). Reality differs per agent — verified from captured `~/.kandev/logs/acp/` frames of a "spawn 3 subagents that each run `sleep 30`" prompt:

| Agent | Subagent-internal tool calls | Nestable? |
|---|---|---|
| **Claude** | emitted on the parent session, each tagged with **`_meta.claudeCode.parentToolUseId`** = the parent Task tool_call's id | **Yes** — `parentToolUseID()` reads it in `adapter_tools.go` and sets `AgentEvent.ParentToolCallID`, which becomes the message's `parent_tool_call_id` and nests under the card |
| **Codex** | **not emitted over ACP**; spawn/start signals use legacy `_meta.codex.collaboration` / `_meta.codex.subagent` or the 2.x structured input equivalents, while `wait` and `closeAgent` report child status and results through `rawInput.agentsStates` | No — spawn/start signals render through `subagent_task`, and completed wait/close reports update that existing card; these reports do not attest background liveness, and there are no child internal tool calls to attach |
| **Copilot** | emitted on the parent session with `_meta["github.com/copilot"].agentId`; parent calls report `agent_type`, `name`, `description`, `prompt`, and `mode` in `rawInput` | Yes, when the child ID matches a background launch or exactly one open foreground card has no child binding; ambiguous calls remain top-level |
| **Cursor** | **not emitted over ACP at all** (Task `tool_call_update` carries only `{durationMs, isBackground}`) | No — no child data exists |
| **OpenCode** | emitted into a **separate child ACP session** (the `metadata.sessionId` we store as `SubagentTaskPayload.ChildSessionID`) | Not yet — they never reach the parent stream; would require merging that child session via the stored `child_session_id` |

Top-level `parentToolCallId` is not in the ACP schema. Use `_meta` for Claude parent identity.
