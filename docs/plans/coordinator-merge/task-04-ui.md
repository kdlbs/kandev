---
id: "04-ui"
title: "Manager surfaces"
status: pending
wave: 4
depends_on: ["03-merge"]
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-MERGE-001
  - REQ-COORDINATOR-MERGE-002
  - REQ-COORDINATOR-MERGE-003
acceptance_criteria:
  - AC-COORDINATOR-MERGE-001.1
  - AC-COORDINATOR-MERGE-002.1
  - AC-COORDINATOR-MERGE-002.5
  - AC-COORDINATOR-MERGE-003.1
  - AC-COORDINATOR-MERGE-003.4
  - AC-COORDINATOR-MERGE-003.5
system_design:
  - ../../specs/coordinator/system-design/merge.md
---

# Task 04: Manager surfaces

## Summary

Settings repository table, Queue change state, predecessor editor, proposal card and outcomes on desktop and phone, with localized copy in all seven locales.

## In scope

- Deliver the listed acceptance criteria with the named tests, against the paired design.

## Out of scope

- Releases, branch pushes, provider bypass and the parallel answer proposal.

## Acceptance

- 001.1: Queue shows each field and "unavailable" with retry (merge.spec.ts "queue change state").
- 002.1/002.5: table, per-branch protections, save failure and retry (merge.spec.ts, mobile-merge.spec.ts).
- 003.1: editor refuses duplicates and shows stale conflict.
- 003.4: concurrent double click approves once; uncertain card has Check now and, after 24 hours, Release, and no Approve; proposal shows its owning coordinator.
- 001.1/002.1: "No checks" label and the settings note that such repositories cannot be merged and that merge-queue repositories are unsupported.
- 003.5: proposal card states the merge cannot be undone and has no Undo.

## ASCII UI preview

UI-01 settings and UI-02 Queue/proposal: see [plan previews](plan.md#ascii-ui-preview).

```text
Phone: service-a
Merge policy [Requires approval v]
Pipeline: enforced | Reviews: unknown [Retry]
Conditions not enforced by the provider can change before merge acceptance.
[Save]
```

Use stacked cards with a single vertical scroll owner and accessible touch controls.

## Verification

```bash
make build-web build-backend
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/coordinator/merge.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/coordinator/mobile-merge.spec.ts)
```

## Files likely touched

- apps/web/components/coordinator; apps/web/src/locales; apps/web/e2e/tests/coordinator

## Dependencies

03-merge

## Risks

Provider reads may fail or race; preserve the fail-closed and uncertain-outcome rules.

## Parallelism

sequential

## Inputs

- [Requirements](../../specs/coordinator/requirements/merge.md) and [design](../../specs/coordinator/system-design/merge.md).

## Results

Pending.
