---
spec: docs/specs/ui/requirements/message-queue-run.md
created: 2026-08-16
status: done
---

# Implementation Plan: Unify Queue Run Controls

## Overview

Replace misleading header **Run next** and bulk **Send Now** actions with one
durable per-session **Auto-run** switch. OFF lets the current response finish
and holds every later FIFO entry. ON runs eligible entries as separate turns.
Every row, including the head, keeps targeted **Send Now**; an accepted Send
Now also enables Auto-run and transiently promotes that row before the
preserved remainder.

The backend owns and persists this policy because automatic drains originate
from readiness, workflow, lifecycle, CI automation, and other server paths.
The browser projects and mutates that authoritative state.

---

## Backend

### Persist queue-owned session policy

Extend `apps/backend/internal/orchestrator/messagequeue` with a dedicated
`queue_session_state` table and equivalent memory state:

- `auto_run` is keyed by session ID;
- absence reads as ON for upgrade compatibility;
- explicit ON and OFF survive an empty queue and repository reconstruction;
- policy mutations use the existing per-session admission and
  `queue_session_locks` serialization;
- task-session transfer combines source and destination with pause-wins
  semantics, then removes the source state;
- message snapshot/restore does not overwrite policy.

Add repository/service operations to read and set the policy and to reserve a
head only when Auto-run is enabled. The policy-aware reservation result must
distinguish “paused” from “enabled but empty/error” so producers can treat a
held message as accepted work rather than a dispatch failure.

Change `ClaimSendNow` so the exact-selection mutation also persists ON before
commit. Rejected claims leave the prior value untouched. Accepted claims keep
ON if their asynchronous prompt path later restores message rows. Extend
`QueueStatus` with the always-present `auto_run` projection.

Repository tests cover memory, SQLite, and Postgres-compatible behavior:

- missing-state default and ON/OFF persistence;
- policy-aware reserve and durable lifecycle reservation;
- an OFF-vs-reserve transaction race;
- accepted and rejected Send Now claims;
- empty queues, transfer pause-wins, and snapshot independence;
- status projection and repository reconstruction.

### Enforce policy across orchestration

Add an orchestrator operation that owns switch behavior under the existing
per-session cancel/queue-take guard:

- OFF persists without cancelling the active turn;
- ON persists and immediately attempts the head only when promptable;
- ON succeeds but reports no immediate dispatch while busy, empty, awaiting
  clarification, or otherwise lifecycle-blocked;
- every outcome publishes authoritative queue status.

Route every automatic FIFO take through the policy-aware reservation. Audit at
least:

- ready and boot-ready ACP drains;
- passthrough ready delivery;
- workflow on-enter, transition, failure, and deferred-move drains;
- lifecycle prompt drains;
- CI automation replacement drains;
- any indirect helper that reserves the ordinary FIFO head.

Do not apply the implicit check to exact targeted operations. Instead:

- accepted Send Now resumes atomically through its repository claim;
- successful legacy `DrainQueuedMessage` enables Auto-run and preserves its
  current response shape;
- explicit user Cancel persists OFF when pending entries remain before it
  releases its cancellation guard;
- internal cancellation and Send Now replacement cancellation leave policy
  unchanged;
- CI/lifecycle producers treat policy-paused work as safely queued, not as a
  failed delivery.

Add orchestrator tests for active-turn OFF, promptable ON, readiness
continuation, clarification, passthrough, lifecycle and CI entries, Cancel,
Send Now, workflow transfer, and the acknowledged OFF-vs-ready race boundary.

### Expose the WebSocket contract

Add `ActionMessageQueueAutoRunSet` and register a queue handler for:

```text
message.queue.auto_run.set
  request:  { session_id: string, enabled: boolean }
  response: { session_id: string, auto_run: boolean, dispatched: boolean }
```

Require explicit boolean presence, authorize the session before mutation, map
validation/internal failures through existing queue error conventions, and
publish `message.queue.status_changed`. Existing `message.queue.get`, status
events, drain, and Send Now payloads remain compatible except for the additive
`auto_run` status field.

---

## Frontend

