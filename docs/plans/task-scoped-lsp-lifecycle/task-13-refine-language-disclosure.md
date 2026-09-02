---
id: "13-refine-language-disclosure"
title: "Refine language disclosure"
status: completed
wave: 6
depends_on: ["12-persist-status-visibility"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 13: Refine Language Disclosure

## Acceptance

- Each visible language is a compact, independently expandable row; its summary keeps language,
  state, and detection visible while policy, evidence, and actions remain inside the content.
- Policy uses the shared Select primitive with correct focus, disabled, desktop, and 44 px touch
  behavior.
- Hidden languages are excluded from aggregate rows and counts without changing controller state;
  a matching editor shortcut force-shows and focuses its language.
- When every language is hidden, the task/workspace entry remains discoverable and the disclosure
  shows a translated visibility-settings explanation.

## TDD sequence

1. Add failing pure view-model tests for hidden filtering, aggregate counts, all-hidden state, and
   editor-language force visibility.
2. Add failing component tests for collapsed-by-default rows, independent expansion, shared Select,
   and current-language focus.
3. Implement shared visibility derivation and controlled Collapsible rows; retain one responsive
   disclosure and controller.
4. Polish density, wrapping, state hierarchy, and keyboard/touch behavior without changing the
   task-scoped lifecycle contract.

## Verification

```bash
cd apps/web && pnpm exec vitest run lib/lsp/task-lsp-view-model.test.ts components/lsp/task-lsp-control.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint lib/lsp/task-lsp-view-model.ts components/lsp
```

## Output contract

Record RED/GREEN evidence, before/after UI structure, and desktop/touch behavior.

## Results

Completed 2026-08-06.

- RED: the view model retained hidden languages in rows/counts, each language rendered fully open,
  and the policy picker was a native select. Component tests also proved the all-hidden fallback
  and current-editor force-visibility behavior were absent.
- Replaced the stacked expanded cards with independent controlled Collapsible rows. Summaries keep
  language, state, detection, and a rotating chevron visible; details contain policy, lifecycle
  evidence, progress, errors, and actions.
- Replaced the native policy dropdown with the shared Select primitive, including disabled state,
  focus treatment, bounded width, and 44 px touch items. Phone/tablet continue to embed the same
  rows inside the existing Status drawer without nesting another surface.
- Aggregate derivation filters hidden languages before counts and compact status, while an editor
  shortcut force-shows and initially expands its current language. When every row is hidden, the
  task fallback remains and links to editor settings.
- GREEN: focused view-model/control tests passed, full web ESLint completed with zero warnings,
  typecheck passed, and the production control refactor stayed within repository complexity limits.
