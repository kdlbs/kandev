---
status: current
system: ui
requirements:
  - REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-001
---

# Changes File Row Containment System Design

## Purpose and boundaries

The shared Changes timeline renders working-tree files and provider pull-request
files in both desktop `ChangesPanel` and phone `MobileChangesPanel`. This design
owns only their responsive row geometry. Existing Git and provider hooks remain
authoritative for paths, statistics, statuses, repository identity, and diff
selection.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-001` | [Row layout contract](#row-layout-contract), [Responsive composition](#responsive-composition), [Verification](#verification) |

## Components and responsibilities

- `apps/web/components/task/changes-panel-file-row.tsx` remains the shipped
  working-tree exemplar: its path region can shrink, its directory context
  yields before the basename, and its trailing metadata cannot shrink.
- `apps/web/components/task/changes-panel-pr-files.tsx` renders provider file
  rows through `PRFileRow` and shall use the same shrink priorities without
  changing PR/repository-aware `onOpenDiff` arguments.
- `LineStat` and `FileStatusIcon` keep their current semantic and visual
  contracts. They do not own row sizing.
- `ChangesPanelBody` remains shared by desktop and phone compositions, so one
  row correction serves both viewports.

## Row layout contract

Each row keeps two horizontal regions. The leading icon-and-path region has a
zero minimum inline size and may shrink. Directory context truncates first;
the basename also accepts truncation when it alone exceeds the remaining width.
The trailing statistics-and-status region does not shrink. The existing full
path `title` remains on the path control.

This is a CSS layout correction only. It adds no responsive state, JavaScript
measurement, path rewriting, tooltip, or alternate row component.

## Responsive composition

- **Desktop outcome:** a resizable Dockview Changes panel, including its 180px
  supported minimum, keeps long paths clear of trailing metadata.
- **Mobile entry point and surface:** the existing Changes bottom-navigation
  item opens the focused `MobileChangesPanel`; the shared inline timeline and
  its single `PanelBody` scroll owner remain unchanged.
- **Nearest shipped exemplars:** the working-tree `FileRow` supplies the flex
  shrink geometry, while `MobileChangesPanel` supplies the phone composition.
- **Hierarchy and primary action:** file identity stays first, trailing change
  evidence stays visible, and tapping the row still opens its diff.
- **Surface rationale:** file rows are primary, frequently scanned content, so
  in-place truncation fits better than a drawer, route, or horizontal scroller.
- **Shared logic:** all data, selection, and diff handlers remain shared;
  viewport-specific presentation is unchanged.
- **Geometry:** dynamic viewport behavior, safe-area handling, touch sizing,
  and vertical scroll ownership are unchanged. The correction must not add
  panel-level or document-level horizontal overflow.

## Verification

- The existing focused Vitest suite continues to guard PR/repository-aware diff
  routing; browser tests own flex geometry and hit-testing evidence.
- Desktop Playwright seeds a long PR basename, resizes the Changes column to its
  supported minimum, and proves path/metadata separation, containment,
  hit-testing, diff opening, and no row or document horizontal overflow.
- Pixel 5 Playwright exercises the same seeded PR row through the existing
  mobile Changes entry point and proves the same visible and actionable result.

## Related decisions

None. The correction reuses an existing row layout pattern and changes no
architecture boundary.
