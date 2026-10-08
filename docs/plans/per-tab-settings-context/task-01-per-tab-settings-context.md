---
id: "01-per-tab-settings-context"
title: "Preserve per-tab Settings workspace context"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001
acceptance_criteria:
  - AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.1
  - AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.2
  - AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.3
  - AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.4
  - AC-WORKSPACES-PER-TAB-SETTINGS-CONTEXT-001.5
system_design:
  - ../../specs/workspaces/system-design/per-tab-settings-context.md
---

# Task 01: Preserve per-tab Settings workspace context

## Summary

Keep the workspace that is active in the current browser tab when Settings
loads. Use the shared cookie and saved preference only when the tab's workspace
is absent or invalid.

## In scope

- Validate the current tab, cookie, and saved preference against the loaded
  workspace list.
- Keep Office and Kanban workspaces eligible for Settings.
- Apply an active-workspace change through the store action that updates its
  revision.
- Cover fallback selection and Settings-to-Home navigation on desktop and
  phone.

## Out of scope

- Changing cookie writes, saved preference persistence, or backend APIs.
- Changing workspace-list loading or access rules.

## Acceptance

- A valid workspace already active in the tab takes priority during Settings
  hydration.
- Invalid or missing tab state falls back to the valid cookie, saved
  preference, first workspace, then no active workspace.
- A changed active ID uses `setActiveWorkspace`, and Settings does not replace
  a different shared cookie value or persist the tab-local selection.

## Verification commands

- `cd apps/web && pnpm exec vitest run lib/routing/route-bootstrap.test.ts src/settings-routes.test.ts src/settings-routes.bootstrap.test.ts src/settings-routes.workspace-revision.test.tsx`
- `(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/settings/workspace-tab-settings-home.spec.ts)`
- `(cd apps/web && pnpm e2e:run --host --project mobile-chrome e2e/tests/settings/mobile-workspace-tab-settings-home.spec.ts)`

## Dependencies

- [Per-tab Settings context plan](plan.md)
- [Per-tab Settings workspace context design](../../specs/workspaces/system-design/per-tab-settings-context.md)

## Results

Implemented in PR #4199. Focused unit tests passed (44 tests), desktop and
phone E2E cases passed, focused ESLint passed, and commit hooks passed.
The documentation coverage preflight, specification catalog validation, and
specification linters also passed before this work order was added to the PR.
