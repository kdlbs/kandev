---
spec: docs/specs/platform/bounded-task-status-delivery.md
created: 2026-08-01
status: implemented
---

# Implementation Plan: Bounded Task Status Delivery

## Overview

Replace task-switcher dependence on background session streams with one
backend-owned, bounded task-status projection. Hydrate that projection through
the existing boot/task snapshot paths, publish revisions on the workspace task
feed, and make both desktop and mobile rows consume it. Preserve full session
streams for selected detail surfaces, make subscribe/focus idempotent, decouple
Git monitoring from browser interest, and reserve WebSocket delivery capacity
for correlated responses. Finally, make message submission retry-safe so a
transport interruption cannot create a duplicate prompt.

This is a staged migration. Existing task runtime fields remain as a coarse
fallback until all snapshot producers and consumers carry `status_summary`.
No task-row consumer may fall back to subscribing to an inactive session.

## Architecture

### Projection ownership

Add a typed package under `apps/backend/internal/task/statussummary` with:

- the bounded `TaskStatusSummary` model and aggregate enums;
- repository interfaces for batch load, compare/update, and delete;
- a keyed projector that rebuilds or applies source observations;
- provider interfaces for session/pending/error/Git/PR reads, wired by
  `backendapp` to avoid package cycles; and
- semantic equality that excludes revision/update timestamps, so a no-op does
  not write or publish.

Persist one row per task in `task_status_summaries` with `task_id`,
`workspace_id`, `revision`, serialized summary, and `updated_at`. Use the
repository's portable schema/migration conventions and cover both SQLite and
Postgres replay. Serialize compare/update per task so concurrent source events
cannot lose a newer projection. Deleting a task cascades to the projection.

The authoritative inputs remain unchanged:

| Authoritative occurrence                                 | Projected fields                               | Refresh rule                                                    |
| -------------------------------------------------------- | ---------------------------------------------- | --------------------------------------------------------------- |
| Task/session lifecycle and primary-session changes       | `primary_session`                              | Rebuild on semantic lifecycle event                             |
| Foreground activity/subagent changes                     | `foreground_activity`, `active_subagent_count` | Rebuild only on published aggregate change                      |
| Pending permission/clarification                         | `pending_action`                               | Read the existing task-level pending aggregate                  |
| Recoverable agent error, dismissal, newer agent response | `active_error`                                 | Explicit error-change signal plus relevant agent-message events |
| Per-repository Git observation                           | `git`                                          | Coalesce by task/repository, then aggregate latest observations |
| GitHub task-PR update                                    | `pull_request`                                 | Recompute bounded attention state and representative identity   |

Filter message events before scheduling a rebuild. Streaming thought, shell,
status, and other message updates that cannot change pending/error semantics
must not churn the task projection.

### Snapshot and delta delivery

Extend `task/dto.TaskDTO` with `StatusSummary`. Batch-load summaries while
building boot state, task lists, and workflow snapshots; do not add one query
per task. A missing row is rebuilt from authoritative records and may be
returned as an absent/coarse fallback for the current request rather than
blocking the whole workspace.

Add internal `TaskStatusSummaryUpdated` and wire action
`task.status_summary.updated`. The gateway routes it to the task's workspace
with the same authorization boundary as task updates. The event carries a
complete replacement summary. Frontend merges compare `revision` and ignore
duplicates or older deliveries.

### Git monitoring lifecycle

Today slow/paused polling is inferred from browser session subscriptions. Add
an internal runtime-interest source: a live task execution keeps its workspace
at least in slow mode, explicit focus upgrades it to fast, and the workspace
returns to paused only when neither runtime nor viewer interest exists. Feed
bounded per-repository observations into the summary projector before or with
the existing live snapshot persistence.

Keep completion/settled Git snapshots authoritative after execution ends. A
temporary poll failure preserves the latest summary. Multi-repository totals
are calculated from a keyed latest-observation set so one repository update
does not erase siblings.

### Session stream and WebSocket delivery

Make server subscription membership observable to handlers. A
`session.subscribe` transition from absent to present sends one initial full
snapshot; duplicate subscriptions only acknowledge. `session.focus` updates
polling priority and acknowledges without calling `sendSessionData`. Add
`session.git.refresh` for the selected detail surface and migrate the frontend
away from using focus as a refresh command.

Split each gateway client's outbound path into:

