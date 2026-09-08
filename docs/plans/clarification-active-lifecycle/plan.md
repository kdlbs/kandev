---
spec: docs/specs/tasks/requirements/clarification-active-lifecycle.md
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

All locally available task gates passed; environment-gated parity also passed in PR CI:

- Backend authority: repository, clarification, and orchestrator packages passed. PostgreSQL parity
  was skipped locally without `KANDEV_TEST_POSTGRES_DSN` and passed in PR CI.
- Summary convergence: status-summary, task-service, backend-app, and focused handler tests passed.
- Frontend: 3 Vitest files / 68 tests, TypeScript, i18n, and frozen install passed.
- Final review remediation: backend repository, clarification, and orchestrator packages passed;
  changed backend lint reported zero issues. Six focused web files passed 108 tests, followed by
  TypeScript, zero-warning full lint, i18n check, and i18n ratchet.
- Dispatch-acknowledgement remediation: repository, clarification, and orchestrator packages passed;
  focused detached-resume tests and changed backend lint reported zero issues.
- Restart-window remediation: unpublished successor reservations persist exact clarification recovery
  identity, startup restores empty unattempted reservations before executor reconciliation, and
  message-backed reservations become accepted authority. Focused SQLite/startup tests passed;
  PostgreSQL parity is environment-gated.
- Session-selection review remediation: six shared desktop/mobile selection and removal suites passed
  75 tests; typecheck, zero-warning full lint, and the i18n ratchet passed.
- Rejected-load review remediation: both primary-session and sessionless pending-selection fallbacks
  revalidate click-time summary ownership; the focused race suite passed 14 tests.
- Review remediation: agentctl acceptance is not reported as success until the reserved successor is
  durably published; post-acceptance publication failure remains terminal and rollback-ineligible.
  Clarification overlay and transcript filtering now share the same optional authority scope.
- At-most-once remediation: immediately before external executor dispatch, a durable attempt marker
  makes restart recovery fail closed across the acceptance/publication crash window. Marker-write
  failure rolls back before dispatch; known synchronous rejection remains safely rollback-eligible.
- Ready-race remediation: live reservations broadcast acceptance or rollback. An overlapping ready
  event waits for that result and revalidates prompt generation, preventing a delayed predecessor from
  completing the successor or running workflow completion against it.
- Final bot-review remediation: clarification persistence operations use fresh bounded contexts;
  post-acceptance transport failures run cleanup without reopening the answer; reservation rollback
  lets the still-authoritative predecessor ready event complete; startup recovery errors fail before
  prompt admission; and stale pending-owner loads still open the requested task on desktop and mobile.
  Full clarification/orchestrator tests, ten race-enabled focused repetitions, changed-code backend
  lint, 55 focused web tests, zero-warning focused web lint, typecheck, and i18n ratchet passed.
- Follow-up bot review: clarification detach database failures are error-level diagnostics, and pause
  cancellation treats a wired no-active-turn snapshot as explicit authority. A first turn created
  during detachment is preserved, matching the existing successor-turn protection; both focused races
  passed ten repetitions, followed by the full orchestrator and clarification suites and zero-issue
  changed-code backend lint.
- Later bot review: failed reserved-turn rollback keeps its live reservation unresolved, blocking ready
  handling and prompt admission until restart recovery; clarification detach and expiry repository
  operations now use fresh detached 30-second contexts. Both focused regressions passed ten times.
- Final Codex review: task-session HTTP and WebSocket lists fail closed when authoritative pending-action
  projection fails. Desktop and mobile async selection also remain inert when the selected task
  projection disappears, avoiding navigation to a deleted task. The dual-transport handler regression,
  57 focused web tests, zero-warning focused web lint, typecheck, i18n ratchet, and changed-code Go lint
  passed.
- Claude review remediation removes a dead clarification lookup, narrows handler and cancellation
  repository interfaces, keeps caller metadata unchanged across any partial completion failure, and
  documents the transaction-local claim timestamp. Durable-turn ordering coverage now includes the
  created-time tie-break alongside existing load-state, start-time, ID, and nanosecond cases.
- Final Codex review returns restored clarification rows through the repository/service boundary and
  publishes their pending state after commit, closing the interleaved task-summary refresh race. The
  durable selection overview now matches task-only fallback for changed summaries and inert handling
  only when the task projection disappears. Broader clarification-focused handler, repository, and
  orchestrator tests passed, followed by zero-issue changed-code Go lint.
- CodeRabbit and Claude follow-up align recovery's attempted-flag decoder with tolerant SQL semantics
  and pin the generationless-ready drop after a reservation rollback. Boolean, string, and numeric flag
  encodings plus adjacent recovery/ready races passed ten focused repetitions; broader recovery and
  ready suites passed, followed by zero-issue changed-code Go lint.
