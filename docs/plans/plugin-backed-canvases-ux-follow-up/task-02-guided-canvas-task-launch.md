---
id: "02-guided-canvas-task-launch"
title: "Add guided canvas task launch"
status: done
wave: 2
depends_on:
  - "01-workspace-canvas-discovery"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-001
  - REQ-CANVASES-AGENT-WEB-APPS-009
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-001.7
  - AC-CANVASES-AGENT-WEB-APPS-009.1
  - AC-CANVASES-AGENT-WEB-APPS-009.2
  - AC-CANVASES-AGENT-WEB-APPS-009.3
  - AC-CANVASES-AGENT-WEB-APPS-009.4
  - AC-CANVASES-AGENT-WEB-APPS-009.5
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 02: Add guided canvas task launch

## Summary

Open the standard task creation flow from workspace Canvases settings. Apply
one canvas preset without hiding the normal workflow and agent choices.

## In scope

- Add scratch source mode and local executor preference to task dialog presets.
- Add one localized `CanvasTaskCreateLauncher`.
- Keep the sidebar empty state as a setup link to workspace Canvases settings.
- Add the workspace settings primary action.
- Keep workflow, workflow step, agent profile, and compatible executor fields
  editable.
- Open the created task through the standard success path.
- Cover desktop and phone launch behavior.

## Out of scope

- Direct canvas creation, automatic publication, and panel activation.

## Acceptance

- The workspace settings entry point passes the title, prompt, source mode, and
  executor preference.
- No repository or explicit local path is attached to the new task.
- A user can change the workflow and agent profile before submission.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task-create-dialog-state.test.ts components/task-create-dialog-form-reset.test.ts components/canvas/canvas-task-create-launcher.test.tsx components/app-sidebar/sections/canvases-section.test.tsx components/settings/workspace-canvases-page.test.tsx
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm e2e:run tests/canvas/plugin-canvas.spec.ts -- --retries=0
cd apps/web && pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --retries=0
```

## Files likely touched

- `apps/web/components/task-create-dialog-types.ts`
- `apps/web/components/task-create-dialog-form-reset.ts`
- `apps/web/components/task-create-dialog-effects.ts`
- `apps/web/components/canvas/canvas-task-create-launcher.tsx`
- `apps/web/components/app-sidebar/sections/canvases-section.tsx`
- `apps/web/components/settings/workspace-canvases-page.tsx`
- `apps/web/src/locales/**/canvases.json`
- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`

## Dependencies

- Task 01 provides the final sidebar and workspace settings entry points.

## Risks

- A launch-only executor preference can become a persisted profile override.
- The no-repository preset can race the dialog's last-used restoration.

## Parallelism

`sequential`

## Inputs

- Guided canvas task launch and mobile design sections.
- ADR 0028 for task-create preference ownership.
- `CANVAS-UX-03` in the investigation.

## Results

Implemented the shared `CanvasTaskCreateLauncher` for workspace Canvases
settings. The launcher uses the standard task dialog with a scratch source and
direct-local executor preference, while leaving workflow, step, and agent
profile choices editable. The sidebar now provides an expanded empty-state
setup link to workspace settings instead of a header create shortcut. Added
state, reset, executor selection, and disabled-feature coverage plus localized
copy in all required catalogs.

Verification passed: focused web coverage included 6 files and 53 tests; web
typecheck, lint, i18n checks, and the new-code ratchet passed.

Follow-up verification passed: the sidebar, launcher, and workspace-settings
coverage passed with 14 tests, and changed-file lint, formatting, and
specification validation passed.