- a reserved control queue for request responses and errors; and
- a bounded notification queue for unsolicited state.

The write pump drains control traffic first while preserving FIFO order within
each class. Notification overload remains bounded and observable. Control
enqueue failure closes the connection instead of silently losing the frame,
allowing the client to reconnect and reconcile. Add metrics/structured logs
for notification drops, control overflow, queue depth, and disconnect reason.

### Frontend task-summary migration

Add summary types and map `status_summary` in `lib/kanban/map-task.ts`. Store the
latest summary on each Kanban task and patch it from
`task.status_summary.updated` only when the revision advances. Preserve the
summary when a partial task update omits it.

Refactor the desktop sidebar and mobile task-switcher sheet to derive:

- pending flags from `summary.pendingAction`;
- error display from `summary.activeError` plus the existing local
  acknowledgement stamp;
- diff stats from `summary.git`;
- session state/activity from the summary; and
- the task-row PR icon from the bounded PR aggregate.

Remove `useBulkGitStatusSubscription` and the switcher's workspace-wide PR
fetch. Active task/detail consumers retain their session Git and detailed PR
stores. If a summary is missing, render coarse task/session state and omit the
unknown decoration; never restore an inactive subscription.

### Retry-safe message submission

Generate one stable `client_message_id` when the user submits and include it
in `message.add` (the backend accepts `message_id` as a compatibility alias).
Keep that ID across reconnect/retry and optimistic/response/notification
hydration.

On the backend, serialize acceptance by message ID and check for an existing
authorized user message before session-state validation, `on_turn_start`,
message creation, or dispatch. The first accepted request owns the ID. A
duplicate for the same task returns that message; cross-task or unauthorized
reuse is a conflict. Use `CreateMessageWithID` for the first write and reload
after a uniqueness race. Duplicate handling must not repeat lifecycle hooks or
prompt dispatch.

On timeout/disconnect, wait for bounded response/notification/list
reconciliation and retry the same ID after reconnect when needed. Clear the
composer once acceptance is known. Show **Message send status unknown** only
when reconciliation cannot establish whether the ID was accepted.

## Tests

### Backend unit and integration tests

- Projection repository: portable schema replay, batch reads, monotonic
  compare/update, semantic no-op, concurrent source updates, delete cascade,
  and missing-row repair.
- Projection derivation: status precedence, error suppression/dismissal,
  bounded UTF-8 preview, multi-repository Git aggregation, PR attention state,
  and filtering of irrelevant message events.
- Snapshot delivery: boot/task/workflow payloads load summaries in batches and
  workspace events carry the complete newer revision.
- Polling: live execution retains slow monitoring with no browser subscriber;
  focus upgrades to fast; settle/removal returns to the correct remaining
  interest level.
- Gateway: duplicate subscribe/focus does not replay, targeted Git refresh
  emits only Git data, notification saturation cannot starve a response, and
  control overflow closes explicitly.
- Message submission: duplicate and concurrent duplicate IDs create one row,
  run turn-start once, dispatch once, and reject cross-scope reuse.

### Frontend tests

- Task mapping and task-event reducers preserve newer summaries and reject
  stale revisions or omitted fields.
- Desktop/mobile derivation reads only summary fields while preserving pending,
  activity, completion, error, Git, and PR precedence.
- Sidebar mounting and task switching issue no session subscriptions for
  inactive tasks.
- Message send keeps one ID through timeout/reconnect, reconciles response and
  notification gaps idempotently, and retains a usable composer/model selector.

## E2E Tests

- Seed a workspace with at least 27 tasks/sessions, select one task, and record
  WebSocket frames. Assert that inactive session IDs never receive
  `session.subscribe` and never deliver message, shell, model, MCP, or Git
  detail frames to the switcher.
- Switch among five tasks. Assert each switch performs at most the constant
  active-session membership/focus work and that counts do not grow with the 27
  task rows. Attach frame counts and byte totals to the Playwright result for
  before/after diagnosis without making compression-dependent byte counts the
  correctness oracle.
- Drive clarification, recoverable error/dismissal, Git, and PR changes on
  inactive tasks. Assert both desktop and `mobile-chrome` rows update through
  summary revisions.
- Saturate the notification path with background activity, then publish the
  selected session's model configuration and submit a message. Assert the model
  selector remains available, the response resolves, and exactly one user
  message/turn is present after a forced retry of the same message ID.

