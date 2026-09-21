---
id: "01-preview-header-copy-url"
title: "Copy-task-link control in the preview header"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/kanban-preview-workflow-step-navigation.md"
system_design: "../../specs/ui/system-design/kanban-preview-workflow-step-navigation.md"
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
     renders an `IconCopy` ghost icon button, `h-8 w-8`, with a tooltip and
     `aria-label` reading "Copy task link"; on click it copies
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
     controls" terminology and AC-002.2/.4 (18px → 70px gap bound) for a
     third fixed-width control.
  5. `docs/specs/ui/system-design/kanban-preview-workflow-step-navigation.md`'s
     "Header layout" section is recomputed for three `h-8 w-8` controls
     (104px controls-plus-gaps, 158px/159px remainder, `g <= 70px`/`71px`).
  6. `preview-workflow-step-navigation.spec.ts`'s minimum-width containment
     test asserts the new control's visibility, enabled state, row alignment,
     and position ahead of the Maximize control, and passes at the 300px
     panel minimum.
- **Verification:**
  - `cd apps/web && pnpm exec vitest run components/task-preview-panel.test.tsx`
    — 21/21 passed.
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
  this environment and passed; the width-budget bound (`g <= 70px`) was
  proven empirically via the extended Playwright containment test rather than
  trusted from the hand derivation alone.
