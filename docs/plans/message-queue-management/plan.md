---
spec: docs/specs/ui/message-queue-management.md
decision: docs/decisions/2026-08-03-separate-message-queue-provenance-cancellation-and-capacity.md
created: 2026-08-03
status: implemented
---

# Fix Plan: Message Queue Management

## Overview

Make the queue panel's **Clear all** promise literal, add removal for every
visible pending message, and expose the per-session queue limit under
**Settings > General > Message Queue**. Preserve durable in-flight delivery,
session isolation, message provenance, and already accepted retries.

## Confirmed Root Cause

The reported Bitbucket Integration task has ten persisted queue rows. Every row
has `queued_by='agent'`. The frontend sends `message.queue.cancel`, whose
handler calls `messagequeue.Service.CancelAll`; the SQLite and memory
repositories then delete only rows whose `queued_by` is not `agent`,
`workflow`, or `server`. The response therefore says `removed: 0`. The hook's
optimistic empty state is replaced by the next authoritative status event, so
the full queue immediately reappears.

Individual removal has the same repository guard. The React row action also
uses one `canEdit` flag for both **Edit** and **Remove**, hiding removal on
agent/workflow rows. This behavior implemented ADR 0051's prior ownership
boundary, so the repair requires an explicit contract change rather than only
a frontend patch.

The live installation was inspected read-only. No queued row, task, session,
or setting was mutated during diagnosis.

## Behavioral Contract

- An authorized session owner may remove any pending entry returned by queue
  status and may clear all such entries, regardless of provenance.
- `queued_by` continues to control editing and merge compatibility.
- The visible queue uses compact one-based ordinals after remove, merge, or
  drain; durable FIFO position keys remain unchanged.
- A durable lifecycle row already marked in flight remains hidden and
  non-cancellable. Cancellation and reservation must be race-safe.
- Every user-facing queue WebSocket handler authorizes its session. Queue add
  and append additionally validate that `task_id` belongs to `session_id`.
- Capacity resolves as valid environment value, then persisted install
  setting, then default `10`. `0` means unlimited.
- Saving capacity applies live, does not prune existing rows, and limits only
  new admissions. Restore/retry of already accepted work bypasses the cap.

## Backend Design

### Pending-row cancellation

- Change the `Repository.DeleteByID` and `DeleteAllBySession` contracts from
  user-origin deletion to visible-pending deletion.
- In both SQLite/Postgres-compatible and memory repositories, serialize with
  the existing per-session mutation lock and skip
  `lifecycle_reserved_in_flight` rows.
- Use persisted compare guards and affected-row counts so an out-of-process
  reserve or update that wins cannot be deleted or counted afterward.
- Preserve the mandatory `(session_id, entry_id)` scope and the privileged
  `PurgeTask`/`AcknowledgeByID` paths.
- Update service logging and error text to remove the obsolete "not owned by
  caller" implication.

### Queue action authorization

- Add a task-service authorization method for a `(task_id, session_id)` pair:
  authorize both resources, load the session, and fail closed when the pair is
  inconsistent.
- Inject a queue access authorizer into `QueueHandlers` from backend
  composition.
- Guard get, add, append, update, remove, merge, clear, and drain before any
  queue read or mutation. Add/append use pair authorization; session-only
  actions use session authorization.
- Return the existing non-enumerating error shape and test that unauthorized
  calls neither reveal status nor publish a mutation event.

### Runtime capacity setting

- Add `internal/system/queuesettings` with typed settings, generic-settings
  adapter, environment resolver, live target interface, service, and HTTP
  handlers.
- Persist `{"max_per_session": N}` under install-wide key `message_queue`.
- Resolve the setting before the orchestrator queue starts. Extend system
  wiring with the live `messagequeue.Service` so GET/PATCH use the same target
  and register under `/api/v1/system/message-queue/settings`.
- Register GET on the signed-in system group and PATCH on its admin subgroup.
  PATCH validates `N >= 0`, returns `409` while a valid environment override is
  active, persists first, then live-applies the response value.
- Replace the queue service's plain integer with a concurrency-safe runtime
  value and add `SetMaxPerSession`. Each admission snapshots it once for the
  repository call, log, and `queue_full` response.
- Normalize non-positive environment values to effective `0`; ignore and warn
  on malformed values. Preserve `KANDEV_QUEUE_MAX_PER_SESSION` compatibility.
