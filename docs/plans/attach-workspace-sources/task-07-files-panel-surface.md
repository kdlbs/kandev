---
id: "07-files-panel-surface"
title: "Files panel surface"
status: done
wave: 4
depends_on: ["03-protocol-surfaces", "06-shared-source-picker"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 07: Files Panel Surface

## Acceptance

- Eligible desktop and mobile Files panels expose an accessible Add sources action, with archived,
  repository-less, loading, and active-turn states handled explicitly.
- Desktop Dialog and phone full-height Drawer share logic, support mixed rows, retain retryable input
  on failure, and meet the documented scroll, safe-area, focus, touch-target, and overflow contract.
- Successful task/session events merge repository/folder state, adopt the new workspace path, reset
  affected file-tree caches, and refresh Files/Changes/editor surfaces without a page reload.

## Verification

```bash
cd apps/web
rtk pnpm test -- components/task/add-workspace-sources components/task/file-browser lib/ws/handlers
rtk pnpm run typecheck
```

## Files likely touched

- `apps/web/components/task/add-workspace-sources/**` (new)
- `apps/web/components/task/file-browser-toolbar.tsx`
- `apps/web/components/task/file-browser.tsx`
- `apps/web/components/task/files-panel.tsx`
- `apps/web/components/task/task-files-panel.tsx`
- `apps/web/components/task/task-files-panel-hooks.ts`
- `apps/web/lib/ws/handlers/tasks.ts`
- `apps/web/lib/ws/handlers/agent-session.ts`
- `apps/web/lib/state/slices/kanban/**`
- `apps/web/lib/state/slices/session-runtime/**`

## Dependencies

Tasks 03 and 06.

## Inputs

- Plan: Frontend and Mobile design contract.
- Mobile exemplar: `components/task/mobile/mobile-picker-sheet.tsx`.
- Existing Files toolbar and mobile/tablet task layouts.

## Output contract

Summary, files changed, tests run, rendered desktop/mobile checks, blockers, risks, divergence, and
task/plan status updates.
