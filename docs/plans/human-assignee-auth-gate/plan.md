---
created: 2026-09-05
status: complete
requirements:
  - REQ-TASKS-HUMAN-ASSIGNEE-001
system_design:
  - ../../specs/tasks/system-design/human-assignee.md
legacy_specs: []
---

# Implementation Plan: Human Assignee Authentication Gate

## Overview

Hide the human-assignee feature unless authentication is enabled and a real
user is present. Apply one auth-mode rule to the task topbar, the Office
property row, and kanban card indicators.

The backend intentionally returns a synthetic `default-user` when
authentication is disabled. The affected components test only for user
presence, so they expose a team feature on single-user installations.

## Scope

### In scope

- Gate every human-assignee control and indicator on enabled authentication.
- Preserve the current enabled-mode assignment and takeover behavior.
- Avoid directory requests when the feature is hidden.
- Add component and browser regression coverage for the real disabled-mode
  auth payload.

### Out of scope

- Changes to authentication mode resolution or the synthetic identity.
- Changes to assignment storage, APIs, events, or workspace-reach rules.
- Clearing assignments when authentication is disabled.
- Changes to task-write permissions or viewer behavior.
- Adding a human-assignee control to the mobile task topbar.

## Technical approach

### Shared exposure rule

Use `auth.mode === "enabled"` and a non-null `auth.user` as the exposure
condition. Do not use `auth.authenticated` because it is true in disabled
mode.

### Task topbar and Office properties

Update `TaskAssigneeControl` in
`apps/web/components/task/task-assignee-control.tsx`. Return no content before
the directory hook becomes active when the exposure condition is false.

Update `HumanAssigneeRow` in
`apps/web/components/task/simple/task-properties.tsx`. Hide the complete
property row in disabled, setup, and anonymous states.

### Kanban card indicators

Update `KanbanCardBadges` and `hasCardBadges` in
`apps/web/components/kanban-card-content.tsx`. Exclude the human-assignee
badge and its row contribution when the exposure condition is false.

Keep `AssigneeBadge` presentational. It shall not own auth-mode decisions or
request directory data for a hidden badge.

## Tests

- Extend
  `apps/web/components/task/task-assignee-control.test.tsx` with the synthetic
  disabled-mode payload. The control stays absent and directory APIs are not
  called.
- Add focused coverage for `HumanAssigneeRow` in
  `apps/web/components/task/simple/task-properties.test.tsx`. Disabled and
  setup modes hide the complete row. Enabled mode retains the picker.
- Extend `apps/web/components/kanban-card-assignee.test.tsx`. A persisted
  assignee remains hidden in disabled mode and visible in enabled mode.
- Keep the existing enabled-mode assignment tests unchanged.

## E2E tests

- Add `apps/web/e2e/tests/task/human-assignee-auth-gate.spec.ts`. Use the
  default disabled-auth fixture and a task assigned to `default-user`.
- Open the task page and prove that `task-assignee-control` is absent.
- Open the kanban board and prove that `kanban-card-assignee` is absent.
- Extend `apps/web/e2e/tests/office/property-pickers.spec.ts`. Prove that the
  human-assignee property row is absent on the disabled-auth fixture.

These tests cover `AC-TASKS-HUMAN-ASSIGNEE-001.9` through the user interface.

## Mobile parity

The task-topbar control is desktop-only. The mobile task topbar has no matching
control, so this change introduces no new mobile surface.

The Office property row uses one component at every viewport. The auth gate
runs before its shared presentation. No layout, touch target, scroll owner, or
safe-area behavior changes.

Focused component tests and browser regressions cover the shared gate. A mobile
Playwright test would repeat the same condition without a distinct mobile code
path, so this change does not add one.

## Work orders

- [x] [Task 01: Gate human-assignee surfaces](task-01-gate-human-assignee-surfaces.md)

## Verification results

- Added one shared auth-mode exposure rule for all human-assignee surfaces.
- Added component coverage for disabled, setup, anonymous, and enabled states.
- Added browser coverage for the task topbar, kanban card, and Office property
  row on an authentication-disabled installation.
- Passed 27 focused component tests, both focused browser tests, TypeScript,
  frontend lint, specification validation, and `git diff --check`.

## Risks

- A user-presence check can reintroduce the bug because disabled mode supplies
  a synthetic user.
- A hidden card badge can still create an empty badge row unless
  `hasCardBadges` uses the same exposure condition.
- The fix must not erase an existing assignment when auth mode changes.

## Documentation

No public documentation change is required.
`docs/public/team-access.md` already says that authentication-disabled
installations hide this feature.