### Project authoritative state

Add `autoRun` to `QueueMeta` and thread `auto_run` through:

- `message.queue.get` typing and API tests;
- `message.queue.status_changed` parsing;
- Zustand queue meta and slice tests;
- `useQueue` refetch/reconciliation.

Add `setQueueAutoRun(sessionId, enabled)` to the queue API and expose
`autoRun` plus `setAutoRun(enabled)` from `useQueue`. Use the existing
per-session loading gate, refetch after success or failure, and never retain an
optimistic value that conflicts with the server. Keep the generic drain and
all-scope Send Now API methods for protocol compatibility while removing their
first-party header plumbing.

### Replace header actions with Auto-run

Update the queue panel header and list wiring:

- remove header Run Next and bulk Send Now, their icons, props, callbacks, and
  test IDs;
- render one visible **Auto-run** Switch with ON/OFF helper copy and a stable
  `queue-auto-run` selector;
- bind `checked` to backend queue meta, disable it during queue mutation or
  backend cancellation, and expose checked/disabled state semantically;
- keep Clear all, desktop pin, and collapse;
- preserve targeted Send Now on every row, including index zero;
- remove UI-facing `drainNext` and `sendAllNow` hook results only after an `rg`
  audit proves no first-party caller remains.

Extract `queued-ghost-auto-run-control.tsx` if needed so the already-large list
does not absorb switch presentation and responsive layout logic.

### Localization

Add Auto-run label, ON/OFF helper, and failure copy to `chat.json` for `en`,
`pt-pt`, `zh-cn`, `zh-hk`, and `zh-tw`; regenerate `pseudo`. Remove obsolete
Run Next keys only after auditing all consumers. Retain row Send Now and
protocol-level error copy.

---

## Mobile Design Contract

- **Shared outcome:** desktop and phone use the same backend policy, hook,
  Switch, row Send Now behavior, and error reconciliation.
- **Composition:** retain the existing inline queue panel and
  `queue-scroll-region`; do not add a drawer. The composer remains visible and
  the queue list remains the sole internal scroll owner.
- **Hierarchy:** queue count and Auto-run state are primary. Clear all,
  desktop-only pin, and collapse remain secondary. Every row retains its own
  exact Send Now action.
- **Geometry:** the Auto-run label and switch form an effective target at least
  44 by 44 CSS pixels on coarse pointers. The helper and controls may wrap to a
  second header line without document horizontal overflow.
- **Interaction:** touch users can toggle without hover; row Send Now stays
  discoverable and touch-sized. Focus, checked, disabled, and screen-reader
  behavior come from the shared Switch primitive.
- **Proof:** Pixel 5 E2E taps OFF and ON, reloads the page, invokes a row Send
  Now from OFF, verifies turn order, target size, and viewport containment.

---

## Tests

### Backend package and handler coverage

- Repository contract tests for defaulting, persistence, atomic reserve,
  transfer, snapshot independence, and Send Now resume.
- Postgres race coverage proves OFF and automatic reserve serialize on the
  real `queue_session_locks` row.
- Orchestrator tests prove OFF never cancels current work, all automatic drain
  origins respect it, ON starts only when eligible, and explicit targeting
  follows its resume contract.
- Existing Cancel recovery tests are amended to assert OFF, while internal
  cancellation tests prove they do not mutate policy.
- Queue handler tests cover missing/non-boolean `enabled`, authorization,
  service absence, ON/OFF success, and additive status output.

### Frontend unit and component coverage

- Queue API tests assert the exact set request and response.
- WebSocket handler and session-slice tests retain `auto_run` from both fetch
  and pushed status.
- Hook tests prove loading/refetch behavior and failure reconciliation.
- Header/list tests prove legacy actions are absent, the switch reflects
  backend state, OFF/ON callbacks use exact values, cancellation disables it,
  and every row still exposes exact Send Now.
- Responsive component assertions cover wrapping classes, accessible helper
  relationships, and coarse-pointer target sizing.

---

## E2E Tests

- **Finish current, then hold:** while a slow response runs with A, B, C
  queued, toggle OFF. Prove the current response completes and no queued turn
  begins.
