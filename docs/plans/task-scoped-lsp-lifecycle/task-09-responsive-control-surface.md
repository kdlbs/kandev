---
id: "09-responsive-control-surface"
title: "Responsive task control surface"
status: completed
wave: 4
depends_on: ["08-frontend-protocol-bridge"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 09: Responsive Task Control Surface

## Acceptance

- Desktop has one active-file-independent aggregate status item when relevant, an always-
  discoverable task/workspace fallback otherwise, and a current-language editor-toolbar shortcut;
  every entry delegates to the same task controller.
- The disclosure lists language detection, policy/effective policy, honest phase/work/elapsed state,
  generation/start/reason/initiator evidence, actionable errors, and Start/Stop/Restart. Restart
  requires translated impact confirmation.
- Phone and coarse-pointer tablet expose the same value/actions inline in the existing Status
  drawer with 44 px controls, one scroll owner, safe-area/dvh containment, no nested drawer, and no
  horizontal overflow; the phone file viewer itself does not attach/start LSP.

## TDD sequence

1. Before UI implementation, update/add mobile production-E2E expectations for task status/control,
   phone no-file-auto-start, tablet same policy/generation, drawer nesting, touch targets, safe area,
   viewport containment, and horizontal overflow. Run one focused scenario RED.
2. Add failing pure view-model tests for relevant-language filtering/order, single/multiple compact
   summaries, error/progress priority, elapsed evidence, and action availability.
3. Add failing components tests for active-panel independence, task fallback visibility, policy
   selection, Start/Stop, Restart confirmation, metadata/errors, editor shortcut delegation, and
   status-bar-disabled behavior.
4. Implement the shared language list and desktop bar/popover, task topbar fallback, editor shortcut,
   then reuse it inline inside App Status drawer. Do not compose a second drawer.
5. Remove the placement UI/resolver; keep its backend value compatibility-only. Add every new string
   to English/pseudo/zh-CN catalogs, run i18n checks, then make focused mobile E2E green.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- --run lib/lsp/task-lsp-view-model.test.ts components/lsp components/app-status-bar components/task/task-top-bar.test.tsx components/editors
cd apps/web && pnpm run typecheck
cd apps && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps/web && pnpm exec eslint lib/lsp/task-lsp-view-model.ts components/lsp components/app-status-bar components/task components/editors
cd apps/web && pnpm e2e:run --project mobile-chrome tests/lsp/mobile-lsp-file-intelligence.spec.ts
```

## Files likely touched

- `apps/web/lib/lsp/task-lsp-view-model.ts`
- `apps/web/lib/lsp/task-lsp-view-model.test.ts`
- `apps/web/components/lsp/task-lsp-control.tsx`
- `apps/web/components/lsp/task-lsp-language-row.tsx`
- `apps/web/components/lsp/task-lsp-restart-dialog.tsx`
- `apps/web/components/lsp/*.test.tsx`
- `apps/web/components/app-status-bar/app-status-items.tsx`
- `apps/web/components/app-status-bar/lsp-status-item.tsx`
- `apps/web/components/app-status-bar/app-status-surface-provider.tsx`
- `apps/web/components/app-status-bar/app-status-drawer.tsx`
- `apps/web/components/task/task-top-bar.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/editors/lsp-status-button.tsx`
- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx`
- `apps/web/components/settings/lsp-status-location-setting.tsx` (removed)
- `apps/web/hooks/use-lsp-status-placement.ts` (removed)
- `apps/web/lib/lsp/lsp-status-placement.ts` (removed)
- `apps/web/src/locales/{en,pseudo,zh-cn}/lsp.json`
- `apps/web/src/locales/{en,pseudo,zh-cn}/settings.json`
- `apps/web/e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts`

## Dependencies

Task 08 supplies authoritative task state/actions and task-keyed Monaco attachments.

## Parallelism

Sequential. Desktop/mobile share view-model and components; splitting would duplicate judgment and
risk nested/inconsistent controls.

## Inputs

- Spec: task status/control and Mobile scenarios.
- Existing App Status bar/drawer, `TaskTopBar`, `SessionMobileBottomNav`, responsive breakpoint, and
  touch-drawer patterns.
- Mobile UI language: 44 px controls, one drawer/scroll owner, safe areas, dvh, containment.

## Output contract

Report desktop/tablet/phone composition, translated keys, RED/GREEN component and mobile E2E
results, geometry evidence, removed placement behavior, and screenshots/artifacts if captured.
Update task/plan status and actual files.

## Results

Completed 2026-08-05.

- Replaced active-file-only lifecycle placement with one task aggregate, a task-topbar fallback,
  and an editor shortcut that all use the task controller. The disclosure exposes policy,
  effective policy, phase/work evidence, completed-work evidence, generation/timing/reasons,
  actionable errors, and confirmed Restart.
- Reused the same disclosure/view-model in the existing phone/tablet Status drawer. It has one
  scroll owner, no nested drawer, 44 px controls, dynamic-viewport/safe-area containment, and no
  file-viewer protocol attachment.
- Removed the status-location setting UI/resolver while retaining its stored compatibility field.
  Added English, pseudo-locale, and Simplified Chinese copy.
- Fixed Radix trigger ref forwarding so the desktop disclosure anchors inside the viewport, and
  subscribed the shared task hook so progress remains live on every mounted surface.
- GREEN: `cd apps/web && pnpm exec vitest run components/lsp/task-lsp-control.test.tsx hooks/domains/lsp/use-task-lsp.test.tsx` — 2 files, 10 tests passed.
- GREEN: `cd apps/web && pnpm run typecheck` and `cd apps/web && pnpm build`.
- GREEN: `cd apps/web && pnpm e2e:run --no-build --project mobile-chrome -- e2e/tests/lsp/mobile-lsp-file-intelligence.spec.ts` — 3 tests passed across phone/tablet composition and the disabled-status-bar path.
