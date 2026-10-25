---
spec: docs/specs/agents/requirements/cursor-subagent-metadata.md
created: 2026-08-20
status: in_progress
---

# Implementation Plan: Cursor Subagent Metadata

## Overview

Cursor delivers its subagent's prompt, model, description, and agent id over a
non-standard agent-to-client request, `cursor/task`, which Kandev currently
rejects with `-32601`. This plan (1) opens a seam so the ACP client can accept a
non-`_`-prefixed extension method, (2) parses `cursor/task` and correlates it to
the standard subagent `tool_call` by `toolCallId`, merging its fields onto the
existing `SubagentTaskPayload`, and (3) maps Cursor's `isBackground` completion
flag onto the payload's existing `IsAsync` so the current "background" chip
renders. The frontend already renders model, prompt (fallback), and the
background chip, so no frontend change is required for the golden path.

---

## Resolved decision and implemented seam

The upstream ACP client routes inbound client extension handling only for
method names starting with `_`, so `cursor/task` would fall through to the
generated `ClientSideConnection.handle` switch and return `-32601`.

The implementation that landed widens that seam in the pinned ACP fork instead
of proxying it in-repo: the fork routes any unrecognized inbound client
*request* to the client's `ExtensionMethodHandler`, and Kandev keeps the
client-owned allow/deny policy in
`apps/backend/internal/agentctl/server/acp/client.go`. The adapter registers a
`CursorTaskHandler` via `acpclient.WithCursorTaskHandler` and passes the client
straight to `acp.NewClientSideConnection` (`adapter.go`), so there is no
repo-local connection proxy. The relaxation is inbound-accept only; the outbound
`CallExtension`/`NotifyExtension` `_`-prefix contract is unchanged.

---

## Backend

### Area 1 — client extension seam (Task 01)
Kandev implements `HandleExtensionMethod(ctx, method, params)` on
`acpclient.Client` (`apps/backend/internal/agentctl/server/acp/client.go`),
returning an empty result for `cursor/task` and `NewMethodNotFound` otherwise.
The pinned ACP fork routes any unrecognized inbound client request to that
handler, so no repo-local connection helper is needed. A `CursorTaskHandler`
option (mirroring `WithUpdateHandler`) hands the raw params to the adapter.

### Area 2 — parse + correlate cursor/task (Task 02)
New `dialect_cursor.go` in `apps/backend/internal/agentctl/server/adapter/transport/acp/`: parse the `cursor/task` params (`agentId`, `description`, `model`, `prompt`, `toolCallId`; `subagentType` is an object and is ignored) defensively over untyped maps. The adapter keeps a per-session `map[toolCallId]cursorTaskMeta` (bounded, cleared on session teardown). On either the `cursor/task` request or the subagent `tool_call` (whichever is second), merge onto the `SubagentTaskPayload` via the existing fill-if-present rule (extend `applySubagentResult` or a sibling `applyCursorTaskMeta`). Wire the adapter's `CursorTaskHandler` in `adapter.go` next to `enqueueACPUpdate`.

### Area 3 — background flag (Task 02, same change)
Extend `cursorSubagentResult` (`subagent.go:349`) to read `rawOutput.isBackground` and set `res.IsAsync = true` when it is `true`. This reuses `SubagentTaskPayload.IsAsync` and the frontend background chip; no new payload field.

---

## Frontend

> No change required for the golden path. `model` already renders as a chip
> (`subagent-meta.ts:85`), `is_async` already renders the "background" chip
> (`subagent-meta.ts:56`), and `prompt` already renders as the card body
> fallback when there is no `result_text` and no children
> (`tool-subagent-message.tsx`). The `SubagentTaskPayload` TS type
> (`apps/web/components/task/chat/types.ts`) already carries every field this
> feature populates. Confirm in Task 04 (E2E/manual) and only add rendering if a
> gap is observed.

---

## Tests

- **Extension dispatch (Task 01):** `cursor/task` returns a success result, not
  `-32601`. File: `apps/backend/internal/agentctl/server/acp/client_test.go`
  (table-driven: `cursor/task` → ok; unknown `foo/bar` → method-not-found).
- **cursor/task parse (Task 02):** parses `agentId`/`description`/`model`/
  `prompt`; ignores object `subagentType`; tolerates missing fields. File:
  `apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_cursor_test.go`.
- **Correlation both orders (Task 02):** request-before-tool_call and
  tool_call-before-request both merge onto one payload; unmatched entry is
  dropped at teardown. File: `..._cursor_test.go` /
  `subagent_test.go`.
- **isBackground (Task 02):** completion `rawOutput.isBackground:true` sets
  `IsAsync`; `false`/absent leaves it unset. File: `subagent_test.go`.
- **No-regression (Task 02/03):** existing subagent tests for Claude/OpenCode/
  Auggie still pass unchanged.

---

## E2E Tests

> No new Playwright spec. `cursor/task` requires a live Cursor login and cannot
> run in CI. Task 04 is a manual `acpdbg`/live-session verification against a
> real Cursor subagent, recording the rendered card (prompt, model, background).

---

## Verification Results

- `cd apps/backend && go test ./internal/agentctl/server/acp/... ./internal/agentctl/server/adapter/transport/acp/... ./internal/agentctl/server/utility/...` passed after adding a real wire-path test for inbound `cursor/task` handling.
- `cd apps && pnpm install --frozen-lockfile` passed in the worktree so focused frontend tests could run.
- `cd apps && pnpm --filter @kandev/web exec vitest run components/task/chat/messages/subagent-meta.test.ts components/task/chat/messages/tool-subagent-message.test.tsx` passed 41 tests across 2 files.
- No frontend code change was required: the existing subagent metadata chip/body renderers already display `model`, `is_async`, and `prompt` once the backend populates them.
- Manual live verification against a real Cursor-authenticated session is still pending (Task 04).

---

## Implementation Waves And Parallel Candidates

```text
Wave 1:
- [x] [task-01-sdk-extension-seam](task-01-sdk-extension-seam.md)

Wave 2:
- [x] [task-02-parse-correlate-cursor-task](task-02-parse-correlate-cursor-task.md)

Wave 3 (verification):
- [x] [task-03-frontend-confirm](task-03-frontend-confirm.md)   (parallel-safe with Task 04)
- [ ] [task-04-manual-live-verify](task-04-manual-live-verify.md)
```

Sequential by default. Task 02 depends on Task 01's `CursorTaskHandler` seam.

---

## Open Questions
- None. The one architectural question (Option A vs Option B) is resolved by
  [ADR-2026-08-20-acp-client-non-underscore-extension-methods](../../decisions/2026-08-20-acp-client-non-underscore-extension-methods.md)
  (Option A). Task 01 is unblocked.
