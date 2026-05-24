# agentctl — HTTP server, adapters, ACP protocol

Scoped guidance for `apps/backend/internal/agentctl/`. Higher-level backend architecture is in `apps/backend/AGENTS.md`.

## API Groups

agentctl exposes these route groups (see `server/api/`):
- `/health`, `/info`, `/status` - Health and status
- `/instances/*` - Multi-instance management
- `/processes/*` - Agent subprocess management (start/stop)
- `/agent/configure`, `/agent/stream` - Agent configuration and event streaming
- `/git/*` - Git operations (status, commit, push, pull, rebase, stage, etc.)
- `/shell/*` - Shell session management
- `/workspace/*` - File operations, search, tree
- `/vscode/*` - VS Code integration proxy

## Adapter Model

Protocol adapters in `server/adapter/transport/` normalize different agent CLIs:
- `AgentAdapter` interface defines `Start()`, `Stop()`, `Prompt()`, `Cancel()`
- Transports: `acp` (Claude Code), `codex` (OpenAI Codex), `opencode`, `shared`, `streamjson`
- Top-level adapters: `CopilotAdapter` (GitHub Copilot SDK), `AmpAdapter` (Sourcegraph Amp)
- `process.Manager` owns subprocess, wires stdio to adapter
- Factory pattern in `server/adapter/factory.go` selects adapter by agent type

## ACP Protocol

JSON-RPC 2.0 over stdin/stdout between agentctl and agent process. Requests: `initialize`, `session/new`, `session/load`, `session/prompt`, `session/cancel`. Notifications: `session/update` with types `message_chunk`, `tool_call`, `tool_update`, `complete`, `error`, `permission_request`, `context_window`.

## Further scoped notes

- `server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`) and iframe-blocking header stripping.
