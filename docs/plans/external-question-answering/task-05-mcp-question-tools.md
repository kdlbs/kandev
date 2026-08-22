---
id: "05-mcp-question-tools"
title: "list_pending_questions_kandev and answer_question_kandev"
status: done
wave: 5
depends_on: ["03-resolve-bundle", "02-bundle-queries"]
plan: "plan.md"
spec: "../../specs/external-question-answering/spec.md"
---

# Task 05: `list_pending_questions_kandev` and `answer_question_kandev`

The visible half: two MCP tools on the external surface only. Both are thin — the list tool wraps
task 02's query, the answer tool wraps task 03's `ResolveBundle`. No second implementation of
either.

- **Acceptance:**
  1. Both tools appear on `SurfaceExternal` and on no other surface (S1, S2);
     `ask_user_question_kandev` remains absent from external (S3); no system-prompt change (S4).
  2. `list_pending_questions_kandev` returns bundles with `pending_id`, `task_id`, `session_id`,
     `created_at` (RFC3339 UTC), `age_seconds` (floored at 0, D7), `context`, and `questions`
     carrying the exact `question_id` / `option_id` identifiers the answer tool accepts (L3, L4).
     An empty result is `{"questions":[],"total":0,"next_cursor":""}`, not an error (L11).
     An unparseable `updated_since` or `cursor` is a validation error naming the argument (L13).
  3. `answer_question_kandev` returns `claimed:true` with the recorded status and normalized
     response on winning, and `claimed:false` with the **winner's** status and response on losing,
     without reporting an error (N3, N4). An inaccessible `pending_id` produces the same not-found
     text as a nonexistent one (N5).
  4. `TestServerModeExternal_ToolCount` asserts 37 and passes.

- **Verification:**
  ```
  cd apps/backend && \
    gofmt -l internal/mcp pkg/websocket && \
    go test -race ./internal/mcp/... ./internal/gateway/websocket/... && \
    go build ./... && \
    golangci-lint run ./internal/mcp/... --timeout=5m
  ```
  Then confirm the catalog pin from task 08 is now true:
  ```
  cd apps && pnpm install --frozen-lockfile && \
    pnpm --filter @kandev/web test -- lib/settings/external-mcp-tools.test.ts
  ```

- **Files likely touched:**
  - `apps/backend/pkg/websocket/actions.go` (`ActionMCPListPendingQuestions`,
    `ActionMCPAnswerQuestion`)
  - `apps/backend/internal/mcp/server/server.go` (new `external-questions` group in
    `profileToolGroups()` at `:816`)
  - `apps/backend/internal/mcp/server/question_handlers.go` (new — tool schemas and dispatch)
  - `apps/backend/internal/mcp/handlers/question_handlers.go` (new — `handleListPendingQuestions`,
    `handleAnswerQuestion`, cursor encode/decode)
  - `apps/backend/internal/mcp/handlers/handlers.go` (register both actions near `:373`; add
    `SetClarificationResolver`)
  - `apps/backend/internal/backendapp/helpers.go:1535` (wire the resolver into `mcpHandlers`)
  - `apps/backend/internal/mcp/server/server_test.go:1017` (35 → 37, and update the arithmetic
    comment)
  - `apps/backend/internal/mcp/server/external_integration_test.go:65` (keep the
    `NotContains "ask_user_question_kandev"` assertion; add positive assertions for both new names)
  - `apps/backend/internal/mcp/handlers/question_handlers_test.go` (new)
  - `apps/backend/internal/gateway/websocket/client_mcp_guard_test.go` (new — pin the `mcp.` prefix
    refusal)

- **Dependencies:** 03, 02.
- **Parallelism:** `sequential` — shares `apps/backend/internal/backendapp/helpers.go` with task 04.

- **Inputs:**
  - Spec L1–L15, N1–N9, S1–S4, § *Permissions* (including why the MCP scope resolver is the wrong
    authorization input), § *Verification notes* → *Tests that need deliberate updating*.
  - Plan § *Backend → MCP tools*.
  - Pattern to copy end to end: `add_task_dependency_kandev` — schema in
    `server.go:1191`, handler in `server/dependency_handlers.go`, backend handler in
    `handlers/dependency_handlers.go`, action constant in `pkg/websocket/actions.go:392`.
  - Authorization comes from the request context that `DispatcherBackendClient.RequestPayload`
    (`server/dispatcher_backend_client.go:41`) forwards into `Dispatch`; do not add a
    `pending_id` case to `internal/gateway/websocket/dispatch_scope.go` — `mcp.*` actions are
    refused over the raw browser WebSocket at `client.go:174`, which the new pinning test locks in.
  - `handler_inventory_test.go` measures the dispatcher delta dynamically and needs **no** change.
  - The cursor is opaque to callers: encode `(created_at, pending_id)` and reject a corrupt value
    rather than ignoring it (L13).

- **Output contract:** summary, files changed, tests run with counts, blockers, risks, and the
  task/plan status update in the same conversation.

## Results

Implemented across commits `c951a6b03`, `d05d9385c`, `4da6da51a`, plus the companion orchestrator
guard widening in `55420d89e` (D4a — see task 06's note; no dedicated task file claims that change,
so it is recorded here and against Wave 6 of the board's task list). Added
`list_pending_questions_kandev` and `answer_question_kandev` to the `external-questions` group in
`profileToolGroups()` (`server.go`), their tool schemas in `server/question_handlers.go`, backend
handlers in `mcp/handlers/question_handlers.go` (cursor encode/decode, `limit` clamping, `age_seconds`
floored at 0), the `ActionMCPListPendingQuestions`/`ActionMCPAnswerQuestion` action constants, and
wired the resolver into `mcpHandlers` via a new `SetClarificationResolver` setter.
`TestServerModeExternal_ToolCount` updated 35 → 37 with its arithmetic comment; `
external_integration_test.go` keeps `NotContains "ask_user_question_kandev"` and gained positive
assertions for both new tool names; a new `client_mcp_guard_test.go` pins the `mcp.` prefix refusal
on the raw browser WebSocket.

Verified in this session's final gauntlet (Wave 10): `go test ./internal/mcp/...
./internal/gateway/websocket/...` passes. `gofmt -l` reports no files; `go build ./...` succeeds;
`golangci-lint run ./internal/mcp/... --timeout=5m` is clean. The catalog-pin confirmation
(`pnpm --filter @kandev/web test -- lib/settings/external-mcp-tools.test.ts`) was re-run after task
08 landed and passes at 37.

No external side effects. Authorization for both tools flows from the same request-context identity
the REST endpoints use (task 03/04), not the MCP scope resolver.
</content>
