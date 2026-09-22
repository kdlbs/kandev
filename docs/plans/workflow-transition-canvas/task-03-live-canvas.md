---
id: "03-live-canvas"
title: "Publish the live workflow canvas"
status: pending
wave: 3
depends_on:
  - "02-host-api"
plan: "plan.md"
requirements:
  - REQ-PLUGINS-WORKFLOW-HISTORY-003
  - REQ-PLUGINS-WORKFLOW-HISTORY-002
acceptance_criteria:
  - AC-PLUGINS-WORKFLOW-HISTORY-002.4
  - AC-PLUGINS-WORKFLOW-HISTORY-003.1
  - AC-PLUGINS-WORKFLOW-HISTORY-003.2
  - AC-PLUGINS-WORKFLOW-HISTORY-003.3
  - AC-PLUGINS-WORKFLOW-HISTORY-003.4
  - AC-PLUGINS-WORKFLOW-HISTORY-003.5
  - AC-PLUGINS-WORKFLOW-HISTORY-003.6
system_design:
  - ../../specs/plugins/system-design/workflow-transition-history.md
---

# Task 03: Publish the live workflow canvas

## Summary

Replace the existing canvas's sample path with recorded task history. Add a
workspace overview that becomes active after user-controlled promotion, then
publish a validated release for the current task.

## In scope

- Edit only the source root assigned to canvas
  `65d40cfb-7bd3-407a-beb7-be78a551e62a` and republish it through
  `publish_canvas_kandev` after local verification.
- Use scoped relative Kandev data routes, fetch every page needed for a
  complete visible count, poll only while visible, and offer manual refresh.
- Label current-order backward moves and unknown/removed endpoints honestly.
  Keep sound opt-in and ignore initial-load and reconnect differences.
- Implement desktop beside-trail layout and phone Steps/Trail focus with
  visible selectors, 44 px controls, keyboard focus, safe-area padding, and
  bundled localized copy.
- Add disposable canvas E2E fixtures for route and mobile behavior, plus a
  local exact-source Playwright smoke script.

## Out of scope

- Automatically promoting the canvas, changing tasks, or accessing the DB
  directly. The final response must distinguish the active task release from
  the workspace view that awaits user promotion.

## Acceptance

1. The task canvas shows its real ledger trail and current task; no sample
   count or move is presented as live evidence.
2. A promoted workspace canvas can select a workflow/task and show route
   counts, current task counts, and trail; phone and desktop reach the same
   information with distinct layouts.
3. Loading, empty, older-host, denied, and retry states work; sound is opt-in;
   a watched move refreshes within 15 seconds; publication reports an active
   release or a precise failure.

## ASCII UI preview

`UI-01: Task trail` and `UI-03: Phone focus` from the
[full plan](plan.md#ascii-ui-preview) are the implementation target:

```text
Desktop: [ordered steps + current marker] | [recorded trail, newest first]
Phone:   [workflow/task context] [Steps | Trail] -> one focused scroll view
States:  Loading | Empty | Older host | Permission denied | Error + Retry
```

`UI-02: Workspace overview` adds the workflow picker, current task counts,
historical route counts, and task selection after promotion. The phone view
keeps the same data behind the visible selectors and Steps/Trail controls.
These are structural requirements for `AC-PLUGINS-WORKFLOW-HISTORY-003.2`,
`.5`, and `.6`; exact spacing is illustrative.

## Verification

```bash
node --check .kandev/canvases/65d40cfb-7bd3-407a-beb7-be78a551e62a/script.js
node apps/web/e2e/tests/canvas/workflow-canvas-source-smoke.cjs .kandev/canvases/65d40cfb-7bd3-407a-beb7-be78a551e62a
make -C apps/backend build
(cd apps/web && pnpm run build:e2e)
make -C apps/backend e2e-plugin-package
(cd apps/web && pnpm e2e:run --host --project=chromium tests/canvas/workflow-transition-history.spec.ts)
(cd apps/web && pnpm e2e:run --host --project=mobile-chrome tests/canvas/mobile-workflow-transition-history.spec.ts)
```

After checks, call `publish_canvas_kandev` with the assigned source path and
record its release ID and activation status in this work order. Publication
does not promote the canvas; the user controls workspace scope.

## Files likely touched

- `.kandev/canvases/65d40cfb-7bd3-407a-beb7-be78a551e62a/manifest.yaml`
- `.kandev/canvases/65d40cfb-7bd3-407a-beb7-be78a551e62a/index.html`
- `.kandev/canvases/65d40cfb-7bd3-407a-beb7-be78a551e62a/script.js`
- `.kandev/canvases/65d40cfb-7bd3-407a-beb7-be78a551e62a/styles.css`
- `apps/web/e2e/tests/canvas/workflow-transition-history.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-workflow-transition-history.spec.ts`
- `apps/web/e2e/tests/canvas/workflow-canvas-source-smoke.cjs`

## Dependencies

Task 02's browser data routes and docs.

## Risks

The source root is task-local and ignored by Git. Keep the exact-source smoke
result and published release ID in the work-order Results. A workspace scope
change needs explicit user promotion and possibly grant review.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/workflow-transition-history.md)
- [System design](../../specs/plugins/system-design/workflow-transition-history.md)
- [Complete UI preview](plan.md#ascii-ui-preview)
- Existing task canvas release `051c4ea1-1f59-4589-ad64-bd6204e8edad`

## Results

Pending.
