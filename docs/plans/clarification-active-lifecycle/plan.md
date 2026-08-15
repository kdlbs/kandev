---
spec: docs/specs/clarification-active-lifecycle/spec.md
created: 2026-08-14
status: completed
---

# Implementation Plan: Active Clarification Lifecycle

## Overview

Make current-turn ownership the single definition of an active clarification, then use that
authority in workflow guarding, detachment, response validation, task-summary projection, chat
state, and task navigation. Black-box desktop/mobile regressions land and fail first. Backend
authority and summary convergence follow, then frontend current-turn and pending-session selection,
then production-build E2E returns green.

No schema migration, task-summary shape, new HTTP route, or direct mutation of the reported main
instance is required.

### Confirmed root cause and reproduction evidence

- `task_session_messages.metadata.status` retained older detached rows as `pending`, by design.
- `FindPendingClarificationMessagesBySessionID` scanned every historical turn. Every later turn
  completion found those rows, wrote `agent_disconnected=true` again, and republished
  `message.updated`.
- The workflow clarification guard used that same unbounded finder, so an inert historical row could
  block `on_turn_complete`.
- `GetPendingActionsBySessionIDs` already scoped task/session API projections to the latest surviving
  message turn. The live main instance therefore returned no flat pending action for two affected tasks
  while their persisted `status_summary.pending_action` still said `clarification`. Review also found
  that message deletion could move this provisional boundary backward despite a newer durable turn.
- The status projector restored that cached summary and tracked one request identity per session.
  Existing-summary hydration repaired only missing rows, so a stale pending field survived restart
  and reload.
- Frontend loaded-message discovery scanned all turns, allowing an older pending bundle to reappear
  after the visible newer bundle was rejected.
- One reported task had a genuine current clarification in a non-primary secondary session. The
  task-level icon was correct, but desktop and phone activation preferred remembered/primary session,
  hiding the question.

Read-only inspection found three projected clarification tasks: two stale projections with no
current-turn action and one genuine secondary-session action. Logs for the first case showed an old
bundle detached, a newer bundle rejected, and a later completion republishing the old pending row.

---

## Backend

### Current-turn clarification authority

- In `apps/backend/internal/task/repository/sqlite/message.go`, replace the historical pending finder
  with `FindActiveClarificationMessagesBySessionID`. Select the newest durable
  `task_session_turns` row per session, then restrict messages to that turn. Reuse this derivation in
  `GetPendingActionsBySessionIDs` so SQLite and PostgreSQL agree and message deletion cannot reactivate
  an older bundle.
- Treat missing clarification status as pending for parity with the existing compact projection.
  Return only current-turn `clarification_request` rows whose status is empty or `pending`.
- Rename the repository interfaces and focused mocks in
  `apps/backend/internal/task/repository/interface.go`,
  `apps/backend/internal/clarification/handlers.go`, and
  `apps/backend/internal/orchestrator/service.go`.
- In `apps/backend/internal/clarification/canceller.go`, use the active finder for detach and expiry
  fallback. Skip an already-terminal or already-detached row before writing or publishing. Count only
  bundles that changed so repeated completion is a semantic no-op.
- In `apps/backend/internal/orchestrator/clarification_guard.go`, guard only on active current-turn
  rows. Preserve fail-closed behavior when the authoritative query fails.
- In `apps/backend/internal/clarification/handlers.go`, validate the database-fallback response
  against active current-turn ownership before any fallback write. A detached current-turn answer
  succeeds only after one synchronously acknowledged orchestrator resume; a detached current-turn
  rejection persists without resuming the agent. Any answer or rejection for a superseded/terminal
  bundle returns conflict without a write or agent resume.
- Order durable turns identically by `started_at`, `created_at`, then `id` descending. Add an
  environment-gated PostgreSQL behavior test for the changed query. No migration or persisted-row
  rewrite.

### Authoritative summary convergence

- Add a `PendingActionLoader` to `apps/backend/internal/task/statussummary/projector.go`. It returns
  current actions keyed by input-capable session for one task.
- Wire the loader in `apps/backend/internal/backendapp/gateway.go` using the task repository's bounded
  session list and `GetPendingActionsBySessionIDs`; do not load transcripts.
- In `apps/backend/internal/task/statussummary/projector_events.go`, refresh pending state from the
  loader after message, permission, and clarification occurrences. Ordinary messages in a newer turn
  therefore clear superseded questions. Event ordering and `pending_id` memory no longer define the
  production projection.
- Refresh authoritative pending state while restoring a persisted projection and after compare-and-set
  rejection. On loader failure, retain last known state and surface the error; never optimistically
  clear a question.
- Rename/extend `HydrateMissingTaskStatusSummaries` in
  `apps/backend/internal/task/service/service_status_summary_rebuild.go` to reconcile existing summary
  pending fields as well as build absent rows. For an existing row, clone the latest stored summary,
  replace only `PendingAction`, advance revision/time on semantic change, and use bounded CAS
  reload/retry.
