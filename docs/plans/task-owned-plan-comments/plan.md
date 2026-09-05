---
created: 2026-09-02
status: completed
requirements:
  - REQ-TASKS-PLAN-COMMENTS-001
  - REQ-TASKS-PLAN-COMMENTS-002
  - REQ-TASKS-PLAN-COMMENTS-003
  - REQ-TASKS-PLAN-COMMENTS-004
system_design:
  - ../../specs/tasks/system-design/plan-comments.md
legacy_specs:
  - ../../specs/ui/requirements/plan-comment-drafts.md
  - ../../specs/ui/system-design/plan-comment-drafts.md
---

# Implementation Plan: Task-Owned Plan Comments

## Overview

Move pending plan comments from per-session `sessionStorage` into one
backend-owned collection for the task's current plan. First establish the
persisted CRUD and synchronization contract, then extend direct and queued
prompt admission to consume referenced comments atomically. After those server
boundaries exist, replace the frontend session projection, migrate legacy
browser drafts, and prove selected-session Send versus primary-session Run on
desktop and mobile.

The order prevents the UI from exposing a shared state that cannot yet survive
reload or guarantee its delivery semantics.

## Scope

### In scope

- Backend persistence, task authorization, optimistic versions, collection
  revisions, and WebSocket snapshots for current-plan comments.
- One shared Plan annotation projection and composer context item across all
  sessions in a task.
- Ordinary Send to the selected session and plan-comment Run to the current
  primary session.
- Server-owned prompt formatting and atomic consumption with both direct and
  queued acceptance.
- Safe one-time migration from session-scoped browser records.
- Desktop and phone behavior, localized errors, unit/integration tests,
  Playwright coverage, and the existing public task guide.

### Out of scope

- Changing ownership of diff, file, pull-request, walkthrough, or agent-message
  comments.
- Sent-comment history separate from the accepted transcript or queue row.
- Comments on historical plan revisions.
- Automatic prompt broadcast or ambient injection into every session.
- Redesigning the Plan editor, session tabs, or composer layout.

## Technical approach

### Persistence and synchronization

Add `models.TaskPlanComment` and `task_plan_comments`. Add
`comments_revision` to `task_plans`; every comment mutation increments it in
the same transaction. The table uses the existing task and current-plan rows
as cascade owners, keeps caller UUIDs for idempotent migration, and uses an
integer `version` for edit and delivery conflicts.

Extend `repository.PlanRepository` and `PlanService` with comment list/create/
update/delete operations. Add `task.plan.comments.*` actions in
`apps/backend/pkg/websocket/actions.go`, handlers beside
`task_plan_handlers.go`, and `task.plan.comments.changed` publication through
the task WebSocket broadcaster. List, mutation response, and event all carry a
complete `{task_id, plan_id, revision, comments}` snapshot.

### Prompt admission and routing

Add optional `plan_comment_refs` to `message.add` and `message.queue.add`.
Callers submit IDs and versions, not formatted comment text. The server loads
the authoritative rows, emits the current `### Plan Comments` Markdown shape,
and stores the expanded prompt.

Extend the existing direct user-message and durable queue transactions to
validate and conditionally delete the references while inserting the target
row and incrementing the collection revision. A leaf transaction helper shares
that logic between task-message and message-queue repositories. Existing
`client_message_id` provides direct replay; comment-bearing queue additions
gain `client_queue_id` and bypass immediate auto-merge so replay can resolve an
exact durable entry.

Plan-comment Run derives the task primary rather than the selected tab. Its
direct or queue request sets `require_primary_session`; the repository verifies
that designation under the task lock before accepting. Busy Run creates a
distinct queue entry instead of appending into unrelated text. Other comment
types retain the selected-session `useRunComment` behavior.

### Frontend task projection

Move `PlanComment` out of the session-indexed comments state and add a
task-keyed snapshot to task-plan state. Add a plan-comment API client and
WebSocket handler. `usePlanComments(taskId)` loads and reconciles complete
snapshots by plan ID and revision.

Update `TaskPlanPanel`, `usePlanComments`, `usePendingComments`,
`useChatPanelState`, and both structured and passthrough submit paths to consume
the task snapshot. Every session composer renders the same count chip and
submits the IDs and versions it displayed. The chip opens Plan and has no
remove action; task-wide edit/delete stays on each annotation.

Stop formatting plan comments in `buildSubmitMessage` and
`buildDocumentContext`; the server owns their canonical text. Keep client-side
formatting and clearing for all other comment types. Make selection Popover and
mobile Drawer mutations await backend acknowledgement, preserving text and
selection with inline localized failure feedback.

### Legacy migration

Once a task's current plan and sessions are known, scan
`kandev.comments.<sessionId>` for its session IDs. Upload only plan records with
their existing UUIDs, then remove each acknowledged plan record from its old
payload. Leave failed rows and every other comment source intact. Gate
comment-bearing Send and Run until migration resolves; a missing current plan
keeps the browser data for later recovery.

### Public documentation

