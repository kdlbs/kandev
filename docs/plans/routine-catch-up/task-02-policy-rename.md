---
id: "02-policy-rename"
title: "Rename the catch-up policy across every surface"
status: done
wave: 1
depends_on: ["01-tick-and-gap"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-ROUTINE-CATCHUP-003
acceptance_criteria:
  - AC-OFFICE-ROUTINE-CATCHUP-003.1
  - AC-OFFICE-ROUTINE-CATCHUP-003.2
  - AC-OFFICE-ROUTINE-CATCHUP-003.3
  - AC-OFFICE-ROUTINE-CATCHUP-003.4
  - AC-OFFICE-ROUTINE-CATCHUP-003.5
  - AC-OFFICE-ROUTINE-CATCHUP-003.6
  - AC-OFFICE-ROUTINE-CATCHUP-003.7
  - AC-OFFICE-ROUTINE-CATCHUP-003.8
system_design:
  - ../../specs/office/system-design/routine-catch-up-02.md
---

# Task 02: Rename the catch-up policy across every surface

## Summary

`enqueue_missed_with_cap` is a false statement of what the code does (fires
once, never enqueues N runs). Rename it to `summarize_missed` everywhere it is
written or displayed, while accepting the old value forever as a deprecated
alias so existing rows and API callers keep working.

## In scope

- `apps/backend/internal/office/models/catchup.go`: the policy enum,
  `NormaliseCatchUpPolicy` (accepts the deprecated alias on write, normalizes
  on read), the one-time upgrade migration for pre-existing rows.
- `apps/web/app/office/routines/{create-routine-dialog.tsx,
  [id]/routine-detail-view.tsx}` and `apps/web/src/locales/*/office.json`
  (5 languages + pseudo): the policy label, including the "(wakes once)"
  qualifier.
- `docs/specs/office/system-design/scheduler-01.md` /
  `scheduler-02.md`: correct the design-doc prose that previously matched the
  misdescribing name.

## Acceptance

- The policy value is `summarize_missed`; `enqueue_missed_with_cap` is
  accepted as a deprecated alias on every write path and normalized away on
  every read path, including a one-time upgrade pass over existing rows.
- The web UI's catch-up policy label states, in every required locale, that
  the policy wakes once rather than enqueueing per missed tick.
- `scheduler-01.md` and `scheduler-02.md` no longer describe the retired
  enqueue-N-runs behavior.

## Verification

```bash
cd apps/backend && go test ./internal/office/models/... ./internal/office/repository/sqlite/... -run CatchUpPolicy -count=1
cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm vitest run app/office/routines
python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/backend/internal/office/models/catchup.go`
- `apps/web/app/office/routines/create-routine-dialog.tsx`
- `apps/web/app/office/routines/[id]/routine-detail-view.tsx`
- `apps/web/src/locales/*/office.json`
- `docs/specs/office/system-design/scheduler-01.md`
- `docs/specs/office/system-design/scheduler-02.md`

## Dependencies

Depends on task 01 for the `Routine.CatchUpPolicy` plumbing this rename reads
and writes.

## Parallelism

Sequential with task 01 (shared frontend control and Go enum).

## Results

Implemented with TDD: policy rename plus deprecated-alias normalization on
both write and read paths, upgrade migration for pre-existing rows, frontend
label update across both routine dialogs and all required locales (including
the "(wakes once)" qualifier added during PR review), and both scheduler
design docs corrected. `i18n:check` / `i18n:ratchet` clean; full frontend
vitest suite green.
