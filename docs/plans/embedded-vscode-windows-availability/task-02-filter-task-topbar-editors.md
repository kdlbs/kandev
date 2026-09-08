---
id: "02-filter-task-topbar-editors"
title: "Filter task-topbar editors"
status: done
wave: 2
depends_on: ["01-expose-backend-host-os"]
plan: "plan.md"
spec: "../../specs/ui/requirements/embedded-vscode-windows-availability.md"
---

# Task 02: Filter task-topbar editors

## Acceptance

- When backend `hostOS` is `windows`, the task-topbar editor set excludes only editors with
  `kind === "internal_vscode"` while preserving the existing enabled/installed rules.
- The primary action resolves its saved default against that filtered set, falls back to the first
  compatible editor, and resolves to no editor when the set is empty.
- Non-Windows and missing backend host values retain embedded-editor availability; browser platform
  detection is not used.

## Verification

Use TDD: add focused host-platform and default-fallback cases, run them to observe failure,
implement the helper and component wiring, then rerun:

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/editors-menu-availability.test.ts
```

## Files likely touched

- `apps/web/components/task/editors-menu-availability.ts`
- `apps/web/components/task/editors-menu-availability.test.ts`
- `apps/web/components/task/editors-menu.tsx`

## Dependencies

- Task 01 must be complete.

## Parallelism

Sequential. This task owns the production behavior consumed by Task 03.

## Inputs

- Spec sections: **What**, **API surface**, and scenarios 1–5.
- Plan sections: **Topbar editor availability**, **Tests**, and **Risks**.
- Existing patterns:
  - `apps/web/components/task/editors-menu.tsx`
  - `apps/web/src/boot-payload.ts`
  - `apps/backend/internal/editors/discovery/editors.json`

## Risks

- Filtering only the rendered dropdown would leave the saved embedded default launchable through
  the primary split button.
- Matching by display name is unstable; the platform rule must use the existing
  `internal_vscode` kind.

## Output contract

Report the behavior implemented, files changed, red and green test commands/results, blockers or
risks, and update this task to `done` plus its checkbox/status in `plan.md`.
