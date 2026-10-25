---
id: "01-frontend-submenu"
title: "Frontend: PR submenu in the + add-panel menu"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/add-panel-pr-submenu.md"
---

# Task 01: Frontend PR submenu

- **Acceptance:**
  1. `AddPanelMenuItems` renders linked PRs as a "Pull requests"
     `DropdownMenuSub` when `state.prs.length > 1`; a single PR still renders
     inline with label `PR #N`; zero PRs renders nothing.
  2. Sub-menu rows keep the `add-panel-pr-item-<owner>-<repo>-<number>` testids
     and the disambiguated `PR #N — owner/repo` labels; the sub-trigger carries
     `add-panel-pr-submenu`; selecting a row calls `addPRPanel` with
     `prTaskKey(pr)` and the active session id.
  3. New unit tests cover 0 / 1 / 2+ PR cases, submenu open, label assertions,
     and the click handler.
- **Verification:**
  - `cd apps && pnpm --filter @kandev/web test -- dockview-add-panel-items.test.tsx`
  - `cd apps/web && pnpm run typecheck`
  - `cd apps/web && pnpm run lint -- components/task/dockview-add-panel-items.tsx components/task/dockview-add-panel-items.test.tsx`
- **Files likely touched:**
  - `apps/web/components/task/dockview-add-panel-items.tsx`
  - `apps/web/components/task/dockview-add-panel-items.test.tsx` (new)
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:**
  - Spec `What`/`Scenarios`: docs/specs/ui/requirements/add-panel-pr-submenu.md
  - Plan Frontend + Tests sections: docs/plans/add-panel-pr-submenu/plan.md
  - Existing sub-menu pattern: `apps/web/components/task/handoff-profile-menu-items.tsx`
    (`DropdownMenuSub`/`SubTrigger`/`SubContent` usage)
  - Test-mocking pattern: `apps/web/components/task/task-pr-shortcut.test.tsx`
    (mocks `@/components/state-provider`, `useTaskPR`, `useTaskMRs`),
    `apps/web/components/task/new-session-dialog.test.tsx` (mocks
    `@/components/state-provider` with a full state object)
  - jsdom submenu-opening pattern: `fireEvent.pointerEnter`/`pointerMove` per
    `apps/web/components/task-create-dialog-pill.test.tsx` and
    `apps/web/components/task/executor-settings-button.test.tsx`
- **Output contract:** summary, files changed, exact verification commands with
  results, task status → `done`, plan checkbox update.
