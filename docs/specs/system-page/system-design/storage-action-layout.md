---
status: current
system: system-page
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001
created: 2026-11-02
updated: 2026-11-03
owners:
  - kandev
---

# Storage action layout system design

## Purpose and boundaries

This design changes the geometry of the two top-level Host storage actions.
The [requirements](../requirements/storage-action-layout.md) belong to
system-page because the controls operate its storage maintenance service.
The existing [maintenance design](storage-maintenance-01.md) owns operation
semantics. The [control sizing contract](../../ui/requirements/control-sizing.md)
supplies desktop and touch dimensions.

## Original failure

`StorageActions` in
`apps/web/components/settings/system/storage/storage-maintenance-settings.tsx`
previously switched to a horizontal row at the viewport `sm` breakpoint. Its
action grid also switched to two columns there. Each button retained `sm:w-44`.

The grid could shrink while its fixed-width buttons retained their width.
The Settings sidebar reduced the section width independently of the viewport
breakpoint. This caused the overlap shown in the supplied screenshot.

`StorageActionButton` also used an intrinsic-width `inline-flex` span for a
disabled reason. Full-width buttons therefore need to size that wrapper too.

## Requirement mapping

| Acceptance criteria | Design section |
| --- | --- |
| 001.1, 001.2, 001.11 | Available-width layout |
| 001.3, 001.6, 001.7, 001.8 | Button and wrapper geometry |
| 001.4, 001.5, 001.9, 001.10 | Controls and interaction |
| 001.12, 001.13, 001.14 | Controls and interaction |

All criteria in this table belong to
`REQ-SYSTEM-PAGE-STORAGE-ACTION-LAYOUT-001`.

## Available-width layout

Use one named inline-size container on the existing
`StorageMaintenanceSettings` root. Its width represents the actual section
space after navigation and page padding. Keep one mounted action tree.

Use a one-column grid for `StorageActions` by default. At a container width of
52rem, place the explanation beside a 24rem action group. Use
`minmax(0, 1fr)` for the explanation and permit its text to wrap.

Below that threshold, give the explanation and action group separate rows.
At non-phone viewports, use two equal action columns when the container is at
least 24rem wide. Otherwise use one action per row. Below the canonical 768px
phone boundary, always use one full-width action per row.

Use Tailwind 4 container variants already supported by the workspace. Combine
the container condition with `md` for the two-column action group. Remove the
two `sm:w-44` button widths and the current `sm` row transition. Give grid
children `min-w-0` and buttons `w-full`. Keep the existing 16px section gap and
12px gap between actions.

The rem thresholds scale with the root font. They define presentation only.
Do not add a resize observer, persisted preference, or responsive data hook.

## Button and wrapper geometry

Add an optional `fullWidth` prop to `StorageActionButton`. Remove this prop
before spreading button DOM props. When enabled, apply `w-full min-w-0` to
both the button and its disabled-reason span. Enable it only on Analyze and
Run now. Preserve existing wrapper sizing for all other consumers.

Permit label text to wrap inside these two buttons. Keep their icon from
shrinking. Override each applicable fixed-height variant with automatic height
when needed, while retaining the desktop or touch minimum height. An
unprefixed height override alone does not replace the shared `max-md` and
coarse-pointer height variants.

The two grid cells stretch to the same row height. Wrapped labels can increase
that height. Do not clip labels, use ellipsis, reduce the font, or hide overflow
to conceal a geometry failure.

## Controls and interaction

Reuse `StorageActionButton`, `settingsActionClassName`, and the existing Tabler
icons. Preserve the outline Analyze button, primary Run now button, focus
rings, border radius, and colors. Keep each existing translated state label
and icon. This layout change adds no animation or copy.

Set each action's accessible name to its translated idle label. Render one
persistent, visually hidden `role="status"` region beside each button.
Keep the region empty before a job starts. Then render the same translated
label as the button through active, succeeded, and failed states. This keeps
the terminal outcome available after the spinner disappears.

`StorageJobButton` owns the button and its status region. It derives the
visible and announced label once. Hide its decorative spinner from the
accessibility tree to prevent duplicate announcements. Reuse the existing
translations in every locale.

Keep Analyze before Run now in DOM order. Preserve the current handlers,
disabled reasons, `data-testid` values, and `data-job-state` attributes.
Resizing must retain focus, active job state, and policy drafts.

## Phone composition

Entry remains Settings > System > Storage > Host. The explanation precedes
two separate full-width action rows. Run now remains the primary action.
Inline controls suit two short operations on an existing destination.

The nearest settings exemplar is `CleanupActions` in
`apps/web/components/settings/system/tool-payload-retention-card.tsx`. It uses
one phone action per row and shared control sizing. The curated direct-phone
navigation precedent is `apps/web/components/kanban-with-preview.tsx`.
Reuse that direct-destination principle rather than introducing an action sheet.

The existing Settings page remains the scroll owner. No fixed element,
viewport-height surface, nested scroller, or new safe-area handling is needed.
Desktop and phone share the same controller and operation state.

## Verification boundaries

Playwright must measure actual button and disabled-wrapper bounds inside their
cells. Check a visible gap, absence of text clipping, center-point hit testing,
and document overflow. A visibility assertion alone cannot detect this bug.

Cover the canonical phone, a narrow fine-pointer viewport, a coarse-pointer
tablet, and section widths immediately around both container thresholds.
Check queued, running, completed, failed, member-disabled, and longer localized
labels. Mock operation endpoints in geometry tests to keep cleanup disposable.

The [work order](../../../plans/storage-action-layout/task-01-responsive-actions.md)
owns the exact commands and test cases. No backend, API, persistence, or
observability change is required.
