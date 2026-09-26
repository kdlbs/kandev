---
spec: docs/specs/integrations/requirements/external-question-answering.md
created: 2026-08-17
status: done
---

# Implementation Plan: External Question Answering

## Overview

The deliverable is a durable, authorized, idempotent clarification resolution path; the two MCP
tools are thin wrappers over it. Order follows that dependency: the `clarification_resolutions`
claim table and its repository first, then the durable bundle read queries the list tool needs,
then the single `ResolveBundle` service operation, then the two callers (REST endpoints, MCP
tools), then the frontend and catalog, then E2E. Nothing publishes a resume event until the claim
row exists, so every intermediate state leaves the product working: after task 04 the existing
overlay is already fixed (double-answer, authorization), and tasks 05+ add the external surface on
top of a path that is already correct.

---

## Backend

### Schema — `clarification_resolutions`

`apps/backend/internal/task/repository/sqlite/base_schema.go`

- New const `clarificationResolutionsSchemaDDL` and step `initClarificationResolutionsSchema`,
  appended to the `initSchema()` step table after `r.initTaskReviewSchema`:

```sql
CREATE TABLE IF NOT EXISTS clarification_resolutions (
    pending_id  TEXT PRIMARY KEY,
    task_id     TEXT NOT NULL,
    session_id  TEXT NOT NULL,
    status      TEXT NOT NULL,
    response    TEXT NOT NULL,
    resume      TEXT NOT NULL DEFAULT 'failed',
    resolved_by TEXT NOT NULL DEFAULT '',
    source      TEXT NOT NULL,
    resolved_at TIMESTAMP NOT NULL,
    FOREIGN KEY (task_id) REFERENCES tasks(id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS idx_clarification_resolutions_task
    ON clarification_resolutions(task_id);
```

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`: `migrateClarificationResolutions()`
  applies the same DDL through `r.migrate.Apply`, called from `runMigrations()`. The init step
  already covers existing databases (`CREATE TABLE IF NOT EXISTS` runs every boot), but the spec
  and `apps/backend/AGENTS.md` both require the logged, replay-tested migration entry; it is the
  path the replay test asserts against.
- `ensureMessageMetadataIndexes` (same file, `base_schema.go:92`) gains a third index so the
  bundle list query has an index-assisted pre-filter:
  `idx_messages_clarification_status ON task_session_messages(type, (<JSONExtract metadata.status>))`.
  Pre-filter only — membership is still the resolution-row check (spec L2/D4).

**`resume` is a column the spec's data-model table omits.** R8a requires a loser to receive the
value *recorded for the winning resolution*, which is only possible if it is persisted. The column
is written pessimistically at insert (`failed` for outcomes that publish, `not_applicable` for
cancel and R9 rejection) and upgraded to `published` after a successful publish, so a loser reading
between insert and update never sees a falsely optimistic value. Update the spec's table with this
column in the same change.

### Model and repository

- `apps/backend/internal/task/models/clarification_resolution.go` (new): `ClarificationResolution`,
  plus `ClarificationBundle` / `ClarificationBundleQuestion` used by the read side.
- `apps/backend/internal/task/repository/sqlite/clarification_resolution.go` (new):
  - `InsertClarificationResolution(ctx, *models.ClarificationResolution) (claimed bool, existing *models.ClarificationResolution, err error)`
    — `INSERT ... ON CONFLICT (pending_id) DO NOTHING`; when zero rows are affected, re-read and
    return the winner's row with `claimed=false`. The primary-key conflict *is* the CAS (spec step 4).
  - `UpdateClarificationResolutionResume(ctx, pendingID, resume string) error`.
  - `GetClarificationResolution(ctx, pendingID) (*models.ClarificationResolution, error)`.
- `apps/backend/internal/task/repository/sqlite/clarification_bundle.go` (new):
  - `ListUnresolvedClarificationBundles(ctx, models.ClarificationBundleFilter) ([]*models.ClarificationBundle, error)`
    — `task_session_messages` where `type='clarification_request'`, `LEFT JOIN tasks` /
    `LEFT JOIN workspaces`, `NOT EXISTS (SELECT 1 FROM clarification_resolutions ...)` (D4),
    `GROUP BY <JSONExtract metadata.pending_id>` with `MIN(created_at) AS bundle_created_at` (D1),
    `ORDER BY bundle_created_at ASC, pending_id ASC` (L6), keyset cursor
    `(bundle_created_at > ? OR (bundle_created_at = ? AND pending_id > ?))` (D6),
    `bundle_created_at >= ?` for `updated_since` (L8), `LIMIT ?+1` to derive `next_cursor` (L9).
    Owner scoping is in the query, not a post-filter, so pagination stays correct:
    `AND (t.workspace_id = '' OR w.owner_id = '' OR w.owner_id = ?)` when scoped (D5).
  - `GetClarificationBundle(ctx, pendingID) (*models.ClarificationBundle, error)` — the
    per-question rows plus `task_id` / `session_id`, used by `ResolveBundle` step 1.
  - Both build questions from message metadata by `question_index` ascending (L5), degrading to
    `question_id` with empty prompt/options when `question` metadata is unparseable (L15), and
    reporting each message's own `status`, treating absent/unrecognized as `pending` (D3, L12).
- `apps/backend/internal/task/repository/interface.go`: add the five methods to the message/task
  repository interface.

Both files use `dialect.JSONExtract` and `r.ro.Rebind`; every new dialect-sensitive method needs
the env-gated Postgres behavior test (`KANDEV_TEST_POSTGRES_DSN`) per `apps/backend/AGENTS.md`.

### Service — `ResolveBundle`

`apps/backend/internal/clarification/resolver.go` (new). One operation, both surfaces:

```go
type OutcomeKind string // "answered" | "rejected" | "cancelled"

