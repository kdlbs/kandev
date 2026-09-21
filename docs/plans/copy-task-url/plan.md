---
spec: docs/specs/ui/requirements/kanban-preview-workflow-step-navigation.md
created: 2026-09-21
status: draft
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
- `AC-UI-KANBAN-PREVIEW-STEP-NAVIGATION-002.4` amended: the inter-element-gap
  bound moves from 18px to 70px, and the prose is reframed — with three
  controls, the title-floor override is the routinely-engaged path rather
  than a theoretical edge case.
- "Out of scope" copy bullet narrowed to REQ-001/-002, since REQ-003
  introduces its own new copy.

### `docs/specs/ui/system-design/kanban-preview-workflow-step-navigation.md`

- `REQ-UI-KANBAN-PREVIEW-STEP-NAVIGATION-003` added to the requirement
  mapping and to Components and responsibilities (`CopyTaskUrlButton`).
- "Header layout" arithmetic recomputed for three `h-8 w-8` controls: content
  box 262px (inline)/263px (floating) unchanged; controls-plus-gaps
  104px (was 68px); remainder 158px (was 194px) inline / 159px (was 195px)
  floating; bound `g <= 70px` (was 18px) inline, `g <= 71px` (was 19px)
  floating.
- Test-strategy E2E bullet updated to assert all three panel controls, not
  two.

---

## Tests

- **What:** the copy control renders in the panel controls cluster, before
  the Maximize control, only when a task is previewed; clicking it copies the
  task's origin-joined `/t/:id` URL via the mocked `copyToClipboard`; the
  button briefly shows a checkmark and reverts after the confirmation window;
  the accessible name/tooltip differ from the Link submenu's wording.
  **File:** `apps/web/components/task-preview-panel.test.tsx` (new
  `describe("TaskPreviewPanel copy task link", ...)` block, 4 tests).
  **How:** mock `@/lib/utils/copy-to-clipboard`, wrap the render helpers in
  `TooltipProvider` (Radix throws without an ancestor provider in this repo's
  version — see `components/workflow-selector-row.test.tsx`), use
  `fireEvent.click` + `await act(async () => {})` to flush the copy promise
  rather than `waitFor` under fake timers (which deadlocks against
  `waitFor`'s own polling timers), then `vi.advanceTimersByTime(1500)` inside
  `act` for the revert assertion.

## E2E Tests

- **Scenario:** the existing "keeps the header a single row at the panel's
  minimum width" test (`preview-workflow-step-navigation.spec.ts`) is
  extended to assert the copy-task-link control is visible, enabled, shares
  the header row's vertical center with the other controls, and sits before
  the open-full-page control — empirically re-proving the title floor holds
  at the 300px minimum with three fixed-width controls now in the budget,
  rather than trusting the recomputed `g <= 70px` bound by derivation alone.
  **File:** `apps/web/e2e/tests/kanban/preview-workflow-step-navigation.spec.ts`.

## Implementation Waves

Small feature — sequential, no parallel candidates.

```text
Wave 1:
- [x] [task-01-preview-header-copy-url](task-01-preview-header-copy-url.md) — component + wiring + unit tests (21/21 pass) + spec/system-design amendments + extended E2E containment test (passing), committed.
```

## Open Questions

(Delete when empty.)
