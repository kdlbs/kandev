---
id: "12-unified-files-workspace-actions"
title: "Unified Files workspace actions"
status: done
wave: 7
depends_on:
  - "07-files-panel-surface"
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 12: Unified Files workspace actions

## Acceptance

- Files shows one labeled/icon workspace-actions trigger instead of separate **Add sources** and
  **Open workspace folder** controls. Its menu exposes both actions on desktop, tablet, and phone.
- The trigger and **Open workspace folder** remain enabled when source attachment is blocked.
  **Add sources** alone is disabled and its existing busy/loading/ineligible reason is visible in
  the menu.
- Selecting Add sources closes the menu and opens the existing Dialog/Drawer without focus
  contention; dismissal returns focus to the combined trigger. The phone trigger and menu rows have
  at least a 44px active dimension.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run \
  components/task/file-browser-toolbar.test.tsx \
  components/task/add-workspace-sources/add-workspace-sources-availability.test.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web exec eslint \
  components/task/file-browser-toolbar.tsx \
  components/task/file-browser.tsx \
  components/task/files-panel.tsx \
  components/task/task-files-panel.tsx \
  --max-warnings=0
```

## Files likely touched

- `apps/web/components/task/file-browser-toolbar.tsx`
- `apps/web/components/task/file-browser-toolbar.test.tsx`
- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/files-panel.tsx`
- `apps/web/components/task/task-files-panel.tsx`
- `apps/web/components/task/add-workspace-sources/add-workspace-sources-availability.ts`

## Dependencies

Task 07.

## Inputs

- Spec: combined action-menu requirements and active-turn scenario.
- Plan: **Frontend → Files panel surface and live state** and **Mobile design contract**.
- Patterns: `components/editors/file-actions-dropdown.tsx`,
  `components/app-sidebar/sections/tasks-view-picker.tsx`, and the responsive Radix menu rules in
  `app/globals.css`.

## Output contract

- Update only this task file to `in_progress` when starting and `done` after acceptance and
  verification pass.
- Return a compact handoff capsule: accepted behaviors, base/head SHA, changed files and entry
  points, targeted command results, risk tags, and any uncertainty.
- Do not update `plan.md`; the planner serializes shared-plan status.
