---
id: "01-preview-header-copy-url"
title: "Copy-task-link control in the preview header"
status: done
wave: 1
depends_on: []
plan: "plan.md"
system_design:
  - "../../specs/ui/system-design/kanban-preview-workflow-step-navigation.md"
requirements:
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002
acceptance_criteria:
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.1
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.2
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.3
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.4
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.2
  - AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4
---

# Task 01: Copy-task-link control in the preview header

- **Acceptance:**
  1. `CopyTaskUrlButton` (`apps/web/components/task/copy-task-url-button.tsx`)
     renders an `IconCopy` ghost icon button, `h-8 w-8`, with a stable
     `aria-label` reading "Copy task link" and a supplementary tooltip hint;
     on click it copies
     `${window.location.origin}${linkToTask(taskId)}` via the shared
     `copyToClipboard()` utility and shows an `IconCheck` confirmation for
     1500ms.
  2. `TaskPreviewPanel`'s `PreviewPanelHeader` renders the control in the
     panel controls cluster, gated on a selected task, positioned after the
     actions-menu trigger and before the Maximize control.
  3. The icon and wording are distinct from the Link submenu's `IconLink`
     (`kanban-card-link-submenu.tsx`), satisfying
     AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.3/.4.
  4. `docs/specs/ui/requirements/kanban-preview-workflow-step-navigation.md`
     gains REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003 and amends the "Panel
     controls" terminology and AC-002.2/.4 (18px/19px → `g <= 74px`/`75px`
     fine pointer, `g <= 34px`/`35px` coarse pointer, binding) for a third
     fixed-width control.
  5. `docs/specs/ui/system-design/kanban-preview-workflow-step-navigation.md`'s
     "Header layout" section is recomputed for the three panel controls,
     whose widths are NOT uniform (Copy and Maximize add `h-8 w-8`, 32px at
     a fine pointer; Close stays unstyled `size="icon"`, 28px; all three
     converge on 44px under `[@media(pointer:coarse)]:`): 100px fine /
     140px coarse controls-plus-gaps, 162px/163px fine remainder
     (inline/floating), 122px/123px coarse remainder, `g <= 74px`/`75px`
     fine, `g <= 34px`/`35px` coarse (the coarse-pointer inline bound is
     the tightest and binding).
  6. `preview-workflow-step-navigation.spec.ts`'s minimum-width containment
     test asserts the new control's visibility, enabled state, row alignment,
     and position ahead of the Maximize control, and passes at the 300px
     panel minimum.
- **Verification:**
  - `cd apps/web && pnpm exec vitest run components/task-preview-panel.test.tsx`
    — 22/22 passed (21 at first commit, plus the round-2 F1 regression test;
    33/33 passed across `task-preview-panel.test.tsx` and
    `task-preview-panel-step-indicator.test.tsx` together, the latter also
    touched by round 2 for an unrelated `TooltipProvider` wrap).
  - `cd apps/web && pnpm run typecheck` — clean.
  - `cd apps/web && pnpm run lint -- components/task-preview-panel.tsx components/task-preview-panel.test.tsx components/task/copy-task-url-button.tsx` — 0 problems.
  - `python3 scripts/lint-spec-files.py --all` — all specification files passed.
  - `python3 scripts/list-docs.py validate` — validated 294 decisions and 1063 specifications.
  - `cd apps/web && pnpm e2e:run --host --no-strict -- e2e/tests/kanban/preview-workflow-step-navigation.spec.ts --grep "keeps the header a single row"` — 1 passed.
- **Files likely touched:**
  - `apps/web/components/task/copy-task-url-button.tsx` (new)
  - `apps/web/components/task-preview-panel.tsx`
  - `apps/web/components/task-preview-panel.test.tsx`
  - `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/task.json`
  - `apps/web/e2e/tests/kanban/preview-workflow-step-navigation.spec.ts`
  - `docs/specs/ui/requirements/kanban-preview-workflow-step-navigation.md`
  - `docs/specs/ui/system-design/kanban-preview-workflow-step-navigation.md`
- **Dependencies:** None.
- **Parallelism:** sequential.
- **Inputs:**
  - Reference copy pattern: `components/integrations/change-request-detail-copy-button.tsx`,
    `components/task/port-forward-dialog-actions.tsx` (`PortUrlActions`)
  - `copyToClipboard()`: `apps/web/lib/utils/copy-to-clipboard.ts`
  - `linkToTask()`: `apps/web/lib/links.ts`
  - Test-mocking pattern for `copyToClipboard` + `TooltipProvider`:
    `components/integrations/change-request-detail.test.tsx`,
    `components/workflow-selector-row.test.tsx`
- **Output contract:** summary, files changed, exact verification commands with
  results, task status → `done`, plan checkbox update.
- **Status note:** marked done. All verification commands above were run in
  this environment and passed; the fine-pointer width-budget bound
  (`g <= 74px`) was proven empirically via the extended Playwright
  containment test, which runs against this project's desktop-chromium fine
  pointer only. The tighter, binding coarse-pointer bound (`g <= 34px`) is
  not exercised by that E2E project and rests on the hand derivation alone
  (see the system design's "Header layout" section).
- **Superseded arithmetic:** the `g <= 74px`/`34px` bounds above were wrong
  at a coarse pointer (they omitted the step indicator's own floor and the
  task actions trigger). The reconciled budget and the pointer-aware minimum
  panel width are implemented by
  [task-02](task-02-pointer-aware-panel-minimum.md).
