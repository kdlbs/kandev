---
id: "02-dialog-setting"
title: "Blocker setting in the Linear watch dialog"
status: done
wave: 2
depends_on:
  - "01-hold-blocked-issues"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-LINEAR-BLOCKERS-002
acceptance_criteria:
  - AC-INTEGRATIONS-LINEAR-BLOCKERS-002.1
system_design:
  - ../../specs/integrations/system-design/linear-watch-blockers.md
---

# Task 02: Blocker Setting in the Linear Watch Dialog

## Summary

Add a switch for `requireBlockersDone` to the watch dialog, keep it through
the form round trip, and mark such watches in the watch table.

## In scope

- `LinearSearchFilter.requireBlockersDone`, `FormState.requireBlockersDone`,
  `formStateFromWatch`, `buildFilterPayload`.
- The switch in `FilterFields`, the `blockers:done` table summary, and locales.

## Out of scope

- Showing held issues or their blockers.

## ASCII UI preview

See `UI-01` in the [plan](plan.md#ascii-ui-preview): a switch row under Query.

## Acceptance

1. Saving stores the setting; reopening and saving keeps it; team and
   workspace changes keep it; a flag-only filter does not enable Save.
2. Desktop and phone flows show the setting on after save and reopen; on
   phones the switch has a 44 px hit area.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/linear/linear-issue-watch-form.test.ts components/linear/linear-issue-watch-fields.test.tsx)
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/integrations/linear-watcher-blockers.spec.ts
pnpm --dir apps/web e2e:run --host --shards 1 --project mobile-chrome tests/integrations/mobile-linear-watcher-blockers.spec.ts
```

## Files likely touched

- `apps/web/components/linear/linear-issue-watch-{filter-fields,form,table}.ts(x)` and tests
- `apps/web/lib/types/linear.ts`, `apps/web/src/locales/*/linear.json`
- `apps/web/e2e/tests/integrations/*blockers*`

## Dependencies

Task 01.

## Parallelism

`sequential`

## Results

Author-reported: the form and filter-field unit tests pass. Both Playwright
specs pass locally on `chromium` and `mobile-chrome`.