- Publish `task.status_summary.updated` after a successful existing-row repair so other connected
  clients converge. Preserve unrelated primary, activity, error, Git, PR, and queued-prompt fields.
- Update the task-list and boot callers in
  `apps/backend/internal/task/handlers/task_http_handlers.go` and
  `apps/backend/internal/backendapp/boot_state.go` to use the reconciler.

---

## Frontend

### Current-turn transcript discovery

- In `apps/web/lib/utils/pending-clarification.ts`, accept the newest durable turn ID from the existing
  `turns.bySession` state and bound clarification discovery to it. While turn history is unavailable,
  use the session's authoritative `pending_action` rather than treating the latest surviving message
  as current; retain latest-message fallback only for legacy sessions with no turn records.
- Build the selected bundle only from that turn and exact `pending_id`; terminal or older-turn rows
  cannot become the overlay fallback after a newer bundle is skipped.
- Extend `apps/web/lib/utils/pending-clarification.test.ts` with older-pending/newer-turn, same-turn
  bundle, missing-turn legacy, newer-rejected-bundle, and deleted-newer-turn-message cases.

### Pending-owner task navigation

- Add a pure resolver in `apps/web/components/task/task-select-helpers.ts` that selects the first
  (newest, because the API order is newest-first) input-capable session whose `pending_action`
  matches the task summary action. When a task advertises pending input but no matching input-capable
  owner exists, fail closed and navigate only to the task; never guess the remembered, primary, or
  first session. Clean tasks retain those normal session fallbacks.
- When a desktop task advertises pending input, always finish the existing session-list load before
  switching, even if the preferred session already has an environment mapping. Apply the same rule
  when the global sidebar starts from a non-task route. Keep the existing selection-token and
  external-navigation race guards, and invalidate deferred off-route selection when the pathname
  changes or its initiating sidebar callback unmounts.
- Reuse the resolver in
  `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.ts`; remove the synchronous
  primary fast path for a pending task, close the drawer only after the selected session/URL is set,
  and preserve failure fallback.
- Permission uses the same navigation rule because `TaskPendingAction` is shared. This plan does not
  change permission lifecycle semantics.

### Mobile design contract

- **Desktop outcome / phone entry:** both task-row surfaces lead to the session that owns the visible
  pending indicator. Phone entry stays the existing Tasks control and task-switcher drawer.
- **Nearest exemplar:** `session-task-switcher-sheet.tsx` remains the shipped inset bottom drawer with
  its existing safe-area, focus, dismissal, and internal-scroll behavior.
- **Hierarchy:** task row remains the only choice. No second question-specific control is added.
- **Presentation:** no visual, copy, geometry, or route redesign.
- **Shared logic:** one pure pending-owner resolver drives desktop and phone selection.
- **Touch behavior:** phone E2E uses `.tap()`, asserts drawer dismissal, task URL, active chat, and
  question visibility.

---

## Tests

- **What:** only newest-durable-turn clarification rows are active; older-turn pending rows are
  excluded, missing status remains pending only in that turn, deleting every newer-turn message does
  not reactivate history, and SQLite/PostgreSQL agree.
  - **Files:** `apps/backend/internal/task/repository/sqlite/message_active_clarification_test.go`,
    `apps/backend/internal/task/repository/sqlite/message_crud_coverage_test.go`, and
    `apps/backend/internal/task/repository/sqlite/message_pending_postgres_test.go`
  - **How:** real database tests create two turns with pending rows, delete every message in the newer
    turn, and verify the active finder and compact pending projection still exclude the older bundle.
- **What:** repeated detach does not update or publish an already-detached bundle; a new turn prevents
  old-row detachment; query failure keeps the workflow guard closed.
  - **Files:** `apps/backend/internal/clarification/canceller_test.go`,
    `apps/backend/internal/orchestrator/clarification_guard_test.go`, and
    `apps/backend/internal/orchestrator/event_handlers_agent_clarification_test.go`
  - **How:** focused fakes count writes/events, plus orchestrator service tests around the repository
    boundary.
- **What:** current-turn detached response still resumes or dismisses as appropriate, but a stale
  older-turn response returns conflict and initiates no agent resume.
  - **File:** `apps/backend/internal/clarification/handlers_test.go`
  - **How:** handler-to-repository integration with persisted bundles and an empty in-memory store.
- **What:** projector restore, newer ordinary message, terminal events, and CAS races converge to the
  authoritative pending map without losing another summary field.
  - **Files:** `apps/backend/internal/task/statussummary/projector_test.go` and
    `apps/backend/internal/task/service/service_status_summary_rebuild_test.go`
  - **How:** table-driven projector loader tests and real summary-store CAS tests, including loader
    failure and a competing revision.
- **What:** boot/task-list repair an existing stale summary, not only an absent row.
  - **Files:** `apps/backend/internal/backendapp/status_summary_boot_test.go` and the task HTTP handler
    tests nearest existing status-summary coverage.
  - **How:** persisted stale summary plus current-turn messages, then assert returned/persisted revision
    and complete replacement event.