- **Resume FIFO:** toggle ON in the promptable parked state. Prove A, B, C run
  as separate turns in order without another click.
- **Targeted resume:** from OFF, use Send Now on B. Prove the switch becomes ON,
  B replaces the captured turn without ordinary Cancel effects, then A and C
  run as separate turns.
- **Persistence:** turn OFF, reload, and prove the backlog stays held and the
  switch remains OFF. Repository tests provide backend-restart proof.
- **Cancel consistency:** explicit Cancel with a backlog leaves it parked and
  projects OFF; ON resumes it.
- **Clarification:** ON may be changed and displayed, but no queued message
  bypasses a pending clarification.
- **Mobile parity:** repeat hold, reload, resume, and targeted Send Now on
  `mobile-chrome`; assert 44-pixel geometry and zero horizontal overflow.
- Audit desktop and mobile E2E for removed `queue-drain-next` and header
  `queue-send-now` selectors. Keep per-row `queue-entry-send-now` coverage.

Use the managed E2E runner, causal waits, production builds, and its teardown.
Put `--project` before test paths and keep mobile scenarios in
`mobile-*.spec.ts` files.

---

## Documentation

After browser proof passes, update `docs/public/coordination.md` and
`docs/public/sessions-and-review.md` to explain:

- Auto-run ON processes one queued message per turn;
- OFF lets the current response finish and holds later entries;
- Cancel stops immediately and leaves a pending backlog OFF;
- any row's Send Now resumes Auto-run and transiently runs that row first;
- lifecycle and clarification guards can defer an ON queue.

Remove first-party guidance for header Run Next, bulk Send Now, or Skip to
next. Keep protocol-only all-scope and drain references where appropriate.
Mark the spec and index `shipped` only after implementation and E2E evidence.

---

## Verification Results

Tasks 01 through 05 complete. Memory, SQLite, race-enabled, messagequeue,
orchestrator, queue-handler, frontend unit/component, typecheck, lint, i18n,
desktop E2E (4/4), and mobile E2E (3/3) checks pass. Postgres lock proof is
present but skipped locally because `KANDEV_TEST_POSTGRES_DSN` is unset. Public
documentation validation passes 61/61 tests and validates all 41 published
pages.

---

## Implementation Waves And Parallel Candidates

Default execution is sequential in the primary conversation.

Wave 1:

- [x] [Task 01: Persist queue Auto-run policy](task-01-persist-queue-auto-run.md)

Wave 2:

- [x] [Task 02: Enforce queue Auto-run](task-02-enforce-queue-auto-run.md)

Wave 3:

- [x] [Task 03: Add the Auto-run switch](task-03-add-auto-run-switch.md)

Wave 4:

- [x] [Task 04: Prove Auto-run browser flows](task-04-prove-auto-run-flows.md)

Wave 5:

- [x] [Task 05: Publish Auto-run guidance](task-05-publish-auto-run-guidance.md)

No task is marked parallel-safe. Orchestration consumes Task 01's atomic
repository contract; frontend consumes Task 02's protocol; E2E consumes final
selectors and behavior; docs publish only proven behavior.

---

## Risks

- An OFF click can race the next reservation at turn completion. Repository
  serialization and explicit tests must preserve the acknowledged boundary:
  the winner may become current, but no later row may overtake OFF.
- Automatic drains are distributed across ACP, passthrough, workflow,
  lifecycle, and CI code. A missed raw `ReserveQueued` call would bypass user
  policy; implementation must audit every production call site.
- A false return currently conflates empty, blocked, and failed dispatch in
  some callers. Policy deferral needs an explicit result so automation does not
  report a safely held prompt as failed.
- Send Now already uses asynchronous claim restoration. Auto-run activation
  must be atomic with claim and remain ON after accepted restoration without
  changing pre-claim failure behavior.
- The header is dense on phones. Use wrapping and shared switch semantics, then
  prove touch geometry and containment rather than shrinking the control.
- Removing bulk UI must not remove `scope: "all"` or legacy drain protocol
  compatibility.
