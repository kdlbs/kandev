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

## Further scoped notes

- `server/api/AGENTS.md` — reverse-proxy body rewriting (`Accept-Encoding`) and iframe-blocking header stripping.
