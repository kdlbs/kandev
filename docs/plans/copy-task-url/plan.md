---
created: 2026-09-21
status: draft
requirements:
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-001
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002
  - REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003
system_design:
  - ../../specs/ui/system-design/kanban-preview-workflow-step-navigation.md
---

# Implementation Plan: Copy Task URL From the Preview Header

## Overview

The kanban preview panel's header exposes an open-full-page control and a
close control, but no way to copy the previewed task's link without opening
the full task page. This adds a one-click copy-task-link control to the
preview header's panel controls cluster
(REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003), positioned before the
open-full-page control and visually distinct from the Link submenu's
`IconLink` (which links an external PR/issue to the task, a different action).

Adding a third fixed-width control to the panel controls cluster changes the
header's width-budget arithmetic that
REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002 governs, so this plan amends that
containment requirement and its system design alongside the new control,
rather than shipping a control the existing containment spec does not account
for.

Implementation is a pure frontend change (new leaf component + one wiring
edit) plus the spec/system-design amendments and E2E containment coverage. No
backend or store changes.

---

## Frontend

### `apps/web/components/task/copy-task-url-button.tsx` (new)

- `CopyTaskUrlButton({ taskId }: { taskId: string })`: a `Tooltip`-wrapped
  ghost icon `Button` (`size="icon"`, `h-8 w-8`, matching the Maximize
  control's sizing) showing `IconCopy` from `@tabler/icons-react`.
- On click, builds `${window.location.origin}${linkToTask(taskId)}`
  (`lib/links.ts`) and copies it via the shared `copyToClipboard()`
  (`lib/utils/copy-to-clipboard.ts`) — never `navigator.clipboard.writeText()`
  directly, per `apps/web/AGENTS.md`.
- On a successful copy, swaps the icon to `IconCheck` for 1500ms as visual
  confirmation, then reverts.
- `aria-label` and tooltip content both use `t("task:copyTaskLink")` /
  `t("task:taskLinkCopied")`, distinct wording and a distinct icon from the
  Link submenu's `IconLink` (`kanban-card-link-submenu.tsx`).
- Reference pattern: `components/integrations/change-request-detail-copy-button.tsx`
  and `components/task/port-forward-dialog-actions.tsx`'s `PortUrlActions`.

### `apps/web/components/task-preview-panel.tsx`

- Render `{task && <CopyTaskUrlButton taskId={task.id} />}` in
  `PreviewPanelHeader`'s control cluster, after the `TaskActionsMenuTrigger`
  block and before the Maximize button block — matching
  AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003.1's "before the open-full-page
  control" ordering.

### i18n

- New keys `task:copyTaskLink` ("Copy task link") and `task:taskLinkCopied`
  ("Task link copied") in `en/task.json`, translated in `pt-pt`, `zh-cn`
  (zh-hant pair via `pnpm run i18n:zh-hant`), and the pseudo locale.

---

## Spec amendments

### `docs/specs/ui/requirements/kanban-preview-workflow-step-navigation.md`

- New `REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003` (the copy control) with
  `AC-*.1`-`.4`.
- "Panel controls" terminology entry amended to name all three fixed-width
  controls and note the task-actions-menu trigger's presence/exclusion from
  that budget.
- `AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.2` amended from "both panel
  controls" to "every panel control".
- `AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4` and the new `002.7`: see
  "Touch-floor reconciliation" below. The earlier `g <= 74px`/`34px` bounds
  are superseded.
- "Out of scope" copy bullet narrowed to REQ-001/-002, since REQ-003
  introduces its own new copy.

### `docs/specs/ui/system-design/kanban-preview-workflow-step-navigation.md`

- `REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003` added to the requirement
  mapping and to Components and responsibilities (`CopyTaskUrlButton`).
- "Header layout" rewritten with measured terms, the budget, and the
  pointer-aware minimum; see "Touch-floor reconciliation" below.
- Test-strategy E2E bullet updated to assert all three panel controls, not
  two.

### Touch-floor reconciliation (wave 2)

The third control broke two floors at the old 300px minimum. At a coarse
pointer the cluster (160px) and the 88px title floor left 14px for a step
indicator whose AC-001.17 hit area is 44px. At a fine pointer the cluster
(120px) left 54px, below a two-digit count's 67.4px. The spec pair now:

- Defines the control cluster (including the task actions trigger), the
  pointer mode, the minimum panel width, and the indicator floor.
- AC-001.17: the 44px applies at every panel width.
- AC-002.1 to .3: hold at the minimum panel width for the current pointer mode.
- AC-002.4: the indicator never shrinks below its floor; only the step name
  truncates. Cluster + 88px title + floor fit for up to 99 steps with gaps of
  6px or less.
- AC-002.7 (new): minimum 320px fine, 380px coarse; rendered width is the larger
  of the chosen width and the live minimum; storage keeps the chosen width
  (floor 320px); the resize drag and the inline/floating choice follow it.
- Decision: raise the minimum per pointer mode. Rejected alternatives: exempting
  the 44px floor, a title below 88px, and a narrow-width overflow menu. Cost:
  at most 80px of board width at a coarse pointer.

Budget: fine `120 + 88 + 68 = 276`, so `W >= 314 + g`; coarse
`160 + 88 + 88 = 336`, so `W >= 374 + g`. At 320px and 380px, `g <= 6px`
inline and `g <= 7px` floating. Implemented by
[task-02](task-02-pointer-aware-panel-minimum.md).

---

## Tests

- **What:** the copy control renders in the panel controls cluster, before
  the Maximize control, only when a task is previewed; clicking it copies the
  task's origin-joined `/t/:id` URL via the mocked `copyToClipboard`; the
  button briefly shows a checkmark and reverts after the confirmation window;
  the accessible name/tooltip differ from the Link submenu's wording.
  **File:** `apps/web/components/task-preview-panel.test.tsx` (new
  `describe("TaskPreviewPanel copy task link", ...)` block, 5 tests).
  **How:** mock `@/lib/utils/copy-to-clipboard`, wrap the render helpers in
  `TooltipProvider` (Radix throws without an ancestor provider in this repo's
  version — see `components/workflow-selector-row.test.tsx`), use
  `fireEvent.click` + `await act(async () => {})` to flush the copy promise
  rather than `waitFor` under fake timers (which deadlocks against
  `waitFor`'s own polling timers), then `vi.advanceTimersByTime(1500)` inside
  `act` for the revert assertion.

## E2E Tests

- **Scenario (wave 1, done):** the existing "keeps the header a single row"
  containment test asserts the copy-task-link control's visibility, enabled
  state, row alignment, and position before the open-full-page control.
- **Scenario (wave 2):** per the system design's Test strategy, the same
  containment test is extended with a 10+ step workflow (the task on step 10
  or later, so both sides of the count have two digits), a long step name, and
  the task actions trigger at the 320px fine minimum; a new test runs the same
  setup and assertions on `coarseDesktopTestPage` (1280x900, coarse pointer) at
  380px, adding the indicator's 44x44 floor. Both assert the panel rendered
  inline and that the indicator is at most half its title-and-indicator group
  (the AC-002.4 cap).
  **File:** `apps/web/e2e/tests/kanban/preview-workflow-step-navigation.spec.ts`.

## ASCII UI preview

`UI-02: Preview header at the minimum panel width`, from the kanban board with
a task previewed. Before: at 300px coarse, the indicator is squeezed to 14px.
After: the panel minimum grows so every floor fits. The structure is required;
the spacing is illustrative. Phones render no preview (below 768px).

```text
Fine pointer, panel 320px (was 300px):
| Fix login redi... | * Rev.. 12/15 | ... [copy] [open] [x] |
  title >= 88px      indicator floor   control cluster 120px

Coarse pointer, panel 380px (was 300px):
| Fix login redi... | * Rev.. 12/15 v | ...  [copy] [open] [x] |
  title >= 88px      indicator >= 44x44  control cluster 160px
```

Maps to AC-001.17, AC-002.1 to .4 and .7; checked by the task-02 E2E tests.

## Implementation Waves

Small feature — sequential, no parallel candidates.

```text
Wave 1:
- [x] [task-01-preview-header-copy-url](task-01-preview-header-copy-url.md) — component + wiring + unit tests (33/33 pass across 2 files, 5 tests in the new describe block) + spec/system-design amendments + extended E2E containment test (passing), committed.

Wave 2:
- [ ] [task-02-pointer-aware-panel-minimum](task-02-pointer-aware-panel-minimum.md) — pointer-aware minimum panel width (320/380px), indicator floor, unit + component tests, fine and coarse E2E containment.
```

## Open Questions

(Delete when empty.)
