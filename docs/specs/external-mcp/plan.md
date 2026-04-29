# External MCP Endpoint — Implementation Plan

**Spec:** `./spec.md`

## Architecture

The existing MCP machinery already separates cleanly along a thin seam:

- `internal/agentctl/server/mcp/` — `Server` wraps `mark3labs/mcp-go`, registers tools, exposes routes.
- The tools talk to the backend through one interface:
  ```go
  type BackendClient interface {
      RequestPayload(ctx context.Context, action string, payload, result interface{}) error
  }
  ```
  In `agentctl` this is `ChannelBackendClient`, which tunnels over the agent-stream WebSocket and lands on handlers registered against `internal/mcp/handlers.Handlers` on the backend's `ws.Dispatcher`.

Reuse path: register a second `mcp.Server` instance **inside the backend**, pointed at a `BackendClient` that calls the same `ws.Dispatcher` directly — no WS round-trip, no tunneling. The dispatcher is just a `map[string]Handler`, so direct dispatch is a one-line call.

## File map

**Move**
- `internal/agentctl/server/mcp/` → `internal/mcp/server/` (package renamed `mcpserver`).
  - Two import sites to update: `cmd/agentctl/main.go`, `internal/agentctl/server/api/server.go`.
  - Test files come along.

**Modify** (`internal/mcp/server/server.go`)
- `New(...)` keeps current signature — sessionID/taskID stay (used in log fields), pass empty strings for the backend instance.
- Add `RegisterRoutesAt(router gin.IRouter, basePath string)` that registers `<basePath>`, `<basePath>/sse`, `<basePath>/message`. Existing `RegisterRoutes` keeps registering at root for agentctl.
- Pass `basePath` through to `WithBaseURL` so the SSE event embeds the right `messageEndpoint` URL.

**New** (`internal/mcp/external/`)
- `direct_backend.go` — `DirectBackendClient` wraps a `*ws.Dispatcher`, implements `BackendClient.RequestPayload` by building a `*ws.Message`, calling `Dispatch`, returning the unmarshalled payload.
- `provider.go` — `Provide(dispatcher *ws.Dispatcher, baseURL string, log *logger.Logger) (*mcpserver.Server, error)` constructs the MCP server in `ModeConfig` with `disableAskQuestion=true`.
- `direct_backend_test.go`, `provider_test.go` — unit + integration tests.

**Modify** (`cmd/kandev/helpers.go`)
- After `mcpHandlers.RegisterHandlers(p.gateway.Dispatcher)` in `registerMCPAndDebugRoutes`, build the external MCP server with `external.Provide(p.gateway.Dispatcher, "http://localhost:<port>", p.log)` and call `srv.RegisterRoutesAt(p.router, "/mcp")`.
- Wire its `Close()` into the existing cleanup chain.

**Modify** (frontend)
- New "External MCP" panel in Settings. Static — shows the URL, copy button, and ready-to-paste snippets for Claude Code, Cursor, Codex. No backend calls.
- File: `apps/web/components/settings/external-mcp/external-mcp-panel.tsx` + add to settings index.

**Cleanup** (deferred to its own commit)
- Remove `ports.MCP = 40429` (`internal/common/ports/ports.go`).
- Remove `McpServerEnabled`, `McpServerPort` from `internal/agentctl/server/config/config.go`.
- Remove the unused `40429` constant in `apps/cli/src/constants.ts`.

## Tool surface (v1)

Config-mode tools as currently registered in `(*Server).registerConfigWorkflowTools / Agent / Mcp / Executor / Task` plus `create_task_kandev` from kanban tools. Excluded:

- `ask_user_question_kandev` (no UI loop for external invocations).
- `create_task_plan_kandev`, `get/update/delete_task_plan_kandev` (require a live session).
- Internal-only: `clarification_timeout`.

Implementation: add a `ModeExternal` constant or a `WithoutAskQuestion + WithoutPlanTools` option. Simplest: introduce `ModeExternal` that registers exactly the tools above. Reuses every existing handler.

## Phases

1. **Refactor (no behavior change).** Move the package, rename imports, add `RegisterRoutesAt`. Add `ModeExternal` constant + branch in `registerTools()`. All existing tests pass.
2. **Backend wiring.** New `internal/mcp/external/` package with `DirectBackendClient` and `Provide`. Wire into `helpers.go`.
3. **Frontend Settings panel.** Static React panel with copy-to-clipboard for the three snippets.
4. **Tests.** Unit tests for `DirectBackendClient` (round-trips through dispatcher). Integration test that boots a Gin router with the provider and runs `tools/list` + a representative `tools/call` (`list_workspaces_kandev`) over real HTTP using `mcp-go` client.
5. **Cleanup.** Delete dead `40429`-related code.
6. **Verify.** `make fmt` then `make typecheck test lint` (backend); `pnpm typecheck test lint` (web).

## Risks / open points

- **mcp-go path prefix support.** Need to confirm `WithBaseURL("http://localhost:38429/mcp")` correctly emits the `/mcp/message?sessionId=…` URL in the SSE handshake. If not, register at root with a Gin `Group("/mcp")` and have the SSE handler be unaware of the prefix — Gin strips it before the wrapped handler sees it. Will validate during Phase 1.
- **Route conflicts.** Backend Gin router currently has `/ws`, `/health`, `/api/v1/*`, plus debug & pprof routes. `/mcp` is unused. Verified.
- **Direct dispatch concurrency.** `ws.Dispatcher` is read-only after registration; safe to call from many goroutines. No locking needed in `DirectBackendClient`.
- **Session-scoped tool handlers expecting non-empty session/task IDs.** Config-mode handlers don't inspect `sessionID`/`taskID` (they take params from the payload). Confirmed in `internal/mcp/handlers/config_*_handlers.go`. The `create_task_kandev` handler also takes everything from payload. Safe.
- **Logging noise.** The MCP server logs every tool call with `session_id`. With backend-mode it'll be empty — acceptable, but worth filtering or noting.

## Out of scope (per spec)

Auth, remote binding, session-scoped tools, per-workspace scoping. Token-based auth is the most likely follow-up.