type Outcome struct {
    Kind         OutcomeKind
    Answers      []Answer
    RejectReason string
    Source       string // "web" | "mcp" | "internal"
}

type Resolution struct {
    PendingID, TaskID, SessionID string
    Status                       Status
    Response                     *Response
    Resume                       string // "published" | "failed" | "not_applicable"
    ResolvedBy, Source           string
    ResolvedAt                   time.Time
}

func (r *Resolver) ResolveBundle(ctx context.Context, pendingID string, outcome Outcome) (*Resolution, bool, error)
```

`Outcome.Kind` replaces the spec's `rejected bool` because cancel joins the same claim (X1) and a
three-valued outcome is what the table's `status` column already stores; a bool plus a separate
cancel flag would let both be set.

Ordered steps, matching the spec exactly:

1. `GetClarificationBundle` → `task_id` / `session_id`; empty ⇒ `ErrBundleNotFound` (A5).
2. `taskAuthorizer.AuthorizeTaskAccess(ctx, taskID)`; any error ⇒ `ErrBundleNotFound` (A1–A3).
3. Validate against the bundle's question set (N6, N7, N8, N8a, N8b) — before the claim (N8c).
   `validateRespondAnswers` moves here from `handlers.go:348` and gains the option-ID check (N8)
   and the 2000-character caps (N8b) so both surfaces share one rule.
4. `InsertClarificationResolution`. Conflict ⇒ return the stored row with `claimed=false` and
   perform none of step 5 (R2, R3, R6).
5. Winner only, in this order: per-question durable updates in `question_index` ascending (R4, D2)
   → deliver to the in-memory waiter if present → publish. Any per-question update failure returns
   `ErrPartiallyApplied`, publishes nothing, and leaves the row in place (R5).

Event selection stays as today: live waiter ⇒ `clarification.primary_answered` (R7); no waiter ⇒
`clarification.answered` (R8); rejection with no waiter ⇒ `clarification.stale_dismissed` and no
resume (R9); cancel ⇒ `clarification.cancelled`, closing `CancelCh` when an entry exists (X3).
Publication success upgrades `resume` to `published`; failure leaves `failed` (R8a).

Dependencies are narrow interfaces declared in the package (`bundleReader`, `resolutionStore`,
`taskAuthorizer`, `MessageCreator`, `EventBus`, `*Store`) so `internal/clarification` does not
import `task/service`.

### REST endpoints

`apps/backend/internal/clarification/handlers.go`

- `Handlers` gains `resolver *Resolver`; `NewHandlers` / `RegisterRoutes` signatures extend, and
  `apps/backend/internal/backendapp/helpers.go:1135` passes the resolver built from
  `p.taskRepo` + `p.taskSvc.AuthorizeTaskAccess`.
- `httpRespond` collapses to: bind → `ResolveBundle` → map. Success is
  `200 {"success":true,"claimed":bool,"status":string,"response":{...},"resume":string}`.
  `ErrBundleNotFound` ⇒ `404 {"error":"clarification request not found"}` (A3 — same body as a
  fabricated ID, no task/session/workspace/question text). `ErrValidation` ⇒ 400.
  `ErrPartiallyApplied` ⇒ 500 naming the bundle (R5). The event-fallback branch
  (`handlers.go:326-336`), the `ErrAlreadyResponded` 409, and `applyAnswersToMessages` are deleted —
  their behavior now lives in the resolver.
- `httpCancelRequest` goes through `ResolveBundle` with `Kind: "cancelled"` and no longer 404s when
  the in-memory entry is gone (X2).
- `httpGetRequest` / `httpWaitForResponse` authorize before reading (A2), taking `task_id` from the
  in-memory `Request.TaskID` when present and from the durable messages otherwise; they keep their
  current 404-after-cleanup behavior otherwise.
- `httpCreateRequest` is untouched (A6).

### MCP tools

- `apps/backend/pkg/websocket/actions.go`: `ActionMCPListPendingQuestions = "mcp.list_pending_questions"`,
  `ActionMCPAnswerQuestion = "mcp.answer_question"`.
- `apps/backend/internal/mcp/server/server.go`: new group in `profileToolGroups()` —
  `{name: "external-questions", enabled: external, register: func(s *Server) { s.registerQuestionAnsweringTools() }}`
  (S1, S2). No system-prompt change (S4); `ask_user_question_kandev` stays off external (S3).
- `apps/backend/internal/mcp/server/question_handlers.go` (new): tool schemas and the two handlers
  that forward via `s.backend.RequestPayload`, which carries the HTTP request context and therefore
  the caller's identity.
- `apps/backend/internal/mcp/handlers/question_handlers.go` (new): `handleListPendingQuestions`
  (repo query + cursor encode/decode, L13 rejects an unparseable `updated_since` or `cursor`,
  L10 clamps `limit` to 1..200 default 50, L11 empty result is not an error) and
  `handleAnswerQuestion` (delegates to `ResolveBundle`, `claimed` in the payload, N3/N4/N5).
  Registered in `handlers.go` alongside the other `mcp.*` actions and wired through a new
  `SetClarificationResolver` setter (the constructor already takes 13 parameters).
- The gateway already refuses every `mcp.*` action over the raw browser WebSocket
  (`internal/gateway/websocket/client.go:174`), which is why these actions need no `pending_id`
  entry in `dispatch_scope.go`. Add a pinning assertion so that guard cannot be removed silently.

### `list_tasks_kandev` enrichment

`apps/backend/internal/mcp/handlers/handlers.go:553` — after `dto.FromTask`, batch
`taskSvc.BatchGetSessionsForTasks` + `taskSvc.GetPendingActionsForSessions` and stamp
`TaskPendingAction` / `PrimarySessionPendingAction` using the same projection rules as
`internal/task/handlers/task_http_handlers.go:382-417` (permission outranks clarification;
input-capable sessions only). Null when there is no blocked session (T2). Extract the shared
projection into `internal/task/dto` so the two surfaces cannot drift.

---

## Frontend

### `apps/web/hooks/domains/session/use-clarification-group.ts`

- Both `fetch` calls (`postClarificationBatch`, `postClarificationSkip`) send
  `credentials: "include"`, matching `lib/api/client.ts:80` (W1).
- Both parse the response body and return `{ state, claimed }`. A `claimed: false` success still
  closes the overlay (W2), but the caller skips the optimistic
  `safeApplyResolvedStatus(..., localAnswers)` — the winner's answers are not this client's, and
  writing local answers into metadata would show the user an answer that was never recorded. The
  status still flips so the overlay does not strand on `pending`.
- The `res.status === 409` branch stays as a compatibility path; the backend no longer emits it.

### API client

No new calls. The two MCP tools have no browser surface.

### State

No store slice changes.

---

## Tests

- **Claim is exactly-once under concurrency (R3).** Two goroutines calling `ResolveBundle` on one
  `pending_id` against a shared real SQLite database; assert exactly one `claimed=true`, one row,
  one event published, and the loser receiving the winner's payload.
  `apps/backend/internal/clarification/resolver_concurrency_test.go`. Real DB, not a mocked store —
  the claim is a database constraint.
- **Claim survives restart (R6).** Resolve, clear the in-memory store, resolve again; second call
  returns `claimed=false`. `resolver_test.go`.
- **Insert conflict returns the existing row.** Table-driven unit test over
  `InsertClarificationResolution`. `sqlite/clarification_resolution_test.go`, plus the env-gated
  Postgres variant and a `schema_replay_test.go` case for the new table.
- **Bundle listing.** Ordering (L6), cursor exclusivity and stability (L9, D6), `updated_since`
  intersection with cursor (L14), `limit` clamping (L10), empty result (L11), mixed-status bundle
  with no resolution row is returned (L12), resolved bundle is excluded (D4), unparseable question
  metadata degrades rather than hides (L15), `MIN(created_at)` as bundle time (D1).
  `sqlite/clarification_bundle_test.go`.
- **Authorization (A1–A5).** Table-driven over all four endpoints in both auth modes: enabled with
  a foreign workspace ⇒ 404 with a body byte-identical to a fabricated ID and no task/session/
  workspace/question text; disabled ⇒ unchanged behavior (A4 is the compatibility guarantee).
  `apps/backend/internal/clarification/handlers_authz_test.go`.
- **Validation before claim (N8c).** Each of N6/N7/N8/N8a/N8b rejected with no row written and the
  bundle still answerable. `resolver_validation_test.go`.
- **Partial apply (R5).** Message-update failure on question 2 of 2 ⇒ error, no event, row present,
  bundle not re-listed and not re-answerable. `resolver_test.go`.
- **Resume reporting (R8a).** Publish failure ⇒ `resume:"failed"` with the answer still recorded;
  cancel and stale-dismiss ⇒ `not_applicable`; loser carries the winner's value.
- **Cancel joins the claim (X1–X3).** Cancel with no in-memory entry succeeds; cancel racing an
  answer observes R2; `CancelCh` closes when an entry exists.
- **MCP tool surface.** `internal/mcp/server/server_test.go:1017` `TestServerModeExternal_ToolCount`
  35 → 37; `internal/mcp/server/external_integration_test.go:65` keeps its
  `NotContains "ask_user_question_kandev"` and gains positive assertions for both new names; a new
  test asserts neither tool registers on kanban/office/configuration (S2).
  `internal/mcp/handlers/handler_inventory_test.go` measures the delta dynamically — no change.
- **`mcp.*` stays off the browser WebSocket.** Pinning test on
  `internal/gateway/websocket/client.go`'s prefix guard.
- **`list_tasks_kandev` enrichment (T1, T2).** Handler test asserting both fields for a blocked
  session and null for an unblocked one.
- **Web hook.** `apps/web/hooks/domains/session/use-clarification-group.test.ts` gains cases for
  `credentials: "include"` on both POSTs and for `claimed:false` closing the overlay without
  applying local answers.
- **Catalog.** `apps/web/lib/settings/external-mcp-tools.test.ts` pinned count 30 → 37, drift note
  deleted, key-resolution test unchanged (C3).

---

## E2E Tests

- **Scenario:** GIVEN a clarification bundle in chat, WHEN the user submits and a concurrent
  answer has already won, THEN the overlay closes rather than stranding on `pending`.
  **File:** `apps/web/e2e/tests/chat/clarification.spec.ts` (extend; do not add a spec file).
  **What to verify:** the existing `**/api/v1/clarification/*/respond` route interception returns
  `200 {"success":true,"claimed":false,"status":"answered","resume":"published"}`; the carousel
  closes and the messages render resolved.

No E2E for the two MCP tools — they have no browser surface and are covered by Go tests at the MCP
server and handler layers.

---

## Verification Results

All nine task files' `## Results` sections are the source of truth for exact commands and
outcomes/counts per task; this section is the cross-task summary.

- **Backend gauntlet (this session, Wave 10):** `go test ./internal/clarification/...
  ./internal/mcp/... ./internal/workflow/... ./internal/backendapp/...
  ./internal/task/repository/sqlite/... ./internal/task/... ./internal/gateway/websocket/...` all
  pass. `gofmt -l` reports no files across every touched package. `go build ./...` succeeds.
  `golangci-lint run ./internal/clarification/... ./internal/mcp/... --timeout=5m` is clean.
  `make -C apps/backend test` (full suite) reported 3 failures, all in
  `internal/system/storage/workspaces` (`open dependency workspace root: not a directory`) — a
  package this branch never touches (confirmed via `git log --oneline -1 --` on that path) and a
  pre-existing macOS `t.TempDir()` symlink-resolution quirk (`/var/folders` vs
  `/private/var/folders`), not a regression from this feature.
- **Frontend gauntlet (this session, Wave 10 + prior segments):**
  `pnpm --filter @kandev/web test -- lib/settings/external-mcp-tools.test.ts
  hooks/domains/session/use-clarification-group.test.ts` passes. `pnpm run typecheck`, `pnpm run
  lint`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` from `apps/web` are all clean.
- **E2E gauntlet (this session, Wave 10):** `pnpm run build:e2e && pnpm e2e:raw
  e2e/tests/chat/clarification.spec.ts` passes all 31 cases. `pnpm run e2e:sleep-ratchet` is clean.
  `pnpm run lint:e2e-sleeps` with no path argument surfaces ~374 pre-existing, unrelated errors — a
  structural characteristic of `eslint.e2e-sleeps.config.mjs` (a narrow config meant to be
  path-scoped); scoped to the one touched file it is clean.
- Cleanup/teardown: only ephemeral `t.TempDir()` test databases and the standard Playwright
  artifact directory; no persistent or external side effects from any task in this feature.

---

## Implementation Waves And Parallel Candidates

```
Wave 1 (parallel candidates — user authorization required; disjoint files):
- [x] [task-01-resolutions-table](task-01-resolutions-table.md)
- [x] [task-06-list-tasks-pending-action](task-06-list-tasks-pending-action.md)
- [x] [task-08-external-tool-catalog](task-08-external-tool-catalog.md)

Wave 2:
- [x] [task-02-bundle-queries](task-02-bundle-queries.md)

Wave 3:
- [x] [task-03-resolve-bundle](task-03-resolve-bundle.md)

Wave 4:
- [x] [task-04-rest-endpoints](task-04-rest-endpoints.md)

Wave 5:
- [x] [task-05-mcp-question-tools](task-05-mcp-question-tools.md)

Wave 6:
- [x] [task-07-web-clarification-client](task-07-web-clarification-client.md)

Wave 7:
- [x] [task-09-e2e-duplicate-submit](task-09-e2e-duplicate-submit.md)
```

Tasks 04 and 05 both follow 03 and touch mostly disjoint trees, but both edit
`apps/backend/internal/backendapp/helpers.go` (route wiring at :1135, MCP handler wiring at :1535),
so they are sequential rather than parallel candidates. Task 08 is listed in wave 1 because its
catalog entries and locale keys are independent of the backend; its pinned count of 37 only becomes
*true* once task 05 lands, which is why task 05's checks include re-running the catalog test.

## Open Questions

- The spec's `clarification_resolutions` table omits the `resume` column that R8a's
  "loser carries the winner's recorded value" requires. This plan adds it and states the write
  ordering (pessimistic at insert, upgraded after publish). Update the spec's data-model table in
  the same change rather than leaving plan and spec disagreeing.
</content>
</invoke>
