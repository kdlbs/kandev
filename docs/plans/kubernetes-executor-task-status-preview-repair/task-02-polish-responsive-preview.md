---
id: "02-polish-responsive-preview"
title: "Polish the responsive executor preview"
status: complete
wave: 2
depends_on: ["01-hydrate-shared-status"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 02: Polish the responsive executor preview

## Acceptance

- Fine-pointer Kanban/sidebar executor glyphs match the task PR glyph's
  focusable controlled-trigger behavior and bounded structured-summary anatomy.
- The shared summary shows identity, semantic state, Kubernetes restarts and
  workspace when present, relative creation/check times, loading, and sanitized
  failure without an unstructured sentence stack or color-only meaning.
- Coarse pointers use a safe-area-aware, internally scrolling Drawer with the
  same facts and an effective 44 px trigger that does not select the task row.
- Extracting the domain-neutral tooltip-state hook leaves PR and registered
  change-request task indicators behaviorally unchanged; all new copy is fully
  localized.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/task/remote-cloud-tooltip.test.tsx components/task/remote-executor-task-status-summary.test.tsx components/github/pr-task-icon.render.test.tsx components/integrations/registered-change-request-task-icon.test.tsx && cd web && pnpm run typecheck && pnpm run i18n:zh-hant && pnpm run i18n:check && pnpm run i18n:ratchet && cd ../.. && git diff --check
```

## Files likely touched

- `apps/web/components/task/remote-cloud-tooltip.tsx`
- `apps/web/components/task/remote-cloud-tooltip.test.tsx`
- `apps/web/components/task/remote-executor-task-status-summary.tsx` (new)
- `apps/web/components/task/remote-executor-task-status-summary.test.tsx` (new)
- `apps/web/components/task/use-task-icon-tooltip-state.ts` (new)
- `apps/web/components/integrations/use-change-request-task-tooltip-state.ts`
- focused PR and registered-change-request task-icon tests
- `apps/web/components/task/task-item.tsx` and `apps/web/components/kanban-card-content.tsx` only if their row/card activation boundary needs an explicit integration seam
- `apps/web/src/locales/{en,pseudo,pt-pt,zh-cn,zh-hk,zh-tw}/task.json`

## Dependencies

Task 01.

## Parallelism

`sequential`. This task owns the component and selector contract used by Task
03 E2E coverage.

## Inputs

- Spec: fine-pointer structured summary and coarse-pointer Drawer scenarios.
- Plan: PR-quality responsive disclosure.
- Existing patterns: `PRTaskIcon`, `ChangeRequestTaskStatusSummary`,
  `useChangeRequestTaskTooltipState`, `useTouchDrawer`,
  `ChangeRequestStatusDrawerContent`, and the executor-settings Drawer.

## Output contract

Record visual/interaction RED assertions, GREEN component and localization
commands, desktop and touch hit-target behavior, accessibility checks, files
changed, blockers/risks, and synchronize this task plus `plan.md`.

## Results

- RED showed the old passive glyph lacked the structured identity/row anatomy
  and coarse-pointer dialog semantics. A final portal regression proved that
  closing the touch Drawer invoked the containing task row once.
- GREEN added the shared summary, controlled focus/hover/Escape behavior,
  semantic loading/healthy/error presentation, viewport-bounded fine-pointer
  surface, and a safe-area-aware touch Drawer with an effective 44 px target.
- The Drawer interaction boundary now stops click, keyboard, mouse, pointer,
  and touch events from bubbling through React portals into task selection.
- Mobile task projection now carries the exact executor ID, and PR plus
  registered change-request icons retain their existing behavior through the
  shared domain-neutral tooltip-state hook.
- The sidebar no longer applies a child-level muted color that overrides the
  trigger's eagerly hydrated healthy/error tone; E2E pins the rendered SVG
  against that regression.
- All six locale catalogs pass generation, completeness, placeholder, and
  ratchet checks. Focused Vitest, typecheck, full lint, and Prettier pass. No
  blocker remains.
