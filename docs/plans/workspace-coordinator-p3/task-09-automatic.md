---
id: "09-automatic"
title: "The first automatic class: create_task"
status: pending
wave: 2
depends_on:
  - "01-flag-schema-settings"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-AUTOMATIC-001
  - REQ-COORDINATOR-AUTOMATIC-002
  - REQ-COORDINATOR-AUTOMATIC-003
  - REQ-COORDINATOR-AUTOMATIC-004
acceptance_criteria:
  - AC-COORDINATOR-AUTOMATIC-001.1
  - AC-COORDINATOR-AUTOMATIC-001.2
  - AC-COORDINATOR-AUTOMATIC-002.1
  - AC-COORDINATOR-AUTOMATIC-002.2
  - AC-COORDINATOR-AUTOMATIC-002.3
  - AC-COORDINATOR-AUTOMATIC-002.4
  - AC-COORDINATOR-AUTOMATIC-003.1
  - AC-COORDINATOR-AUTOMATIC-003.2
  - AC-COORDINATOR-AUTOMATIC-003.3
  - AC-COORDINATOR-AUTOMATIC-003.4
  - AC-COORDINATOR-AUTOMATIC-004.1
  - AC-COORDINATOR-AUTOMATIC-004.2
  - AC-COORDINATOR-AUTOMATIC-004.3
system_design:
  - ../../specs/coordinator/system-design/automatic.md
---

# Task 09: The First Automatic Class, create_task (WP-11)

## Summary

Lets a manager raise `create_task` to `automatic` for one coordinator once its
own log earns it, approves such proposals automatically through the phase 1
approve service, and lowers the class on an undo of an automatic create.
Starts only when phase 2 is merged and the ADR records the 30-day log review
confirming `create_task`.

## In scope

- `internal/coordinator/phase2.go`: adapters for `ActionSettings` and
  `DecisionLog` over phase 2's code
  ([Phase 2 interfaces consumed](../../specs/coordinator/system-design/automatic.md#phase-2-interfaces-consumed)).
- `internal/coordinator/automatic.go`: the closed `raisableClasses`, the
  change validator (400 for other classes, 409 naming the first unmet
  condition), `Eligibility`, the class review store and routes, the automatic
  branch in `propose_task_kandev` with the keyed mutex and the 10-per-24h
  limit counted on `automatic_at` whatever the outcome, the raiser check
  before the claim, `decided_automatically` and `automatic_at` on the claim,
  the adapter's exclusion of automatic rows from decided rows, the
  server-computed review window and row count, and `OnUndo` lowering with the
  backstop retry ([automatic](../../specs/coordinator/system-design/automatic.md)).
- Web: in phase 2's permission settings, "Cannot be raised", the eligibility
  list, **Review the last 30 days**, **Mark as reviewed**, **Raise to
  automatic**, the raised record and **Lower**
  ([Screens](../../specs/coordinator/system-design/automatic.md#screens)).
- Copy in six locales.

## Out of scope

- Any other raisable class; improvement proposals (never automatic).

## ASCII UI preview

See [plan UI-06](plan.md#ascii-ui-previews).

```text
Create a card   Requires approval
  Not met  90% approved without edits (85%)  ...
  [Review the last 30 days] [Mark as reviewed] [Raise to automatic] (disabled)
Merge           Cannot be raised
```

## Acceptance

- Setting any class other than `create_task` to `automatic` is 400 naming it;
  `create_task` is 409 naming the first unmet of the five conditions, each
  tested at its boundary (29 and 30 days, 19 and 20 rows, 89% and 90%, one
  undo, 7 days); a log read error refuses; lowering is never checked; review
  and settings writes are refused to readers and a coordinator principal.
- A raised coordinator's valid proposal is approved automatically with every
  phase 1 approve guarantee and no agent start, returns `approved` with the
  task id, logs decider `automatic` and the raising manager; the eleventh in
  24 hours stays `pending` with the limit note, also when earlier automatic
  approvals ended `failed`; a failed approval is left `failed` for a manager
  and keeps its `automatic_at` through stale-claim recovery.
- With the raiser deleted, disabled or no longer a manager, the proposal stays
  `pending` with the unavailable note, nothing is claimed or counted, and the
  class is lowered with its reason; a raiser check error leaves it `pending`
  without lowering.
- Automatic approvals are not decided rows: after a lower and re-raise, 20
  automatic approvals plus 5 manager decisions fail `volume`.
- A review POST with any body stores `window_end` = now, `window_start` =
  now minus 30 days and `row_count` from the log, ignoring client fields; a
  log read error is 503 and stores nothing.
- Undoing a task an automatic approval created lowers the class at once and
  logs the reason; a failed lower is retried by the backstop.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... -run 'Automatic|Eligibility|ClassReview|Phase2' -count=1
cd apps/backend && go test ./internal/coordinator/... -run 'Automatic' -race -count=1
cd apps/web && pnpm test -- app/settings/workspace app/coordinator
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/automatic.spec.ts
```

## Risks

- The e2e test needs 30 days of log rows: seed them through phase 2's test
  fixture, never by weakening the history condition.