- Latest review remediation bounds synchronous detached resume, acknowledges restored pending state
  through the live projector before promising retryability, removes wall-clock polling from repository
  timestamp tests through an injected clock, and clarifies why every successor-turn message shares the
  PostgreSQL session lock.
- New bot-review remediation uses bounded detached rollback after admission-time context cancellation,
  normalizes pending/attempted flag predicates across SQLite and PostgreSQL, surfaces exhausted pending
  summary CAS retries, and makes atomic clarification bundle operations a compile-time repository
  contract. Focused race checks and all six affected backend package suites passed; the environment-gated
  PostgreSQL predicate case skipped locally and remains enabled in CI.
- Follow-up review bounds terminal bundle publication with the same fresh persistence context. The
  cancelled-caller regression, full clarification suite, and focused lint passed.
- Latest Codex review makes superseded forced-load aborts inert across desktop and phone and records
  initial task-projection presence so deletion also invalidates legacy pending-action selections. This
  changes shared selection state only, not mobile composition or touch behavior; focused desktop/mobile
  unit coverage satisfies mobile parity without another Playwright case.
- Four focused task-selection suites passed 61 tests; full web lint, typecheck, and the i18n ratchet
  passed.
- Late Claude review normalizes the detached-message guard across boolean, numeric, and string truthy
  encodings. Restored clarification rows now publish even when synchronous summary acknowledgement is
  unavailable or fails, while the acknowledgement error still reaches the caller.
- Full SQLite repository and task-service suites passed in 49s and 129s; changed-code Go lint reported
  zero issues.
- Later Codex/Claude review rehydrates keyed Git observations after a nil-aggregate restart and rejects
  malformed clarification recovery message-ID arrays instead of silently restoring a subset. Review
  wording now distinguishes hidden unattempted reservations from authoritative attempted or
  message-backed turns and preserves terminal clarification siblings during rejection.
- Full status-summary and SQLite repository suites passed; changed-code Go lint reported zero issues.
- Final review follow-up applies the same nil-aggregate restart and CAS-rebase guarantee to keyed pull
  requests, makes unpublished-turn startup recovery a compile-time turn-repository requirement, and
  adds PostgreSQL coverage for string truthy detached flags. Go RFC3339 parsing coverage confirms the
  fixed nanosecond wire format remains compatible with existing Go consumers.
- The focused regressions and full status-summary, SQLite repository, task-service, and backend-app
  suites passed; changed-code Go lint reported zero issues. The PostgreSQL case skipped locally without
  `KANDEV_TEST_POSTGRES_DSN` and remains enabled in PostgreSQL CI.
- Final bot-review remediation rehydrates configured source observations before the first summary row,
  keeps a durably restored clarification retryable when live convergence fails, and replaces mobile's
  module-global selection sequence with one controller per mounted sheet. Focused Go suites, 42 web
  selection tests, typecheck, zero-warning focused lint, and the i18n ratchet passed.
- Latest Codex review fails orchestrator startup when its turn service is absent instead of silently
  bypassing reservation recovery, and aligns the remaining stale-summary scenario with task-only phone
  navigation. The focused startup regressions and full orchestrator suite passed.
- Follow-up Codex review invalidates the phone task-sheet selection controller on unmount so a deferred
  load cannot mutate or close a replacement tablet sheet. The 48-test focused selection run passed.
- Fresh Claude review wires the existing expiry path into every accepted terminal-session transition.
  Fresh Codex review propagates status-summary repair failures and removes each failed stale summary from
  task-list and boot responses so their authoritative coarse pending action remains visible. Focused
  regressions, all five affected backend suites, and changed-code lint passed.
- Managed production E2E: Chromium 3/3 plus detached recovery 1/1; mobile Chrome 12/12.
- Current-head review remediation keeps unpublished attempted reservations out of client turn history
  and propagates watchdog cancellation through fallback database and prompt work. Focused SQLite and
  orchestrator regressions passed; PostgreSQL parity remains environment-gated in CI.
- Follow-up review adds scheduled positive and superseded watchdog coverage and makes primary and
  detached clarification delivery share one claimed-bundle turn-identity rule.
- Late CodeRabbit review restores the planned deterministic tie-breaks to ascending turn-history reads
  and adds same-timestamp batch-list coverage.
- Latest CodeRabbit review passes terminal session state through every loaded-message pending-input
  selector, so stale current-turn clarification rows cannot re-arm task or session UI after completion
  or cancellation. The 55 focused web tests, typecheck, zero-warning focused lint, and i18n ratchet
  passed.
