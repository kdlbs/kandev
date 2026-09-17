---
id: "01-wire-routine-trigger-contract"
title: "Wire the routine trigger snake_case/camelCase contract"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-TRIGGER-WIRE-001
  - REQ-OFFICE-TRIGGER-WIRE-002
  - REQ-OFFICE-TRIGGER-WIRE-003
  - REQ-OFFICE-TRIGGER-WIRE-004
acceptance_criteria:
  - AC-OFFICE-TRIGGER-WIRE-001.1
  - AC-OFFICE-TRIGGER-WIRE-001.2
  - AC-OFFICE-TRIGGER-WIRE-001.3
  - AC-OFFICE-TRIGGER-WIRE-001.4
  - AC-OFFICE-TRIGGER-WIRE-001.5
  - AC-OFFICE-TRIGGER-WIRE-001.6
  - AC-OFFICE-TRIGGER-WIRE-001.7
  - AC-OFFICE-TRIGGER-WIRE-001.8
  - AC-OFFICE-TRIGGER-WIRE-001.9
  - AC-OFFICE-TRIGGER-WIRE-001.10
  - AC-OFFICE-TRIGGER-WIRE-001.11
  - AC-OFFICE-TRIGGER-WIRE-001.12
  - AC-OFFICE-TRIGGER-WIRE-001.13
  - AC-OFFICE-TRIGGER-WIRE-002.1
  - AC-OFFICE-TRIGGER-WIRE-002.2
  - AC-OFFICE-TRIGGER-WIRE-002.3
  - AC-OFFICE-TRIGGER-WIRE-002.4
  - AC-OFFICE-TRIGGER-WIRE-002.5
  - AC-OFFICE-TRIGGER-WIRE-002.6
  - AC-OFFICE-TRIGGER-WIRE-002.7
  - AC-OFFICE-TRIGGER-WIRE-002.8
  - AC-OFFICE-TRIGGER-WIRE-003.1
  - AC-OFFICE-TRIGGER-WIRE-003.2
  - AC-OFFICE-TRIGGER-WIRE-003.3
  - AC-OFFICE-TRIGGER-WIRE-003.4
  - AC-OFFICE-TRIGGER-WIRE-003.5
  - AC-OFFICE-TRIGGER-WIRE-003.6
  - AC-OFFICE-TRIGGER-WIRE-003.7
  - AC-OFFICE-TRIGGER-WIRE-004.1
  - AC-OFFICE-TRIGGER-WIRE-004.2
  - AC-OFFICE-TRIGGER-WIRE-004.3
  - AC-OFFICE-TRIGGER-WIRE-004.4
  - AC-OFFICE-TRIGGER-WIRE-004.5
  - AC-OFFICE-TRIGGER-WIRE-004.6
  - AC-OFFICE-TRIGGER-WIRE-004.7
  - AC-OFFICE-TRIGGER-WIRE-004.8
  - AC-OFFICE-TRIGGER-WIRE-004.9
  - AC-OFFICE-TRIGGER-WIRE-004.10
  - AC-OFFICE-TRIGGER-WIRE-004.11
  - AC-OFFICE-TRIGGER-WIRE-004.12
system_design:
  - ../../specs/office/system-design/routine-trigger-wire-contract-01.md
---

# Task 01: Wire the routine trigger snake_case/camelCase contract

## Summary

Add the explicit snake_case/camelCase wire adapter for Office routine
triggers in both directions, a single-source-of-truth primary-cron-trigger
selector shared by the row and detail surfaces, and a create-time error path
that reports a failed trigger arm distinctly from a failed routine create
without leaving the dialog open or a phantom schedule behind.

## In scope

- `apps/web/lib/api/domains/office-routine-normalize.ts`:
  `normalizeRoutineTrigger`, `normalizeRoutineTriggerList`,
  `CreateRoutineTriggerInput`, `serializeRoutineTriggerInput`.
- `apps/web/app/office/lib/routine-trigger-selection.ts`:
  `selectPrimaryCronTrigger`.
- `apps/web/app/office/routines/routine-row.tsx`,
  `apps/web/app/office/routines/[id]/routine-detail-view.tsx`: consume the
  shared selector instead of indexing triggers by array position.
- `apps/web/app/office/routines/routines-content.tsx`: `handleCreate`'s
  independent trigger-arm error path, including guarding the post-error list
  refresh so a failed refetch cannot swallow the reported error or leave the
  create dialog open.
- Focused unit tests for every file above.

## Out of scope

- Go DTO changes (Office's wire format stays snake_case).
- Trigger firing/scheduling (`shared.NextCronTime`) and routine status gating.
- Unifying the list row's and detail card's two distinct firing-suppression
  inputs.

## Dependencies

None.

## Parallelism

Sequential. The wire adapter, the selector, and the create-error path share
one contract and were verified together across review rounds.

## Inputs

- `docs/specs/office/requirements/routine-trigger-wire-contract.md`.
- `docs/specs/office/system-design/routine-trigger-wire-contract-01.md`.

## Output contract

Report the root cause, files changed, RED/GREEN evidence, required
verification results, commit receipts, remaining risks, and task/plan status.

## Verification

- `cd apps && pnpm --filter @kandev/web run test`
- `cd apps/web && pnpm run typecheck`
- `make fmt`
- `make lint`

## Results

- RED: normalizer tests failed before the adapter existed (every UI-read field
  was `undefined`); create-serialization tests failed because the wire body
  carried camelCase keys the backend does not bind; row/detail tests failed
  because they read triggers by array position instead of the selector.
- GREEN: `apps/web/lib/api/domains/office-routine-normalize.test.ts`,
  `apps/web/app/office/lib/routine-trigger-selection.test.ts`,
  `apps/web/app/office/routines/routine-row.test.tsx`, and
  `apps/web/app/office/routines/routines-content.test.tsx` pass.
- GREEN: `pnpm --filter @kandev/web run test` (full suite).
- GREEN: `make fmt`, `cd apps/web && pnpm run typecheck`, `make lint`.
- Commits: `1bb0f0dda` (wire the contract), `3d3e9e358` (round-1 test-rigor
  gaps), `9ea3e7fcd` (report AC-002.8 independently of refresh outcome),
  `70d417c5c` (round-3 test-rigor gaps), `1b657ac11` (guard the create
  success-path refresh, round-5 test-rigor gap).
- Remaining risk: none open. Five review rounds found no residual production
  defect; the two remaining test-rigor gaps identified in round 5
  (id/time-order confounding in the primary-selection fixtures, and
  `CreateRoutineTriggerInput` narrowing having no direct test) are documented
  in the task's Kandev plan as a follow-up rather than blocking this delivery.
