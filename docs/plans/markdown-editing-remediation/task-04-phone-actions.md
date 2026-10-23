---
id: "04-phone-actions"
title: "Keep phone file actions reachable"
status: completed
wave: 4
depends_on: ["03-hybrid-recovery"]
plan: "plan.md"
requirements:
  - REQ-UI-MARKDOWN-FILE-EDITING-003
acceptance_criteria:
  - AC-UI-MARKDOWN-FILE-EDITING-003.2
  - AC-UI-MARKDOWN-FILE-EDITING-003.3
  - AC-UI-MARKDOWN-FILE-EDITING-003.4
  - AC-UI-MARKDOWN-FILE-EDITING-003.5
  - AC-UI-MARKDOWN-FILE-EDITING-003.10
system_design:
  - ../../specs/ui/system-design/markdown-file-editing.md
---

# Task 04: Keep phone file actions reachable

## Summary

Replace the clipped phone action row with a compact mode selector, actions menu, and reserved Save action. Preserve the existing editor scroll owner.

## In scope

- Use existing responsive menu primitives with visible triggers.
- Put Reload in the actions menu and keep the remote-update cue visible.
- Keep Save visible in clean, dirty, saving, and remote-update states.
- Keep translated labels, keyboard operation, focus return, safe areas, and at least 44px phone hit targets.

## Out of scope

- A global Button redesign, a new navigation surface, or new movement animation.
- Changing desktop toolbar composition.

## Acceptance

- The 320px remote-update regression first fails on clipped Save. It passes with reachable mode choices, Reload, and Save.
- Rendered checks cover 320px, 393px, 767px, and 768px with appropriate pointer modes, translated labels, and constrained keyboard height.
- No document horizontal overflow or covered controls occur. Required phone hit targets measure at least 44px.

## UI preview

UI-01: Fixed file identity row with Back and dirty/remote cues. The next row contains a current-mode trigger, actions trigger, and visible Save. Reload lives in the actions menu. The editor body alone scrolls vertically. Desktop keeps inline controls. See [combined preview](plan.md#ui-preview).

## Verification

Add the named regression scenarios before the correction. Run the focused test to establish the expected failure, then run this complete block after the fix. Commands start at the repository root.

```bash
(cd apps/web && pnpm exec vitest run components/task/mobile/mobile-file-viewer-panel.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project=mobile-chrome tests/task/mobile-markdown-file-editing.spec.ts --grep "review regression: phone actions")
```

## Files likely touched

- `apps/web/components/task/mobile/mobile-file-viewer-panel.tsx`
- `apps/web/components/task/mobile/mobile-file-actions.tsx`
- `apps/web/components/task/mobile/mobile-file-actions.test.tsx`
- `apps/web/components/task/mobile/mobile-file-viewer-panel.test.tsx`
- `apps/web/e2e/tests/task/mobile-markdown-file-editing.spec.ts`
- `apps/web/src/locales/*/task.json`

## Dependencies

Task 03-hybrid-recovery. Coordinators and browser test files overlap, so execution is sequential.

## Risks

Long translations and browser scaling can break a layout that fits English. Measure bounds and hit areas instead of asserting class names.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/markdown-file-editing.md).
- [System design](../../specs/ui/system-design/markdown-file-editing.md).
- [Review evidence and UI composition](plan.md).

## Results

Implemented a current-mode menu, secondary action menu, visible Save button,
and remote-update cue. Phone controls use 44px targets. The same primitives
preserve keyboard focus and contain menu scrolling. All new copy is localized.

Four action-component tests pass. The targeted aggregate and translation
checks pass. The 320px English screenshot shows visible Source, actions, and
Save controls while a remote update is pending. Browser checks passed for
English and Portuguese at 320px, 393px, and 767px,
including a constrained viewport, Reload, menu focus return, and tablet Save.
The English and Portuguese 320px screenshots were visually inspected.
