---
id: "01-draft-pr-icon-color"
title: "Restore muted draft PR icon precedence"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PR-TASK-STATUS-SUMMARY-001
acceptance_criteria:
  - AC-UI-PR-TASK-STATUS-SUMMARY-001.20
system_design:
  - ../../specs/ui/system-design/pr-task-status-summary.md
---

# Task 01: Restore muted draft PR icon precedence

## Summary

Correct the shared GitHub task-row icon color precedence so an open draft pull
request remains muted when CI reports failure. Prove the correction with a
pure-helper regression and a rendered sidebar scenario while preserving the
existing mobile passive-indicator path.

## In scope

- Add the failing draft-plus-failing-CI regression before production changes.
- Move only the draft branch in `getPRStatusColor`.
- Add the focused desktop rendered assertion and run the existing mobile check.

## Out of scope

- `PRStatusChip` CI status derivation.
- GitHub synchronization, API contracts, merge actions, and queue behavior.
- New mobile controls, layout changes, or translations.

## Acceptance

- An open draft PR with failing checks renders the task-row icon with
  `text-muted-foreground`.
- A non-draft PR with failing checks remains red, and terminal/queued precedence
  remains covered by the existing tests.
- The shared desktop icon and existing mobile task-row flow pass their focused
  checks without document overflow or changed navigation behavior.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  components/github/pr-task-icon-draft.test.ts \
  components/github/pr-task-icon.test.ts
cd apps/web && pnpm e2e:run tests/pr/pr-status-badge.spec.ts \
  -- --grep "draft PR icon"
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-task-status-summary.spec.ts
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/components/github/pr-task-icon.tsx`
- `apps/web/components/github/pr-task-icon-draft.test.ts`
- `apps/web/e2e/tests/pr/pr-status-badge.spec.ts`

## Dependencies

None.

## Risks

- Draft must remain below terminal and active-queue checks but above failure
  coloring for non-terminal task-icon presentation.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/pr-task-status-summary.md`, especially
  `AC-UI-PR-TASK-STATUS-SUMMARY-001.5` and `.20`.
- `docs/specs/ui/system-design/pr-task-status-summary.md`, especially the
  status-color precedence section and mobile behavior.
- Existing `getPRStatusColor` and draft/PR status tests.

## Results

- TDD RED unit run reproduced the bug: the draft plus failing-CI case returned
  `text-red-500`.
- TDD RED desktop E2E run reproduced the rendered sidebar failure: the icon
  received `text-red-500` instead of `text-muted-foreground`.
- Focused frontend unit tests passed: 2 files, 72 tests.
- Desktop Chromium E2E passed: 1 draft PR icon regression test.
- Mobile Chromium E2E passed: 1 existing task-status-summary test.
- Frontend TypeScript typecheck passed.
