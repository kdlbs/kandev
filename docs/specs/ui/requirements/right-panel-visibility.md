---
status: active
system: ui
created: 2026-09-13
owners:
  - kandev
---

# Right panel visibility requirements

## Overview

Users need to reclaim task workspace width without losing the control that restores the right panels.
UI owns this reusable layout interaction. Task and terminal lifecycle remain outside this contract.
The related [layout profiles](task-layout-profiles.md) contract owns reusable arrangements, rather than this visibility control.

## Terminology

- **Right pane:** The rightmost side-by-side region in the current workbench arrangement.
  It can contain one tab group or a subtree of stacked groups. Default contains Files/Changes above Terminal.
  Plan Mode contains Plan; Preview Mode contains Browser; VS Code contains the editor.
- **Hidden pane:** The exact region removed by the most recent Hide action in the current environment and layout context.
- **Tablet fallback:** The two-column task view used between 768 and 1023 CSS pixels with a coarse pointer.

## Requirements

### REQ-UI-RIGHT-PANEL-VISIBILITY-001: Recoverable right panel visibility

**Intent:** Users can hide and show right panels independently of the left navigation sidebar.

#### Acceptance criteria

- **AC-UI-RIGHT-PANEL-VISIBILITY-001.1:** On desktop and tablet task pages, a persistent header button shall hide visible right panels and show hidden right panels.
  The button shall remain reachable in both states. Its position shall remain stable next to the layout controls.
- **AC-UI-RIGHT-PANEL-VISIBILITY-001.2:** Hiding the right panels shall release their width to the remaining workbench.
  Toggling shall preserve left-sidebar state, the active task and session, chat content, and unrelated center panels.
- **AC-UI-RIGHT-PANEL-VISIBILITY-001.3:** The control shall work in both tablet compositions and compact desktop mode.
  When only one workbench region exists and no hidden pane is retained, the control shall be disabled with an explanation.
  It shall neither hide the remaining conversation nor create a standard sidebar.
  Responsive defaults shall not change merely because the control exists.
- **AC-UI-RIGHT-PANEL-VISIBILITY-001.4:** The button shall expose its state and next action to assistive technology.
  Pointer, touch, Enter, and Space shall activate it. New icon buttons shall measure 28 pixels on fine pointers and at least 44 pixels on coarse pointers.
  Focus shall remain on the persistent button after activation. Labels and tooltips shall be localized.
- **AC-UI-RIGHT-PANEL-VISIBILITY-001.5:** Each layout shall retain visibility through its existing device-local restoration scope.
  Reloading the same context shall reflect the restored visibility in the button.
  Viewport changes shall not copy phone or tablet fallback state over the desktop layout.
- **AC-UI-RIGHT-PANEL-VISIBILITY-001.6:** Below 768 CSS pixels, Files and Terminal shall remain accessible through existing full-screen panel navigation.
  Returning to Chat shall reclaim the content surface without altering desktop panel visibility.
  Navigation and the new wider-view control shall not cause horizontal page overflow.
- **AC-UI-RIGHT-PANEL-VISIBILITY-001.7:** Before the active layout is ready, the header control shall be disabled rather than mutate another layout or session.
  During a layout transition, repeated activation shall not cause duplicate panels or lose the active session.

- **AC-UI-RIGHT-PANEL-VISIBILITY-001.8:** Hide shall select the rightmost side-by-side region from the current visual arrangement.
  Plan Mode shall toggle Plan, Preview Mode shall toggle Browser, and VS Code shall toggle its editor.
  Default shall toggle the stacked Files/Changes and Terminal region together.
  Custom layouts shall follow their current split order, independent of panel names, preset names, or legacy right-group IDs.
  Other regions shall keep their panels and selections. The control shall not hide a region containing the active Agent conversation.
- **AC-UI-RIGHT-PANEL-VISIBILITY-001.9:** Show shall restore the hidden region with its panel identities, parameters, selected tabs, and internal split arrangement.
  Its width shall return within current viewport limits. Edits to the remaining layout shall remain intact.
  Show shall not substitute Files/Changes/Terminal for a hidden Plan, Browser, editor, or custom region.
  The next click after Hide shall restore that region, rather than hide another region further left.
- **AC-UI-RIGHT-PANEL-VISIBILITY-001.10:** Hidden-pane recovery shall remain scoped to its task environment and current layout context across reload and task switches.
  An explicit preset selection, custom-layout application, or Reset Layout shall discard the previous hidden target for that environment.
  Closing a pane normally shall not create a hidden-pane recovery target.
  Stale or invalid recovery data shall not overwrite live panels, recreate unavailable panels, duplicate panel IDs, or reset the remaining layout.

## Compatibility and exclusions

The 2026-09-14 user correction supersedes standard-right-column reconstruction and compact-mode sidebar creation.
The persistent button and responsive compositions remain in place.
Exact restoration of the hidden region is now required, including custom groups.
It does not change saved default profiles, breakpoint thresholds, terminal process lifecycle, or left-sidebar persistence.
No new server setting, keyboard shortcut, runtime release toggle, or phone sidebar is required.
Existing saved layouts without hidden-pane data remain valid. Missing data does not authorize a fabricated sidebar.

## Design and delivery

- [System design](../system-design/right-panel-visibility.md)
- [Implementation plan](../../../plans/right-panel-visibility/plan.md)
