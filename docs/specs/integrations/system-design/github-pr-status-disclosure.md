---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001
---

# GitHub PR Status Disclosure System Design

## Boundary and evidence

This design changes detailed GitHub status presentation. It does not change
provider state or the informational task-summary contract.

Source inspection shows three contributing conditions:

- `PRChecksSection` replaces aggregate counts with detailed workflow rows.
  Notices, comments, automation controls, and the refresh footer also appear conditionally.
- `CHANGE_REQUEST_CI_DESKTOP_SCROLL_CLASS` gives the whole popover a maximum
  height, not a stable height. Its header scrolls with the details.
- The chat chip requests `side="top"`. Increasing content height therefore
  moves the header away from the trigger. The header PR link has a small icon target.

The browser regression reproduced a 104 CSS pixel vertical movement before
implementation. The corrected test measures the frame and original link
coordinates across initial loading and later refreshes. The attached video
was not available for inspection.

## Requirement mapping

| Criteria for REQ-INTEGRATIONS-GITHUB-PR-DISCLOSURE-001 | Design section |
| --- | --- |
| .1, .4 | Stable frame and scroll ownership |
| .2, .3 | Header and asynchronous content |
| .5, .6 | Pointer, keyboard, and mobile |

## Stable frame and scroll ownership

Use the existing Radix Popover and Drawer primitives. Do not modify their
global defaults. Apply stable geometry explicitly to GitHub status surfaces.

Desktop starts with a preferred outer height of 28rem, capped by the dynamic
viewport and Radix's available-height constraint. Retain current widths:
20rem for single-PR surfaces and 24rem for multi-PR surfaces.
Use a width cap for narrow viewports. Height must be independent of result
count from the first visible frame. Preserve collision handling at opening.
Viewport resizing may recompute geometry. Data updates must not do so.

The hierarchy is an outer non-scrolling flex column, an optional fixed tab
strip, a fixed PR header, one flexible scrolling details region, and a fixed
refresh footer. Propagate `min-h-0` and flex sizing through the multi-PR
tabpanel. Avoid a second scroll container at the PopoverContent level.
The body uses `overflow-y-auto`, `overscroll-contain`, and a stable scrollbar
gutter. Check small viewports where header and footer consume more space.

Keep the same frame across PR tab selection. Each selected PR may reset its
details scroll position. Retain existing selection and unlink rules.

## Header and asynchronous content

`PRCIPopover` supplies explicit header, body, and footer regions. Reserve a
two-line title/author area so late author data cannot change header height.
Truncate the title within the remaining width. Preserve title detail behavior.

Give the external link visible localized Open PR text and the same accessible
name, retaining its URL, target, and `pr-popover-pr-link` test ID. Use a 28px
desktop control height and at least 44px in both dimensions on phone/touch.
Do not change topbar click navigation to the internal review panel.

`usePRCIPopover` already returns cached results while fetching. Preserve that
resource behavior and its workspace/host/PR isolation. Show skeleton rows only
when no feedback exists and a fetch is pending. Keep the header and footer
slots mounted in all states. Skeletons are non-interactive and aria-hidden.
Use the existing localized loading/updating status. Do not replace cached
results with skeletons during refresh or invent successful checks.

Keep authentication recovery and existing errors in the body. Preserve the
known PR link even if status authentication fails. Avoid additional requests,
new retries, or a second cache. All content updates remain live.

Do not animate frame height or position during loading. Existing short opening
motion remains subject to reduced-motion handling. Loading feedback must not
depend on animation alone.

## Pointer, keyboard, and mobile

Preserve `useHoverPopover` and its pointer bridge. Wire focus presence for the
content as needed so tabbing to the link or scrolling body keeps it open.
The scroll body has a localized accessible name and a visible focus indicator.
Test Escape and focus return without overriding trigger click behavior.

Use the existing `PRStatusChipDrawer` and `PRStatusChipMultiDrawer` as the
mobile exemplars. Retain their explicit tap entry and close control. The
GitHub drawer uses a stable 80dvh frame with its existing safe-area handling.
At viewport heights of 480px or less, use the full dynamic viewport height.
Hide the decorative handle and retain the drawer title for screen readers.
Pin Close at the top right and reserve space for it in the PR header and
selector. Apply top, left, and right cutout insets to compact content padding
and offset Close by its top and right insets. This compact composition keeps
the PR action visible at 240px
height while retaining the same state, actions, and scrolling details.
Keep the drawer title, PR tabs, PR identity, and Open PR action outside the
single scrolling details body. Disable the wrapper body's scrolling only for
this composition. Do not nest it around another vertical scroller.

Reserve horizontal space inside the details scroller for the automation
switches' extended touch targets. Matching negative margins preserve the
visible content alignment. Do not clip the targets to hide overflow.
On phones, let PR tabs share the available width and wrap when needed;
truncate repository labels while retaining reachable selection and unlink controls.
Keep a text space between the repository and PR number. Fix the phone tab
selector height to the smaller of 8rem and 20dvh, with rows aligned to its top.
Reserve this space for short lists so live association updates cannot move
the PR action while the multi-PR disclosure remains open. When it overflows, selection scrolls
vertically within that bound; status details retain their separate scroll owner.
Selection must leave the fixed PR action and a usable details region visible
on short screens.

Preserve current pointer-based drawer routing. Narrow fine-pointer popovers
still need viewport containment and 44px phone action targets. Share data and
handlers across both presentations. No new mobile data path is needed.

## Shared component compatibility

`change-request-ci-anatomy.tsx` and `change-request-status-chrome.tsx` also serve
GitLab and registered providers. Add only optional layout/header capabilities
with existing defaults, or use a GitHub-owned composition when simpler.
GitLab and plugin callers retain their current geometry and copy. Do not
change exported plugin contracts. Run their existing affected component tests
if implementation modifies shared rendering paths.

## Verification

Measure frame and link bounding boxes before and after separately released
summary and feedback responses. Keep differences within one CSS pixel for
subpixel rounding. Click the original link center with raw mouse coordinates
after each update. Locator auto-positioning alone can conceal this bug.

Cover topbar and chat chip, top/bottom placement, short viewport, one/multiple
PRs, tab switching, first load, cached refresh, empty/error content, and long
workflow lists. Mobile checks assert the drawer, fixed link, actual touch
target, internal scroll, safe-area containment, and no page overflow.

## Related contracts

- [Current GitHub check results](github-pr-check-results.md)
- [Shared task summaries](../../ui/system-design/pr-task-status-summary.md)
- [Feedback resource efficiency](../../ui/requirements/session-refresh-efficiency.md)
- [Implementation plan](../../../plans/stable-pr-status-disclosure/plan.md)