- Fresh Claude review makes cancellation and live response delivery choose one winner under the pending
  entry lock. A cancellation that wins after response lookup now returns not-found immediately, while
  an already-resolved response keeps its durable confirmation path. The deleted-task mobile selection
  comment was rejected because the reviewed spec deliberately keeps that stale continuation inert. The
  focused races passed ten repetitions, the full clarification race suite passed, and changed-code Go
  lint reported zero issues.
- Fresh Codex review makes an explicit clean or permission session projection outrank filtered visible
  turn history. This keeps a predecessor clarification inert while an authoritative attempted successor
  is intentionally unpublished from client history, across both chat discovery and pending indicators.
  The 108 focused web tests, typecheck, zero-warning focused lint, and i18n ratchet passed.
- Follow-up Codex review adds authoritative per-session `pending_action` to semantic message events and
  narrowly mirrors it into loaded by-ID and per-task session projections. A newly streamed question now
  advances a cached clean projection, while terminal and successor mutations clear it without waiting
  for a session-list refresh. Event-arrival ordering stays authoritative across batched message updates.
  The 73 focused web tests, full task-service suite, targeted race suite (10 repetitions), web typecheck,
  zero-warning focused lint, i18n ratchet, backend changed-lines lint, and public-doc validators passed.
- A second Codex pass orders REST and WebSocket pending-action snapshots with a shared process-epoch
  logical revision reserved before each authoritative read. Store merges now reject older projections,
  including a deferred task-session list response that resolves after a newer message event.
- Follow-up Claude review separates degraded turn-service warnings from graceful missing-identity debug
  logs, rejects partial clarification response-delivery intents, and adds context-aware 1 ms / 2 ms
  backoff between startup summary compare-and-set retries.
- Follow-up CodeRabbit review keeps the independent response-delivery recovery pass running after a
  malformed reservation while excluding delivery IDs still owned by unresolved prompt reservations.
  Detached-bundle recovery coverage also confirms the disconnect marker survives restoration.
- An intermediate exact-head Codex review replaced timestamp generations with opaque UUID epochs and
  a bounded client retirement list. A later review exposed the remaining client-reload hole, so the
  final design uses a database-backed monotonic generation and numeric client comparison instead.
- Exact-head CodeRabbit review bounds a responder waiting for an already-started durable confirmation
  while leaving the waiter to retain its eventual result, and consolidates HTTP and WebSocket session
  summary projections behind one helper.
- Latest exact-head Codex review rejects unseen stale epochs after client state rebuild and routes
  canceller-originated message updates through the task service's projection-aware publisher. The
  former uses an atomic `kandev_meta` generation allocated at backend startup; the latter covers both
  expired bundles and first-observed detachment.
- This remediation batch passed all affected backend package suites (backend app, clarification,
  MCP handlers, orchestrator, task handlers, SQLite repository, and task service), focused race
  tests with three final repetitions, 39 focused web tests, the superseded-clarification E2E
  regression, web typecheck, zero-warning frontend and Go changed-lines lint, format and i18n
  checks, and both public-doc validators.
- The final review follow-up passed all affected backend suites again, both projection-focused web
  suites (21 tests), web typecheck, zero-warning frontend and Go changed-lines lint, i18n checks and
  ratchet, and the public-doc validator's 61 tests plus 41-page scan.
- Latest Claude review bounds the sidebar's message-only clarification fallback to the newest visible
  turn. A second suggestion was already covered by `clearPendingLocked`; explicit restart coverage now
  proves stale dismissal clears and persists an aggregate-only restored pending action. The three
  affected web suites (110 tests) and full status-summary package pass.
- Exact-head Codex review makes that sidebar fallback use durable message creation order rather than
  WebSocket arrival order. A delayed predecessor-turn frame can no longer hide the active current-turn
  clarification; the five affected web suites (127 tests), typecheck, and zero-warning focused lint pass.
- Exact-head CodeRabbit review makes clarification restore contingent on the still-live durable delivery
  marker, so a late successful confirmation cannot be reopened after the responder times out. It also
  keeps aborted sessionless mobile selection inert, guarantees blocked test cleanup, and covers deleted
  message pending-action projections. All affected backend packages, 81 focused web tests, typecheck,
  zero-warning frontend lint, and changed-code Go lint pass.
- Exact-head Claude review replaces a watchdog test's hot polling loop with a bounded eventual assertion
  and documents why the legacy event path intentionally relies on the repository's current-turn SQL
  authority guard. Both focused orchestrator regressions pass.
- The next Claude and CodeRabbit pass keeps a stale-dismiss event from clearing a different pending
  identity in event-only projector mode and makes an active-turn lookup error outrank any partial turn
  returned alongside it. Both review regressions pass.
