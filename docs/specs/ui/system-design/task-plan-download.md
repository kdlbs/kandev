---
status: current
system: ui
requirements:
  - REQ-UI-TASK-PLAN-DOWNLOAD-001
---

# Task Plan Download System Design

## Purpose and boundaries

UI owns the local action using the current draft supplied by the existing main
Plan panel. Tasks owns the draft and persistence; this interaction consumes it
without changing its lifecycle. The [requirements](../requirements/task-plan-download.md)
define the outcome.

## Requirement mapping

| Criteria | Design section |
| --- | --- |
| `.1`, `.2`, `.3` | [Control and data flow](#control-and-data-flow) |
| `.4` | [Failure](#failure) |
| `.5` | [Responsive presentation](#responsive-presentation) |

## Control and data flow

`TaskPlanPanel` already passes `state.draftContent` to `PlanPanelHeader` and
the editor. The header passes that exact string to a small
`PlanDownloadButton` if extraction keeps the header bounded. At activation,
pass it unmodified to `triggerFileDownload` in `lib/utils/file-download.ts`
with `fileName: "plan.md"` and `isBinary: false`. The helper owns UTF-8 Blob
creation, browser and native download lifetime, and cleanup. The fixed name is
program data, not translated copy; use a reasoned i18n exemption if required.
Use `draftContent.trim()` only for disabling, never for output. Do not read
`plan.content` for this action, capture a stale snapshot, or introduce session,
workspace, API, save, implementation, or editor state dependencies.

## Failure

Catch a synchronous helper exception and use the established localized toast
with `task:failedToDownloadFile`. Use the default variant: the error variant
reports to the backend, which would violate this local action's boundary. No
retry, state mutation, or new persistence is introduced. Reuse `task:download` for the control's
localized accessible name. The helper's existing download and native lifetime
tests retain ownership of its implementation.

## Responsive presentation

The existing main header is shared with `SessionMobileLayout.MobilePlanPanel`.
Keep one explicit control alongside Implement and Revisions; do not add an
overlay or redesign neighboring actions. Use existing button primitives and
phone/coarse-pointer sizing, with phone width independently considered even on
a fine pointer. The Plan header's action slot shrinks and scrolls horizontally
when its panel is narrower than the action cluster, so keyboard focus and
pointer or touch scrolling can reach each control without page-level horizontal
overflow. Preserve the editor's scroll
ownership, safe areas, focus and touch activation. Translate the visible or
accessible label at render time; allow localized labels to fit without
shrinking text. Geometry and rendered behavior are verified in the work order.
The existing coarse-pointer tablet layout does not mount the main Plan panel;
the coarse-pointer workbench check uses a wider viewport where that panel is
available. This local action does not change tablet navigation.

## Persistence, security, and observability

The action holds only the existing in-memory draft and makes a user-initiated
local file download. It adds no storage, permission, backend interface, logs,
or metrics. Other Plan state and retention rules remain with Tasks.

## Related records

- [Plan text search](plan-text-search.md) is an adjacent UI interaction.
- [Plan write consistency](../../tasks/system-design/plan-write-consistency.md)
  owns persistence and is unchanged.
- [Delivery plan](../../../plans/task-plan-download/plan.md).
