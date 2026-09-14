# ADR-2026-09-06-inline-workflow-step-tabs: Keep workflow step tabs inside the existing editor

**Status:** accepted
**Date:** 2026-09-06
**Area:** frontend, workflow
**Supersedes:** 2026-09-05-workflow-editor-pipeline-inspector

## Context

The first workflow-editor redesign moved workflow editing to a dedicated route
with a new pipeline workspace, a large selected-step inspector, and separate
mobile journey, step, and action screens. The Agent, Automation, and Policies
grouping improves discoverability, but the new shell duplicates an established
workflow-editing experience and makes the tabs visually dominate the selected
step.

The current Workflows settings page already has the right product hierarchy:
workflow-level fields, a compact ordered step strip, and one inline editor for
the selected step. Its problem is control density inside that selected-step
editor, not the absence of a separate workflow workspace.

## Decision

Keep workflow editing in the existing workspace Workflows settings list and
workflow card. Do not add dedicated routes for existing workflows or new
workflow drafts. Keep the existing compact horizontal step strip; selecting a
step replaces the single inline selected-step panel below it.

Add Agent, Automation, and Policies as compact tabs inside that existing panel.
On desktop, render them as a small segmented control near the step heading,
with a visual height of 32 to 36 CSS pixels and a bounded width based on their
labels. Do not render three large full-width tab buttons. Coarse-pointer layouts
must still provide a 44-pixel hit area.

Keep lifecycle action recipes and focused action editing inside the Automation
tab. Selecting an action replaces that tab's recipe list locally and provides a
clear way back. Step, tab, and action selection are component state, not route
state or persisted workflow data.

On phones, preserve the same workflow-card hierarchy. Stack workflow metadata,
let the compact step strip scroll inside its own region, and render the selected
step and touch-safe tabs underneath. Do not introduce journey, full-height step,
or full-height action routes. The page keeps one vertical scroll owner and must
not gain document-level horizontal overflow.

The existing page-level manual-save coordinator continues to own every workflow
card. Multiple workflows may be dirty at once, and the shared Save changes
surface persists them together. The action catalog, derived view model,
validation, and immutable mutations remain shared implementation primitives.

## Consequences

- Authors learn one workflow editing model instead of choosing between an
  inline editor and a dedicated workspace.
- Tabs reduce selected-step density without consuming most of the panel header.
- Workflow name, description, default agent profile, and shared prompt remain
  available on both desktop and mobile because the existing card stays intact.
- New workflow drafts keep their current page-local identity remapping and save
  behavior.
- Configuration issues select the relevant step, tab, action, and field inside
  the current card rather than deep-linking to a route state.
- The dedicated editor routes, route selection helpers, desktop inspector shell,
  mobile journey screens, and their route-specific tests must be removed.
- The workflow wire format, script action contract, profile inheritance, and
  runtime behavior do not change.

## Alternatives considered

- Keep the dedicated editor but make its tabs smaller: rejected because it
  fixes visual weight but retains a second editing layout and route lifecycle.
- Keep both inline and dedicated editors: rejected because two mutation
  surfaces invite capability drift, duplicated tests, and unclear ownership.
- Replace the current strip with a Zapier-style vertical builder: rejected
  because the existing compact strip already communicates Kandev's ordered
  workflow and preserves information density.
- Use an n8n-style freeform canvas: rejected because Kandev does not persist
  arbitrary node coordinates or graph edges.
- Move the selected step into a desktop side panel: rejected because the
  existing inline panel is familiar, uses the available width for forms, and
  adapts to phone layouts without another navigation model.
