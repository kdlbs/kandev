# Plans and Work Orders

## Purpose

Plans and work orders are delivery records. They describe how a change moves
the current system toward its requirements and system design.

They are not living product specifications.

## Plan

`docs/plans/<initiative>/plan.md` is the work-package manifest. It contains the
delivery overview, dependency order, risks, verification strategy, and links to
each work order.

A plan references the applicable requirement documents and system designs. It
does not copy their content.

## Work order

Each `task-<NN>-<slug>.md` file is one work order. A work order contains:

- A short outcome summary.
- In-scope responsibilities.
- Explicit exclusions.
- Applicable `REQ-*` and `AC-*` identifiers.
- Applicable system-design paths.
- One to three implementation acceptance conditions.
- Exact verification commands.
- Likely files and dependencies.
- Results after implementation.

Use a separate work order when work has an independent result or verification
boundary. Do not split work only by backend and frontend layers when a vertical
slice can remain functional.

## ASCII UI previews

When a feature or fix package creates or changes rendered UI, include an
**ASCII UI preview** section in `plan.md` and each work order that changes UI.
Backend-only work orders do not need one. For a small copy or control change,
show only the affected row or region; do not redraw an unrelated page.

- Give each view a stable label, such as `UI-01: Recipient selector`, and name
  its entry point and state. Use fenced `text` blocks with ASCII characters.
- Show the proposed control order, grouping, hierarchy, primary action, and
  navigation. Annotate fixed versus scrolling regions outside the drawing.
- Show before/after views when a fix or rearrangement depends on the difference.
  Only label a view as current behavior when source or rendered evidence supports it.
- Include desktop and phone compositions when they differ. For shared
  composition, one preview with explicit mobile behavior notes is sufficient.
  Follow `/mobile-parity`; shrinking the desktop drawing is not a phone design.
- Show changed empty, error, disabled, loading, or expanded states when they
  materially affect the requested interaction. Do not add unrelated states.
- State which structural choices are requirements and which details are
  illustrative. ASCII spacing is not a pixel specification. Use existing UI
  primitives and design tokens, and keep user-facing copy localized.
- Map the preview to the applicable `AC-*` criteria and targeted rendered checks.
  Drawings supplement behavior, accessibility, and responsive requirements;
  they do not replace them or prove the implementation works.

The plan owns the combined preview. Each UI work order includes its relevant
view or excerpt with the same label and a link to the plan. This keeps the
implementation packet usable without conversation history. If the design
changes, update both copies and the affected criteria together. A preview must
not silently contradict its requirement or system design.

In the conversation's final design-package summary, render a compact version
of the proposed UI inline, including a distinct phone view when needed. Link
the complete preview and identify material assumptions. File links alone do
not provide the requested visual review. A sketch does not add an approval
checkpoint or authorize implementation or delegation.

Implementers read the assigned preview before changing UI and compare the
rendered result against its structural requirements during the work order's
existing verification. Record any unresolved difference or update the package
when the user's direction changes; do not silently drift from the reviewed UI.

## Verification

Every work order owns the verification that proves its implementation result.
Use unit, integration, or end-to-end tests at the appropriate boundary.

A user-facing requirement needs end-to-end evidence somewhere in the work
package. A low-level work order does not need an artificial browser test.

Reference acceptance-criterion IDs from tests when that reference improves
traceability.
