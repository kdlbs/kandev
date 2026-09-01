---
id: "02-prove-watcher-branch-selection"
title: "Prove watcher branch selection"
status: done
wave: 2
depends_on:
  - "01-expose-qualified-remote-refs"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001
acceptance_criteria:
  - AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.2
  - AC-INTEGRATIONS-WATCHER-REMOTE-BASE-BRANCHES-001.5
system_design:
  - ../../specs/integrations/system-design/watcher-remote-base-branches.md
---

# Task 02: Prove Watcher Branch Selection

## Summary

Prove through Jira's real watcher dialog that `origin/main` can be selected,
saved, and restored on desktop and phone. Reuse the existing responsive dialog
and the shared searchable branch-picker composition.

## In scope

- Add the desktop Jira watcher persistence scenario.
- Add the matching mobile touch scenario.
- Add only the stable selectors, page-object methods, or shared test helper
  needed by those scenarios.
- Assert phone dialog containment and no document horizontal overflow.

## Out of scope

- Running a real Jira service.
- Starting the generated watcher task in Playwright; backend and worktree unit
  tests own the existing launch contract.
- Redesigning the phone watcher dialog.

## Acceptance

- Desktop selects and reloads `origin/main` through the Jira watcher UI.
- Mobile completes the same outcome with touch interaction.
- The mobile dialog and branch popover stay contained, branch rows remain
  touch-sized, the list scrolls internally, and no horizontal page overflow is
  introduced.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/integrations/jira-settings.spec.ts -- --grep "qualified remote branch"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-jira-watcher-branches.spec.ts
```

## Files likely touched

- `apps/web/components/watcher-repository-fields.tsx`
- `apps/web/components/jira/jira-issue-watch-dialog.tsx`
- `apps/web/e2e/pages/jira-settings-page.ts`
- `apps/web/e2e/tests/integrations/jira-settings.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-jira-watcher-branches.spec.ts`
- `apps/web/e2e/tests/integrations/jira-watcher-branch-flow.ts`

## Dependencies

- Task 01 supplies distinct qualified remote-ref values.

## Risks

- The watcher form requires workflow and step selections; the E2E helper must
  seed or select them without relying on remembered cross-test state.

## Parallelism

`sequential`

## Inputs

- Task 01 output.
- Jira settings page object and integration mock fixture.
- Mobile UI language and E2E fixture guidance.

## Results

Added a shared Jira watcher flow and desktop and mobile specs. The flow
selects `origin/main`, verifies the exact persisted API value, reloads the
watcher, and verifies the qualified option remains selected. The mobile
coverage also verifies touch interaction, dialog and popover containment,
touch-sized options, internal scrolling, and no document horizontal overflow.
The shared flow and temporary branch setup clean up only the records and
branches they create, including when an assertion fails.

Both verification commands passed after rebuilding the backend and frontend:
the desktop scenario passed and the mobile scenario passed.