- The latest review pass bounds terminal clarification expiry before state publication, patches active
  turn metadata under session authority, exposes mixed-turn bundle corruption in warning logs, and
  makes blocking test cleanup failure-safe. PostgreSQL-gated epoch coverage now checks monotonic and
  corrupt-value behavior. All affected backend suites and changed-code lint pass; the PostgreSQL case
  is locally skipped when `KANDEV_TEST_POSTGRES_DSN` is unavailable.
- Exact-head Greptile and Codex follow-up closes the remaining writer and deadline gaps: full turn
  snapshots now lock and reject stale versions, active metadata patches return their committed state
  without a fallible post-commit read, and terminal expiry preserves the orchestrator's shorter deadline
  through the cancellation-detached persistence layer. Cross-dialect lock coverage is PostgreSQL-gated;
  all locally runnable affected suites pass.
- The accompanying CodeRabbit pass makes reserved-turn waiter cleanup immediate and idempotent in five
  agent-ready regressions. A later exact-head summary regression enforces the feature spec by invalidating
  a known-stale status-summary row when pending-action CAS repair exhausts its retries, preserving the
  authoritative coarse fallback. A later exact-head authority regression supersedes the delayed suggestion
  to restrict add/delete events: all message additions and deletions refresh pending action because they
  can add or remove the evidence that makes a reserved successor authoritative; only ordinary content
  updates omit the projection.
- A final Codex race follow-up routes late prompt-usage and agent-identity fields through an atomic
  metadata patch that works after reservation publication and turn completion. The stale full-snapshot
  CAS remains as a clobber guard, while publication-winning usage metadata now persists on retry-free
  merge semantics. Full SQLite repository, task-service, and orchestrator suites pass.
- Exact-head Codex and Greptile follow-up preserves both client and restart convergence. Ordinary message
  add/delete events now carry the authoritative pending-action projection when message evidence changes
  turn authority. Reserved-turn publication keeps recovery markers durable until `turn.started` is
  accepted, surfaces bus failure, then clears only reservation metadata under session authority even if
  a fast provider completed the turn during publication. Focused regressions and the full task,
  integration, backend-composition, and orchestrator suites pass.
- Delayed CodeRabbit and Claude follow-up makes anomalous nil session loads fail turn reservation without
  a panic, documents all three pending-clarification scope states (including the empty-object trap), and
  emits a structured warning with task, attempt count, and last revision when summary reconciliation
  exhausts its compare-and-set retry budget. Focused backend tests and focused frontend formatting/lint
  pass.
- Exact-head Codex follow-up sanitizes private prompt-dispatch recovery fields from every public turn
  event. The concurrent publication regression now completes through the task service and proves a fast
  `turn.completed` payload cannot retain the pre-cleanup metadata.
- A further exact-head Codex regression marks the known-stale summary invalid in partial boot/task-list
  results when all pending-action compare-and-set retries lose. The authoritative coarse task fallback
  remains visible instead of being shadowed by the stale summary.
- Exact-head Greptile review adds a durable startup event outbox for accepted and ambiguous prompt
  reservations. Recovery replaces private reservation fields with a start-event marker, replays
  `turn.started` and any already-required `turn.completed` event in order, and clears the marker only
  after successful publication. Failed replay now fails startup and remains retryable across restarts.
- Replay regressions, the full task subtree, backend composition, integration, orchestrator, executor,
  and changed-code Go lint pass. PostgreSQL parity remains CI-gated when no local DSN is configured.
- Delayed Codex and Claude review makes summary invalidation explicit across the Go DTO and shared
  desktop/mobile snapshot merge while protecting a newer in-flight WebSocket revision. It also caps
  post-deadline delivery-confirmation waiting at five minutes, treats partial clarification recovery
  metadata as a startup error, suppresses expected cancellation warnings, documents the duplicated SQL
  bind, and pins numeric epoch ordering across `"9"` to `"10"`.
- All affected Go package suites, 31 focused web tests, frontend typecheck, zero-warning frontend and
  changed-code Go lint, and the i18n ratchet pass.
- Latest Claude and CodeRabbit follow-up reports corrupt persisted projection epochs with an actionable
  metadata diagnostic and makes a response claim immutable across an outliving confirmation callback.
  Callback output remains locally owned, while durable finalization and recovery continue to serialize.
- Focused confirmation, immutable-snapshot, projection-epoch, and race-enabled regressions pass, along
  with the full clarification and SQLite repository suites.
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
- Schema changes or unrelated task-summary wire-shape changes.
- Clarification UI redesign or new user-facing copy.
- Historical notification retraction.
- Permission lifecycle changes beyond shared pending-owner navigation.
