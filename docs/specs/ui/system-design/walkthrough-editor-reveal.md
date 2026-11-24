---
status: current
system: ui
requirements:
  - REQ-UI-WALKTHROUGH-EDITOR-REVEAL-001
---

# Walkthrough Editor Reveal System Design

## Purpose and boundaries

The walkthrough launcher opens the source file. The active Monaco instance
owns scrolling to the active walkthrough range in its own viewport.
Repository and file matching use the current walkthrough step and existing
editor identity.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-UI-WALKTHROUGH-EDITOR-REVEAL-001` | [Reveal lifecycle](#reveal-lifecycle) |

## Reveal lifecycle

`WalkthroughFloatingWindow` requests the file through the existing file opener.
`useMonacoWalkthroughRange` applies decorations and measures the marker from
the Monaco model and viewport. The decoration can exist before the range is
in view.

Keep the range reveal pending until the target model is attached and the
editor has a usable layout. Use Monaco's layout or model events when readiness
arrives after the step changes. Apply the reveal after Monaco has restored
the editor's initial view. Use the current clamped range. Confirm the target
lines are inside the editor viewport before treating the reveal as complete.
For a range taller than the viewport, reveal its start.

Bind pending work to the current editor, model, walkthrough step, and range.
Discard it when the user changes steps or closes the walkthrough. After a
successful reveal, keep the existing geometry updates for the marker and
connector. The editor remains free to scroll under user control.