Update the existing session-focus, sidebar-diff, and message-response-gap E2E
tests so they enforce the new transport contract rather than preserving the
old bulk-subscription behavior. The 27-task gateway capture lives in
`apps/web/e2e/tests/session/session-stream-budget.spec.ts` and shares the
diagnostic helper in `apps/web/e2e/helpers/ws-traffic.ts`.

## Mobile design contract

- **Desktop outcome / mobile entry:** both surfaces show the same task-summary
  state. Desktop uses the sidebar; phone uses the existing session task-switcher
  sheet.
- **Nearest exemplar:**
  `apps/web/components/task/mobile/session-task-switcher-sheet.tsx` remains the
  native mobile entry and continues to render shared `TaskSwitcher` rows.
- **Hierarchy / interaction:** no new mobile control, drawer, gesture, or
  navigation layer is introduced. Row taps, dismiss actions, and selection
  behavior remain unchanged.
- **Presentation / scroll:** the existing sheet owns scrolling and safe-area
  behavior. Summary migration changes data flow only and must not add nested
  scroll or viewport overflow.
- **Shared state:** status precedence and row rendering stay shared; the mobile
  hook stops rebuilding status from session messages/Git stores.
- **Mobile proof:** `mobile-chrome` exercises pending, error, Git, and PR
  summary updates plus repeated task switching with no inactive subscriptions.

## Implementation waves

Implementation remains sequential in the user-started session because the
tasks evolve shared task, event, runtime, and WebSocket contracts.

- [x] [Task 01: Persist task status summaries](task-01-persist-task-status-summaries.md)
- [x] [Task 02: Publish live task status](task-02-publish-live-task-status.md)
- [x] [Task 03: Decouple Git polling from viewers](task-03-decouple-git-polling.md)
- [x] [Task 04: Stabilize session transport](task-04-stabilize-session-transport.md)
- [x] [Task 05: Consume summaries in task switchers](task-05-consume-task-summaries.md)
- [x] [Task 06: Make message submission idempotent](task-06-idempotent-message-submission.md)
- [x] [Task 07: Prove bounded task traffic](task-07-prove-bounded-task-traffic.md)

## Verification

```bash
make -C apps/backend test
make -C apps/backend lint
cd apps/web && pnpm exec vitest run
cd apps/web && pnpm run lint
cd apps/web && pnpm run typecheck
cd apps/web && pnpm e2e:run tests/session/session-stream-budget.spec.ts \
  -- --project=chromium
cd apps/web && pnpm e2e:run tests/task/task-status-summary.spec.ts \
  -- --project=chromium
cd apps/web && pnpm e2e:run tests/task/mobile-task-status-summary.spec.ts \
  -- --project=mobile-chrome
cd apps/web && pnpm e2e:run tests/chat/message-send-pressure.spec.ts \
  -- --project=chromium
```

## Risks

- Projection feedback loops are possible if summary events are mistaken for
  authoritative task changes. Source filtering and semantic no-op comparison
  must keep publication one-way.
- Error and pending semantics currently rely partly on loaded messages. The
  projector needs explicit occurrence/dismissal signals and latest-agent-message
  timestamps so removing message subscriptions does not regress indicators.
- A browser-independent slow Git baseline changes lifecycle ownership. It must
  count live executions once per workspace and release interest on every stop,
  failure, cancellation, and recovery path.
- Multi-repository Git and PR updates can arrive independently. The projector
  must aggregate from a complete latest-by-repository/PR view rather than from
  one event payload.
- Strict control priority can starve notifications under sustained request
  load. Drain control first but yield to notifications after a bounded batch;
  keep per-class FIFO tests and queue metrics.
- Message ID deduplication must happen before mutable turn-start behavior. A
  lookup after `on_turn_start` would still allow a retry to advance workflow or
  switch primary sessions twice.
- Traffic E2E can pass accidentally if seeded background sessions emit no
  data. Tests must prove the inactive streams are producing server-side events
  while asserting those detail frames do not reach the browser.

## Documentation impact

This plan adds the internal product contract
`docs/specs/platform/bounded-task-status-delivery.md` and architecture decision
`docs/decisions/2026-08-01-separate-task-summary-session-stream-traffic.md`.
No public documentation changes are expected: commands, settings, public APIs,
and user-visible interaction patterns remain unchanged.
