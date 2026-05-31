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
| `dev.azure.com`, `ssh.dev.azure.com`, `*.visualstudio.com` | `az repos pr create` | Requires `az` on `PATH`, `azure-devops` extension (both in the Kandev Docker image). Auth: `az login` or `AZURE_DEVOPS_EXT_PAT`. |
| Other hosts (e.g. GitLab) | — | Returns an unsupported-remote error. Use host-specific CLIs outside agentctl (e.g. `/pr` skill `glab` for GitLab). |

Azure PR URLs are returned to the client but do not trigger backend `onPRCreated` / TaskPR linkage (GitHub-only today). The web UI keeps a session-scoped pending PR URL so the changes panel hides **Create PR** after a successful Azure create.

## Adapter Model

Protocol adapters in `server/adapter/transport/` normalize different agent CLIs:
- `AgentAdapter` interface defines `Start()`, `Stop()`, `Prompt()`, `Cancel()`
- Transports: `acp` (Claude Code), `codex` (OpenAI Codex), `opencode`, `shared`, `streamjson`
- Top-level adapters: `CopilotAdapter` (GitHub Copilot SDK), `AmpAdapter` (Sourcegraph Amp)
- `process.Manager` owns subprocess, wires stdio to adapter
- Factory pattern in `server/adapter/factory.go` selects adapter by agent type

The `acp` transport is split by concern across `adapter_*.go` files: `adapter.go` (core/lifecycle), `adapter_session.go` (initialize/new/load/resume), `adapter_prompt.go` (prompt/cancel), `adapter_updates.go` (`session/update` notification fan-out), `adapter_tools.go` (`convertToolCallUpdate` / `convertToolCallResultUpdate` → normalized payloads), `adapter_permissions.go`, and `adapter_helpers.go`. Tool-call conversion lives in `adapter_tools.go`, not `adapter.go`.

## ACP Protocol

JSON-RPC 2.0 over stdin/stdout between agentctl and agent process. Requests: `initialize`, `session/new`, `session/load`, `session/prompt`, `session/cancel`. Notifications: `session/update` with types `message_chunk`, `tool_call`, `tool_update`, `complete`, `error`, `permission_request`, `context_window`.

### ACP frame debug logging (`adapter/transport/shared/acplog.go`)

When `KANDEV_DEBUG_AGENT_MESSAGES=true` (on by default in the **dev** profile), the ACP adapter dumps every raw + normalized frame to **per-session** JSONL files:

- Files: `raw-{protocol}-{agentID}-{sessionID}.jsonl` and `normalized-{protocol}-{agentID}-{sessionID}.jsonl` (the `raw-`/`normalized-` prefix + `.jsonl` suffix is a contract with the reader in `internal/debug`).
- Default dir: `~/.kandev/logs/acp/` (override with `KANDEV_DEBUG_LOG_DIR`).
- One kept-open buffered writer + dedicated mutex per session (no global lock on the hot path); rotates on a per-file byte cap.
- A `shared.Janitor` (owned by `cmd/agentctl/main.go run()`, `Start`/`Stop`) flushes periodically and prunes oldest-by-mtime first, enforcing a total-file cap and an age cap so an always-on dev session can't fill the disk. It also closes idle writers so handles don't leak.
- Dev-only live tail of a stuck session from an in-memory ring buffer (zero disk growth): `GET /api/v1/debug/acp/{session}?n=200`.

Env knobs (all optional): `KANDEV_DEBUG_ACP_MAX_FILES` (default 200), `KANDEV_DEBUG_ACP_RETENTION_HOURS` (default 48), `KANDEV_DEBUG_ACP_MAX_FILE_BYTES` (default 8 MiB).

**PRIVACY / PERF:** frames carry the full prompt, file, and tool-call content. Keep this strictly behind the debug flag and local-dev-scoped — never enable in a shared/production deployment. When agentctl runs inside a Docker executor the files land *inside the container*, so this is meant for standalone/dev.

### Recognizing claude-acp meta-tagged tools

`claude-agent-acp` tags certain tool calls with `_meta.claudeCode.toolName` (e.g. `Monitor`, `ScheduleWakeup`, `Agent` for subagents) and may carry results in `_meta.claudeCode.toolResponse`. These are normalized into typed `streams.NormalizedPayload` kinds in `server/adapter/transport/acp/`. The established pattern is **one file per recognized tool** (`monitor.go`, `wakeup.go`, `subagent.go`), each with a defensive untyped-map recognizer (`recognize*`/`is*Meta`/`extract*`), a typed payload, and a sibling `*_test.go`. `convertToolCallUpdate` stashes `title`/`Meta` into the normalizer args; result enrichment happens in `convertToolCallResultUpdate`. To add another claude-acp meta-tool, copy that shape — don't inline detection in `adapter.go`. Detection can also be cross-agent: subagent recognition keys off Claude's `_meta`, OpenCode's tool `title`, and Cursor's `rawInput._toolName`.

### Subagent tool-call nesting: what each agent emits

Kandev renders subagents (the `Task` tool) as cards and *wants* to nest each subagent's internal tool calls under its card (via `parent_tool_call_id`, see `tool-subagent-message.tsx`). Reality differs per agent — verified from captured `~/.kandev/logs/acp/` frames of a "spawn 3 subagents that each run `sleep 30`" prompt:

| Agent | Subagent-internal tool calls | Nestable? |
|---|---|---|
| **Claude** | emitted on the parent session, each tagged with **`_meta.claudeCode.parentToolUseId`** = the parent Task tool_call's id | **Yes** — `parentToolUseID()` reads it in `adapter_tools.go` and sets `AgentEvent.ParentToolCallID`, which becomes the message's `parent_tool_call_id` and nests under the card |
| **Cursor** | **not emitted over ACP at all** (Task `tool_call_update` carries only `{durationMs, isBackground}`) | No — no child data exists |
| **OpenCode** | emitted into a **separate child ACP session** (the `metadata.sessionId` we store as `SubagentTaskPayload.ChildSessionID`) | Not yet — they never reach the parent stream; would require merging that child session via the stored `child_session_id` |

Claude is the one that works today: `claude-agent-acp` (since PR #341) sets `_meta.claudeCode.parentToolUseId` on a subagent's internal calls, and its value already equals the parent Task tool_call id — so it maps straight onto our `parent_tool_call_id`. Cursor exposes nothing to nest. OpenCode needs a kandev-side child-session merge. (Top-level `parentToolCallId` is NOT in the ACP schema — `_meta` is the spec-compliant carrier.)

## Further scoped notes

- `server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`) and iframe-blocking header stripping.