- **What:** loaded transcript discovery follows durable turn identity through message deletion and
  preserves legacy no-turn data.
  - **File:** `apps/web/lib/utils/pending-clarification.test.ts`
  - **How:** pure Vitest message arrays.
- **What:** pending owner outranks remembered/primary selection on desktop and phone while clean tasks
  preserve existing preference and async race behavior.
  - **Files:** `apps/web/components/task/task-select-helpers.test.ts` and
    `apps/web/components/task/mobile/session-task-switcher-sheet-hooks.test.ts`
  - **How:** pure resolver cases plus asynchronous selection harnesses.

---

## E2E Tests

- **Scenario:** an older detached question survives, a newer question is asked and skipped, then the
  old question never resurfaces.
  - **File:** `apps/web/e2e/tests/task/sidebar-pending-question.spec.ts`
  - **What to verify:** current overlay closes, sidebar question icon clears, a later turn completion
    cannot re-arm it, and reload preserves the clear state.
- **Scenario:** a secondary session owns the task's only current clarification.
  - **File:** `apps/web/e2e/tests/task/sidebar-pending-question.spec.ts`
  - **What to verify:** clicking the task row from another task loads sessions, activates the secondary
    session instead of the clean primary, and displays the clarification.
- **Scenario:** the same secondary-session task is selected from the phone task drawer.
  - **File:** `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`
  - **What to verify:** touch activation selects the pending secondary, closes the inset drawer,
    updates `/t/:taskId`, shows the task title and question, and creates no horizontal overflow.
- **Scenario:** a detached current-turn question remains answerable.
  - **File:** existing `apps/web/e2e/tests/chat/clarification.spec.ts`
  - **What to verify:** the existing timeout/deferred-answer regression still resumes before a newer
    turn supersedes the question.

Use API helpers only to establish sessions/turns; all outcomes are asserted through task/chat UI and
survive reload. Managed runner builds production artifacts. No fixed sleeps or widened timeouts.

---

## Verification Results

All task gates passed:

- Backend authority: repository, clarification, and orchestrator packages passed; PostgreSQL parity
  remains env-gated and skipped locally without `KANDEV_TEST_POSTGRES_DSN`.
- Summary convergence: status-summary, task-service, backend-app, and focused handler tests passed.
- Frontend: 3 Vitest files / 68 tests, TypeScript, i18n, and frozen install passed.
- Final review remediation: backend repository, clarification, and orchestrator packages passed;
  changed backend lint reported zero issues. Six focused web files passed 108 tests, followed by
  TypeScript, zero-warning full lint, i18n check, and i18n ratchet.
- Dispatch-acknowledgement remediation: repository, clarification, and orchestrator packages passed;
  focused detached-resume tests and changed backend lint reported zero issues.
- Session-selection review remediation: six shared desktop/mobile selection and removal suites passed
  75 tests; typecheck, zero-warning full lint, and the i18n ratchet passed.
- Managed production E2E: Chromium 3/3 plus detached recovery 1/1; mobile Chrome 12/12.
- `git diff --check` passed. All runners used isolated test state and exited cleanly.

---

## Implementation Waves And Parallel Candidates

Execution remains sequential in the primary conversation.

Wave 1:

- [x] [task-01-clarification-regression-red](task-01-clarification-regression-red.md)

Wave 2:

- [x] [task-02-current-turn-backend-authority](task-02-current-turn-backend-authority.md)

Wave 3:

- [x] [task-03-summary-pending-convergence](task-03-summary-pending-convergence.md)

Wave 4:

- [x] [task-04-pending-owner-navigation](task-04-pending-owner-navigation.md)

Wave 5:

- [x] [task-05-clarification-regression-green](task-05-clarification-regression-green.md)

No task is marked parallel-safe. Tasks 02 and 03 share pending repository contracts; tasks 03 and 04
share projection semantics; task 05 spans all layers. Waves do not authorize subagents.

---

## Risks

- Durable-turn ordering must agree across dialects. Use the same `started_at`, `created_at`, and `id`
  ordering in scalar and batch queries, then run the env-gated PostgreSQL behavior test.
- Clearing on any ordinary message without checking its turn would recreate event-order bugs. The
  repository refresh, not message type, decides whether pending state changed.
- Read-time summary repair can race the live projector. Bounded CAS reload/retry and projector
  resynchronization must preserve newer unrelated summary fields.
- A stale tab can submit after another tab starts a new turn. The response guard must run before any
  fallback persistence or resume request.
- Pending task selection adds an async list request to a formerly synchronous fast path. Existing
  selection tokens, external-navigation guard, layout cleanup, URL ordering, and phone drawer failure
  fallback must remain intact.
- Historical rows remain `pending` in raw metadata. Any future consumer must use the active repository
  projection rather than inventing another all-history scan.

## Out of scope

- Direct cleanup/backfill of the main instance database.
- Schema or task-summary wire-shape changes.
- Clarification UI redesign or new user-facing copy.
- Historical notification retraction.
- Permission lifecycle changes beyond shared pending-owner navigation.
