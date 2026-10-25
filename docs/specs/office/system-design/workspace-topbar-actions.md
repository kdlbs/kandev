---
status: draft
system: office
requirements:
  - REQ-OFFICE-KILL-SWITCH-006
---

# Office Workspace Topbar Actions

## Mapping

This composition amendment implements AC-OFFICE-KILL-SWITCH-006.14 and preserves
.4 through .13. [Kill-switch state](workspace-kill-switch-02.md) remains the
state/confirmation contract; only control placement changes.

## Components and flow

`OfficeShellChrome` currently mounts `WorkspacePauseBanner` below `PageShell`.
Its running branch renders a standalone refresh/pause row. Split action rendering
from state banners. Own exactly one `useWorkspacePause` controller per selected
workspace in shell scope; pass shared state/handlers to topbar actions and banners.
Compose global actions with `chrome.actions` through `PageShell.actions`, so page
filters or actions are not overwritten by workspace controls or vice versa.
Keep a banner only for paused/stale/unavailable/partial-stop state. Retain retry
stop in the partial-failure banner. No duplicate polling hooks or mutation owners.

Desktop: page actions, refresh pause state, pause/resume at the right of the same
topbar as the breadcrumb. Phone: a visible 44px workspace-actions trigger opens
an inset bottom drawer with refresh and pause/resume actions. Existing page and
Office navigation remain available. Reuse `mobile-menu-sheet.tsx` geometry and
`OfficePageNav` workspace navigation; do not compress the desktop button row.
Pause/resume use the existing confirmation flow, preserving reason and focus
return. Opening confirmation closes the action drawer first to avoid stacked
focus traps. A workspace change closes overlays and resets their action target.

## Geometry and failure states

The topbar stays outside the Office main scroller. Action drawer has one internal
scroll region, safe-area clearance, dynamic viewport containment and 44px minimum
phone/coarse-pointer targets. Desktop controls use normal 28px sizing. Paused and
unknown banners remain visible, and no optimistic success is inferred from a
failed refresh. Refresh continues to read pause state only, with an accessible
name that states this purpose. No new pause endpoints or authorization changes.

## Validation

Extend desktop/mobile workspace-kill-switch E2E with every state, agent/run and
export routes, page-action coexistence, confirmation cancellation, workspace
switching, partial-stop retry and failed refresh. Assert controls are inside the
topbar, not a second running-state toolbar, plus 44px phone target bounds and no
horizontal overflow at 390px and around the 768px phone breakpoint.