Update the plan-comment paragraph in `docs/public/tasks-and-workflows.md` to
state that Add creates task-level pending context, all session composers show
it, ordinary Send addresses the selected session, and Run addresses the current
primary.

## Tests

| Acceptance criteria                    | Evidence                                                                                                                                                      |
| -------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `AC-TASKS-PLAN-COMMENTS-001.1` to `.7` | Repository/service/handler tests for persistence, snapshots, task and plan cascades, session independence, orphan cleanup; frontend hook and Plan-panel tests |
| `AC-TASKS-PLAN-COMMENTS-002.1` to `.4` | Composer-state, context-item, structured-submit, and passthrough-submit unit tests                                                                            |
| `AC-TASKS-PLAN-COMMENTS-002.5` to `.7` | Direct-message and queue repository integration tests for conditional consumption, rollback, concurrent losers, and untouched later comments                  |
| `AC-TASKS-PLAN-COMMENTS-003.1` to `.6` | `use-run-comment` tests plus backend primary-guard, busy queue, missing/terminal primary, and stale-primary tests                                             |
| `AC-TASKS-PLAN-COMMENTS-004.1` to `.4` | Migration hook tests for multi-session records, idempotent retry, selective cleanup, failure, and send gating                                                 |

Tests added around the persistence and admission boundaries must run against
SQLite and the repository's Postgres schema/replay harness where that harness
owns dialect parity.

## E2E tests

- `apps/web/e2e/tests/session/task-plan-comments.spec.ts` creates one task with
  a primary and secondary session. It proves the same annotation and context
  item survive tab switches and reload; ordinary Send from the secondary lands
  only in that session; a later Run from the secondary lands only in the
  primary; each accepted path removes exactly its submitted comments task-wide.
  Covers `AC-TASKS-PLAN-COMMENTS-001.1` to `.3`,
  `AC-TASKS-PLAN-COMMENTS-002.1` to `.6`, and
  `AC-TASKS-PLAN-COMMENTS-003.1` to `.3` and `.6`.
- `apps/web/e2e/tests/session/mobile-task-plan-comments.spec.ts` uses the
  `mobile-chrome` project and the session picker plus Plan Drawer to prove the
  shared annotation/context, selected-session Send, primary-session Run, 44 px
  actions, and no horizontal overflow. It also seeds plan comments in two
  legacy session-storage payloads, preserves a colocated diff comment, and
  proves each plan comment migrates once into shared task context. Covers
  `AC-TASKS-PLAN-COMMENTS-001.7` and the responsive form of the delivery
  criteria plus `AC-TASKS-PLAN-COMMENTS-004.1` to `.4`.

## Work orders

- [x] [Task 01: Persist shared plan comments](task-01-persist-shared-plan-comments.md)
- [x] [Task 02: Consume comments during prompt admission](task-02-consume-comments-during-prompt-admission.md)
- [x] [Task 03: Project comments across task sessions](task-03-project-comments-across-task-sessions.md)
- [x] [Task 04: Migrate legacy browser drafts](task-04-migrate-legacy-browser-drafts.md)
- [x] [Task 05: Prove responsive multi-session behavior](task-05-prove-responsive-multi-session-behavior.md)
- [x] [Task 06: Document plan-comment routing](task-06-document-plan-comment-routing.md)

## Verification results

- Backend: full `make test` and `make lint` passed; focused task and queue
  admission packages also passed after final changes.
- Frontend: TypeScript, ESLint, i18n catalog/ratchet checks, 128 focused tests,
  and the full 1,697-file Vitest suite (14,541 passed, 4 skipped) passed.
- Browser: production-build Chromium and `mobile-chrome` Playwright scenarios
  both passed, including selected-session Send, primary-session Run, shared
  consumption, reload, migration, retained diff context, touch targets, and
  overflow checks.
- Specifications and public docs: spec lint, all 61 docs validator tests, and
  validation of all 42 published pages passed.

## Implementation waves and parallel candidates

```text
Wave 1:
- Task 01

Wave 2:
- Task 02

Wave 3:
- Task 03

Wave 4:
- Task 04
- Task 06 (parallel-safe with Task 04)

Wave 5:
- Task 05
```

Implementation remains sequential unless the user explicitly authorizes
subagents. Task 06 is the only parallel-safe pair: it changes public Markdown
while Task 04 changes frontend migration code and tests.

## Risks

- Direct messages and queued prompts currently live behind different
  repository interfaces. Their shared transaction helper must preserve the
  global task-before-session lock order and both dialects' replay behavior.
- Existing `message.queue.append` auto-merges content. Plan Run must use an
  independently identifiable queue row or transport retries can no longer
  prove whether a comment was accepted.
- Tiptap positions are advisory across plan revisions. Selected-text fallback
  and tagged projection transactions must remain intact when backend snapshots
  replace session-local arrays.
- Session-storage migration is tab-local and can be interrupted. Cleanup must
  happen per acknowledged UUID, never by deleting an entire session key.
- A stale or failed comment load must fail closed for comment-bearing delivery
  without blocking comment-free messages on tasks that have no plan.
