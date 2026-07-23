---
id: "01-frontend-link-menu"
title: "GitLab MR contextual link action"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/gitlab-integration/spec.md"
---

# Task 01: GitLab MR Contextual Link Action

## Acceptance

- An unlinked task renders no generic GitLab `Link MR` action in desktop or
  mobile task top bars, while linked MR status/dropdown behavior is unchanged.
- A GitLab-configured workspace exposes `GitLab Merge Request` in the shared
  task `Link` submenu on kanban cards, desktop sidebar rows, and the mobile task
  switcher; unconfigured workspaces omit it.
- Selecting the action opens the existing `TaskMRLinkDialog` for the selected
  task and active workspace with that task's repositories.

## Files Likely Touched

- `apps/web/components/gitlab/mr-topbar-button.tsx`
- `apps/web/components/gitlab/mr-topbar-button.test.ts`
- `apps/web/components/kanban-external-link-availability.ts`
- `apps/web/components/kanban-card-menu-items.tsx`
- `apps/web/components/kanban-card-menu-items.test.tsx`
- `apps/web/components/kanban-card.tsx`
- `apps/web/components/task/task-switcher-context-menu.tsx`
- `apps/web/components/task/task-switcher.tsx`
- `apps/web/components/task/task-switcher.test.tsx`
- `apps/web/components/task/task-session-sidebar-link-actions.ts`
- `apps/web/components/task/task-session-sidebar-task-linking.ts`
- `apps/web/components/task/task-session-sidebar-dialogs.tsx`
- `apps/web/components/task/mobile/session-task-switcher-sheet.tsx`

## Inputs

- Spec `What` bullets and the three contextual-link/top-bar scenarios.
- Plan `Frontend`, `Mobile design contract`, and `Tests` sections.
- Existing GitHub menu callback plumbing and `TaskGitHubPRDialog` wiring.
- Existing `TaskMRLinkDialog`, `useGitLabAvailable`, and linked-MR top-bar
  dropdown behavior.

## Implementation Notes

- Follow `/tdd` and `/mobile-parity`; do not spawn subagents.
- Add the failing menu-policy/unit coverage before production changes.
- Keep the action label provider-specific: `GitLab Merge Request`.
- Preserve `Link another merge request` inside the linked-MR top-bar dropdown.
- Do not add backend endpoints, state slices, or a second GitLab link dialog.
- Update only this task file's status. Do not edit `plan.md`.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/kanban-card-menu-items.test.tsx components/task/task-switcher.test.tsx components/gitlab/mr-topbar-button.test.ts
cd apps/web && pnpm run typecheck
```

## Output Contract

Report the behavior implemented, files changed, RED/GREEN evidence, targeted
test and typecheck results, mobile parity observations, blockers, and residual
risks. Set this task to `in_progress` before code changes and `done` only after
acceptance and verification pass.
