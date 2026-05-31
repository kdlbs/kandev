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

## Further scoped notes

- `server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`) and iframe-blocking header stripping.
