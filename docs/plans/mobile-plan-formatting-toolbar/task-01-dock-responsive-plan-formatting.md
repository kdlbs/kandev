---
id: "01-dock-responsive-plan-formatting"
title: "Dock responsive Plan formatting controls"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-RESPONSIVE-PLAN-FORMATTING-001
acceptance_criteria:
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.1
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.2
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.3
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.4
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.5
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.6
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.7
  - AC-UI-RESPONSIVE-PLAN-FORMATTING-001.8
system_design:
  - ../../specs/ui/system-design/responsive-plan-formatting.md
---

# Task 01: Dock Responsive Plan Formatting Controls

## Summary

Implement the responsive Plan formatting presentation as one vertical TDD
slice. Preserve the desktop Tiptap selection bubble, add the compact
phone/tablet keyboard-edge strip for non-whitespace selections, and prove both
the deterministic geometry and real mobile formatting outcome before marking
the package complete.

## In scope

- Add failing component coverage for responsive presentation, editor state,
  focus retention, command behavior, accessibility, and viewport position.
- Add the failing `mobile-chrome` Plan formatting scenario before production
  implementation.
- Extract and reuse a pure visual-viewport bottom-bar position helper without
  changing terminal keybar behavior.
- Implement the docked toolbar, mobile task-navigation offset plumbing, editor
  bottom clearance, shared formatting controls, and link state. Keep the dock
  hidden for caret or whitespace-only selections and size it at 48 pixels with
  32-pixel visual action surfaces around 44-pixel touch targets.
- Add the localized toolbar name and run focused i18n validation.
- Capture a rendered phone screenshot after the production-build E2E passes and
  record whether physical Android Chrome and iOS Safari checks were available.

## Out of scope

- Task-plan backend or persistence changes.
- Native context-menu suppression or automation.
- New editor commands, plugin API props, or Plan comment-composer redesign.
- Generic QA, broad verification, or unrelated responsive cleanup.

## Acceptance

- The component and browser regressions fail before production changes for the
  diagnosed selection-anchored behavior and caret-visible dock, then pass with
  desktop selection bubbles and compact mobile docked controls using the same
  commands.
- The docked strip appears only for a focused non-whitespace selection, tracks
  visual-viewport resize/scroll, clears task navigation and safe areas,
  preserves selection/focus, exposes accessible 44-pixel touch controls with
  compact visual sizing, and does not create document-level horizontal
  overflow.
- Existing terminal keybar positioning, Plan table behavior, type checking,
  focused lint, and i18n checks remain green.

## Verification

```bash
cd apps/web
pnpm exec vitest run \
  components/editors/tiptap/plan-bubble-menu.test.tsx \
  components/editors/tiptap/tiptap-plan-editor.test.tsx \
  hooks/use-visual-viewport-offset.test.ts \
  components/task/mobile/mobile-terminal-keybar.test.tsx
pnpm e2e:run --project mobile-chrome \
  tests/task/mobile-plan-formatting-toolbar.spec.ts
pnpm run typecheck
pnpm exec eslint \
  components/editors/tiptap/plan-bubble-menu.tsx \
  components/editors/tiptap/plan-bubble-menu.test.tsx \
  components/editors/tiptap/tiptap-plan-editor.tsx \
  components/editors/tiptap/tiptap-plan-editor.test.tsx \
  components/task/task-plan-panel.tsx \
  components/task/mobile/session-mobile-layout.tsx \
  components/task/mobile/mobile-terminal-keybar.tsx \
  hooks/use-visual-viewport-offset.ts \
  hooks/use-visual-viewport-offset.test.ts \
  e2e/tests/task/mobile-plan-formatting-toolbar.spec.ts
pnpm run i18n:check
```

## Files likely touched

- `apps/web/components/editors/tiptap/plan-bubble-menu.tsx`
- `apps/web/components/editors/tiptap/plan-bubble-menu.test.tsx`
- `apps/web/components/editors/tiptap/tiptap-plan-editor.tsx`
- `apps/web/components/editors/tiptap/tiptap-plan-editor.test.tsx`
- `apps/web/components/task/task-plan-panel.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/components/task/mobile/mobile-terminal-keybar.tsx`
- `apps/web/hooks/use-visual-viewport-offset.ts`
- `apps/web/hooks/use-visual-viewport-offset.test.ts`
- `apps/web/src/locales/en/editors.json`
- `apps/web/src/locales/pt-pt/editors.json`
- `apps/web/src/locales/zh-cn/editors.json`
- `apps/web/src/locales/zh-hk/editors.json`
- `apps/web/src/locales/zh-tw/editors.json`
- `apps/web/e2e/pages/session-page.ts`
- `apps/web/e2e/tests/task/mobile-plan-formatting-toolbar.spec.ts`

## Dependencies

None.

## Risks

- Focus and transaction subscriptions must be cleaned up with the editor
  instance or a remounted task can update stale toolbar state.
- The link input intentionally changes focus; hiding the strip on every editor
  blur would unmount the input before the user can submit it.
- Visual-viewport emulation must change only browser geometry and dispatch
  browser events. It must not poke React or Tiptap state directly.
- Keep the internal presentation input out of `RichTextEditorProps`; changing
  the exported plugin contract would expand this repair materially.

## Parallelism

`sequential`

## Inputs

- `REQ-UI-RESPONSIVE-PLAN-FORMATTING-001` and acceptance criteria `.1` through
  `.8`.
- `docs/specs/ui/system-design/responsive-plan-formatting.md`.
- Root-cause source in `plan-bubble-menu.tsx`: unconditional selection-based
  `shouldShow` and `placement: "top"`.
- Shipped visual-viewport exemplar in `mobile-terminal-keybar.tsx` and its unit
  and mobile E2E coverage.
- Existing mobile Plan seeding/navigation pattern in
  `e2e/tests/task/mobile-plan-restore.spec.ts`.

## Results

Implemented the shared Plan formatting controls with a desktop Tiptap
selection bubble and a portal-mounted mobile/tablet dock. The dock preserves
selection and focus, exposes accessible touch targets, tracks visual-viewport
keyboard geometry, clears task navigation and safe areas, and reserves editor
space. Tablet bounds constrain the portaled dock to the Plan pane, and the
mobile regression verifies that the final editor line remains above the dock
when the keyboard is open. The terminal keybar now consumes the shared
position helper. Localized toolbar labels were added to all five catalogs, and
the permanent mobile browser regression captures the keyboard-edge and Bold
flow. The dock now appears only for a focused non-whitespace selection, with a
48-pixel bar and 32-pixel visual action surfaces around 44-pixel touch targets.

Verification completed:

- 43 focused Vitest tests passed across 4 files.
- Production-build `mobile-chrome` E2E passed.
- Web typecheck passed.
- Changed-file ESLint passed.
- `pnpm run i18n:check` passed for all required catalogs.
- Specification lint and `git diff --check` passed.
- Fresh desktop and mobile PR captures passed visual inspection.

Physical Android Chrome and iOS Safari checks were not available in this
environment.
