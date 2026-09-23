---
status: current
system: ui
requirements:
  - REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-001
  - REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-002
---

# File-row containment

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
| `REQ-UI-CHANGES-FILE-ROW-CONTAINMENT-002` | [Touch file rows](#touch-file-rows), [Verification](#verification) |

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

Provider rows retain their existing containment layout. Working-tree rows also
apply the touch presentation below without rewriting paths or measuring text.

## Touch file rows

`FileRow` uses `useResponsiveBreakpoint` to choose the compact presentation
when `isMobile || !isFinePointer`. The existing row click handler continues
to own selection, image editing, and repository/layer-aware diff opening.
`TouchFileRowContent` renders a small file icon, a flexible filename-first
identity button, and one 44px ellipsis trigger. List mode puts the directory
on a secondary line; tree mode retains parent context and indentation. Long
basenames wrap instead of ending in an ellipsis. The identity button itself
is at least 44px tall. Inline statistics share the secondary line without
taking space from the basename.

`TouchFileRowActions` reuses the shared DropdownMenu primitives, including
their inset phone sheet below 640px. Its label shows the full path with wrapping.
Stage/Unstage, Edit, and Discard call the existing callbacks with repository
identity. Menu events stop propagation so actions cannot open the diff. Pending
staging uses the existing per-file pending flag. Discard passes the persistent
ellipsis trigger as its confirmation anchor, outside the transient menu.

This follows the mobile UI language's visible contextual menu pattern and
the shipped `MobileChangesPanel` full-height content surface. Primary content
stays inline because scanning files is frequent; temporary action choices
use the existing bottom menu. No new state, persistence, scroll owner, or
viewport container is introduced. Fine-pointer desktop composition is retained.

## Responsive composition

- **Desktop outcome:** a resizable Dockview Changes panel, including its 180px
  supported minimum, keeps long paths clear of trailing metadata.
- **Mobile entry point and surface:** the existing Changes bottom-navigation
  item opens the focused `MobileChangesPanel`; the shared inline timeline and
  its single `PanelBody` scroll owner remain unchanged.
- **Nearest shipped exemplars:** the working-tree `FileRow` supplies the flex
  shrink geometry, while `MobileChangesPanel` supplies the phone composition.
- **Hierarchy and primary action:** file identity stays first, change evidence
  stays visible, and tapping the row still opens its diff.
- **Surface rationale:** file rows are primary, frequently scanned content, so
  in-place truncation fits better than a drawer, route, or horizontal scroller.
- **Shared logic:** all data, selection, Git mutations, and diff handlers remain
  shared; working-tree touch rows specialize presentation only.
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
- Working-tree phone Playwright proves filename width, row density, long-name
  wrapping, menu containment and focus return, staging/unstaging, Edit, discard
  cancellation, and diff navigation. It checks 393px, 767px, and 768px widths.
- Desktop Git E2E and the existing tree Vitest suite guard inline actions and
  tree behavior. Screenshots record the phone list, menu, and desktop surface.

## Related decisions

None. The correction reuses an existing row layout pattern and changes no
architecture boundary.