- Make ordinary restore and durable lifecycle retry bypass the current
  admission cap. Initial user/agent/workflow/server insertions still obey it.

## Frontend Design

### Queue panel

- Split `canEdit` from `canRemove` in `QueuedGhostMessage` and `RowActions`.
  Every visible row gets remove; editing remains user-only and merge rules stay
  unchanged.
- Keep optimistic behavior only when the hook can reconcile it. On removal or
  clear failure/race, refetch authoritative status and show localized feedback.
- Replace touched queue-header literals with `chat` namespace keys.
- Keep compact hover controls on pointer devices. On coarse-pointer viewports,
  make remove, clear, close, expand, merge, and run-next actions at least 44px
  high/wide and always visible.

### General settings page

- Add API types and fetch/update helpers for the configured/effective response.
- Add a client settings card with a numeric `min=0` input, explicit unlimited
  help, source badge/help, loading/error states, environment lock state, and
  member read-only state.
- Register the draft with `useSettingsSaveContributor`; do not add local
  Save/Cancel controls.
- Add `/settings/general/message-queue` to the Vite route registry, app-page
  composition, General sidebar, breadcrumb label map, and sidebar-navigation
  tests. Redirect the former `/settings/system/message-queue` URL to preserve
  existing bookmarks.
- Add all new copy to translation catalogs and regenerate/check pseudo locale.
  Avoid module-scope `t()` by storing label keys and translating during render.

## Responsive and Mobile Contract

The nearest shipped queue exemplar is
`apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`: retain its inline
panel and single queue scroll region so the composer stays visible. The new
controls must not depend on hover and must meet the 44px coarse-pointer target.

The settings page uses the existing left-side mobile Settings sheet and shared
`settings-scroll-container`. Render one column with no nested page scroller;
keep the numeric field at least 44px high and above the existing save bar plus
safe-area padding. Desktop and mobile expose identical values and permissions.

## TDD and Verification Strategy

1. Change existing backend tests to expect agent/workflow/server pending rows to
   be removable and add in-flight/race/security regressions. Run them RED
   against unchanged production code.
2. Implement cancellation and handler authorization; run memory, SQLite, and
   env-gated Postgres package tests GREEN.
3. Add capacity resolver/store/service/handler tests, including live apply,
   environment precedence, lower-than-current behavior, and retry restoration.
   Run RED, then implement and rerun GREEN.
4. Add frontend component/hook/settings tests first, including split
   edit/remove permissions and environment/member locks. Implement minimal UI
   changes, then run typecheck, lint, and i18n gates.
5. Add desktop and mobile Playwright scenarios for mixed/agent provenance and
   settings navigation/save. Preserve and restore the install-wide cap in test
   teardown so parallel specs cannot inherit modified state.
6. Update public queue/configuration/operations docs and run their validators.

## Implementation Waves

- [x] [Task 01: Make pending queue cancellation authoritative](task-01-authoritative-queue-cancellation.md) — wave 1
- [x] [Task 02: Add live install-wide queue capacity settings](task-02-live-queue-capacity-settings.md) — wave 2, after Task 01
- [x] [Task 03: Expose removal for every visible queue row](task-03-visible-queue-removal-ui.md) — wave 3, after Task 01
- [x] [Task 04: Add the Message Queue General settings page](task-04-message-queue-settings-ui.md) — wave 3, after Task 02
- [x] [Task 05: Prove end-to-end behavior and update public docs](task-05-e2e-and-public-docs.md) — wave 4, after Tasks 03 and 04

Tasks 03 and 04 are file-disjoint enough to run in parallel after their backend
dependencies, but this package does not authorize subagents. Implementation
remains in the user-controlled primary session unless the user later delegates.

## Environment Prerequisite

If `apps/node_modules` is absent, run `pnpm install --frozen-lockfile` from
`apps/` before frontend RED tests. Do not change the lockfile.

## Risks and Boundaries

- A clear/remove request may lose to reservation; that message can still be
  delivered. The UI must reconcile rather than claim it cancelled in-flight
  work.
- Dynamic lowering must not make restore or lifecycle retry lossy.
- Settings tests mutate install-wide state and must restore their baseline even
  after failure.
- Queue contents from the diagnosed live Bitbucket task remain untouched until
  this package is implemented and deployed.
- Editing backend-origin content, changing merge rules, queue reordering,
  per-workspace limits, and automatic pruning remain out of scope.
