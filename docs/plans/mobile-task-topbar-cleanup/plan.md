---
created: 2026-08-24
status: implemented
requirements:
  - REQ-UI-MOBILE-TASK-CHROME-001
system_design:
  - ../../specs/ui/system-design/mobile-task-chrome.md
legacy_specs:
  - ../../specs/ui/requirements/mobile-task-navigation.md
---

# Implementation Plan: Mobile Task Topbar Cleanup

## Overview

Remove Saved layouts and the Git ellipsis from the phone task top bar. Keep the
hamburger task drawer as the only task-level action entry, keep Git capability
in Changes, delete the resulting mobile-only Git/layout branches, and retarget
existing mobile tests to the surviving paths.

## Scope

### In scope

- Remove phone rendering of `LayoutPresetSelector` and Git action controls.
- Make the retained phone task-drawer trigger touch-sized.
- Remove mobile-only layout-selector branches and orphaned phone Git modules.
- Preserve branch and diff summary data in the phone title.
- Retarget Git and remote-contribution mobile E2E flows through Changes.
- Preserve task movement through the existing task drawer.

### Out of scope

- New task-level overflow or duplicated task actions.
- Changes-panel redesign or changed Git eligibility/operation semantics.
- Desktop/tablet top-bar or Dockview layout changes.
- Backend, API, store, persistence, permission, or localization changes.
- Reorganizing unrelated phone top-bar controls.

## Technical approach

### Phone top bar

Update `apps/web/components/task/mobile/session-mobile-top-bar.tsx` to stop
mounting `LayoutPresetSelector`, `GitActionsDropdown`, its dialogs, and
remote-contribution drawer. Remove their props, state, hooks, and callbacks
from the top-bar action composition. Keep `useMobileGitMetrics` for the visible
branch/diff summary, relocating its small file-stat aggregation from the deleted
Git-controls module. Give `mobile-session-menu` an explicit 44-by-44 hitbox.

### Desktop layout selector

Update `apps/web/components/task/layout-preset-selector.tsx` to remove the now
unused `mobile` prop and conditional branches. Keep desktop presets, custom
layout application, reset, save, deletion, coarse-pointer confirmation, and
settings behavior unchanged. Remove only the component test that asserted the
retired phone top-bar variant.

### Orphan cleanup

Delete mobile-only Git action modules and tests once `rg` confirms no remaining
production consumers:

- `apps/web/components/task/mobile/mobile-git-actions-dropdown.tsx`
- `apps/web/components/task/mobile/mobile-git-push-submenu.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-git-controls.test.tsx`
- `apps/web/components/task/mobile/session-mobile-top-bar-dialog-parts.tsx`
- `apps/web/components/task/mobile/mobile-contribution-resolution-drawer.tsx`
- `apps/web/components/task/mobile/mobile-contribution-resolution-drawer.test.tsx`

Update `apps/web/components/vcs/vcs-dialog-fields.test.tsx` to cover only the
surviving shared VCS fields. Do not remove shared Changes/VCS action policy,
dialogs, or remote-contribution components.

## Tests

- `apps/web/components/task/mobile/session-mobile-top-bar-repository.test.tsx`
  drops the obsolete remote-contribution mock while retaining repository-title
  coverage for the simplified header.
- `apps/web/components/task/layout-preset-selector.test.tsx` retains desktop and
  coarse-pointer saved-layout behavior while dropping the retired mobile-mode
  case.
- Existing shared Changes and VCS component tests remain green after orphan
  cleanup. Phone Playwright coverage asserts the retired controls are absent
  and the task-drawer control remains.

## E2E tests

- **Top-bar composition (`AC-UI-MOBILE-TASK-CHROME-001.1`, `.2`, `.5`):**
  extend `apps/web/e2e/tests/task/mobile-task-topbar-long-title.spec.ts` to
  assert no layout/Git triggers, a 44-by-44 task-drawer trigger, retained title
  truncation, action containment, and no document horizontal overflow.
- **Task action (`AC-UI-MOBILE-TASK-CHROME-001.3`):** retain and annotate the
  active-task move scenario in
  `apps/web/e2e/tests/task/mobile-sidebar-task-actions.spec.ts`.
- **Git operation (`AC-UI-MOBILE-TASK-CHROME-001.4`):** retarget
  `apps/web/e2e/tests/git/mobile-local-base-operations.spec.ts` from the removed
  top-bar menu to the Changes Pull menu and complete Rebase.
- **Remote safety (`AC-UI-MOBILE-TASK-CHROME-001.4`):** retarget
  `apps/web/e2e/tests/git/mobile-pr-checkout-drift.spec.ts` to the Changes
  warning menu and shared confirmation, preserving version, safety, scrolling,
  containment, and action-outcome assertions.
- **Provider terminology (`AC-UI-MOBILE-TASK-CHROME-001.4`):** retarget the
  GitLab MR-creation scenario in
  `apps/web/e2e/tests/gitlab/mobile-gitlab-parity.spec.ts` through Changes and
  preserve auto-link and viewport assertions.

## Work orders

- [x] [Task 01: Remove duplicate mobile topbar controls](task-01-remove-duplicate-mobile-topbar-controls.md)

## Verification results

- Four focused component files passed: 11 tests.
- Frontend TypeScript typecheck passed.
- Phone top-bar composition and long-title geometry passed: 1 Playwright test.
- Phone task movement through the retained drawer passed: 1 Playwright test.
- Phone local rebase and remote-contribution recovery through Changes passed: 2 Playwright tests.
- Review remediation added a 700px touch-viewport regression proving the
  recovery warning and confirmation actions stay at least 44 CSS pixels through
  the full phone breakpoint.
- Phone GitLab MR creation through Changes passed: 1 Playwright test.
- A captured Pixel 5 Changes state was inspected for top-bar composition,
  action placement, and horizontal containment; generated capture artifacts
  were then cleaned.
- The first build-backed task-movement run encountered a fixture startup
  `ECONNREFUSED` before reaching UI assertions. Its immediate no-build retry
  passed.

## Documentation impact

No public documentation changes are required. This changes responsive control
placement without changing a public command, setting, API, or documented
operator workflow.

## Risks

- Deleting phone-only dialogs must not delete the shared `VcsDialogsProvider`
  used by Changes.
- Existing Git E2E setup may assume the always-visible top-bar trigger. Each
  retargeted test must wait for the Changes state that makes its contextual
  action available, then prove the operation result rather than visibility
  alone.
- Remote-contribution confirmation changes entry path from the retired mobile
  drawer to the shared Changes confirmation. The test must preserve exact-head
  safety and phone viewport containment.
