# ADR-2026-09-05-workflow-editor-pipeline-inspector: Use a constrained pipeline and focused workflow inspector

**Status:** superseded by 2026-09-06-inline-workflow-step-tabs
**Date:** 2026-09-05
**Area:** frontend, workflow

## Context

The current workflow settings page renders each workflow as a large card and
expands one selected step into a single form containing agent selection,
session behavior, prompts, transitions, automation, and policy toggles. Adding
script actions would make that form denser and make the workflow harder to
understand as a whole.

Workflow definitions are ordered state pipelines. They are not general-purpose
graphs: step order and typed transition actions already determine the runtime
structure. A freeform node canvas would add positioning, zoom, edge editing,
and mobile interaction problems without adding a supported runtime capability.

Desktop authors benefit from keeping workflow context visible while editing a
step. Phone authors need focused screens and predictable Back navigation rather
than a horizontally scrolling desktop canvas or nested expanding cards.

## Decision

Opening a workflow from workspace settings uses a dedicated editor route.
Workspace settings retains a compact list for workflow-level ordering and
operations. New workflow creation uses a sibling route with client-only draft
identities; its first successful manual save replaces the URL with the durable
workflow identity.

The editor presents a constrained pipeline derived from persisted step order
and transitions. It does not persist node positions or provide arbitrary edge
creation, pan, or zoom.

On desktop, the pipeline remains visible beside a persistent inspector. The
selected step inspector has **Agent**, **Automation**, and **Policies** tabs.
Automation is represented as ordered lifecycle recipes for task entry, agent
completion, and task exit. Selecting one compact action row replaces the list
with one focused action editor.

On phone viewports, the same workflow is composed as a vertical step journey.
Step and action selection use dedicated full-height editor states with Back
navigation. A bottom drawer may be used to choose an action type, but not as the
primary editor. Reordering has explicit move controls and does not depend on
drag.

Desktop and mobile share the workflow draft, action catalog, derived summaries,
validation diagnostics, and mutations. They do not share one compressed layout.
The existing settings manual-save coordinator owns persistence and dirty-route
navigation. The redesign does not add autosave, new workflow-level inheritance,
or new persisted editor metadata.

Step, tab, trigger, and action selection are shallow route state above a
route-level draft shell. This gives browser Back predictable mobile behavior
without treating selection or action positions as persistent workflow data.

## Consequences

- Authors see workflow shape and configuration issues before opening detailed
  controls.
- Adding action types increases the action catalog instead of expanding every
  step card.
- Existing actions must move into lifecycle descriptors so the new Automation
  tab does not coexist with a second legacy action form.
- The route shell must retain unsaved drafts across step, tab, and action
  navigation and integrate with the shared Save changes surface.
- Configuration diagnostics need resolvable selection targets for a step, tab,
  action, and field.
- Desktop can use an internally scrolling pipeline, while phone screens have a
  single vertical scroll owner and no document-level horizontal overflow.
- Read-only synchronized workflows use the same information architecture with
  disabled mutations and a visible reason.
- The workflow wire format, transition semantics, and executor behavior remain
  unchanged by the editor architecture.

## Alternatives considered

- Extend the current expanded step card: rejected because every new action or
  policy increases the always-visible form and obscures workflow context.
- Use only a Zapier-style vertical outline at all widths: rejected because it
  makes focused editing clear but underuses desktop width and weakens the
  overview of transitions between steps.
- Use an n8n-style freeform canvas: rejected because Kandev workflows do not
  support arbitrary graph topology, and canvas mechanics would become a new
  product and persistence contract.
- Use the same responsive component tree on desktop and phone: rejected because
  shrinking a two-pane pipeline and inspector produces nested scrolling and
  poor touch interactions. Shared state and mutations are sufficient parity.
- Add workflow-level defaults for every step policy: deferred because it would
  change inheritance and persistence semantics rather than only improve the
  editor experience.
- Add a live Test action button: deferred because executing a command from
  settings needs a separate side-effect, authorization, session-binding, and
  audit contract.
