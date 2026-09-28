---
id: coordinator-copilot-everywhere-design
title: Copilot on every workspace page design
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
requirements:
  - REQ-COORDINATOR-COPILOT-EVERYWHERE-001
  - REQ-COORDINATOR-COPILOT-EVERYWHERE-002
---

# Copilot on every workspace page System Design

## Purpose and boundaries

This design mounts the phase-1 copilot panel ([copilot panel](copilot-panel.md))
outside the Coordinator screens: a launcher and panel host in the workspace
shell for the board, task pages and the Inbox, a coordinator switcher, and
the page context chip of ADR D12. Expand stays dropped (D11). The
conversation, open sequence, attended-only rule and tool surface are
unchanged ([copilot](copilot.md)); the server adds no route and no field.

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-COORDINATOR-COPILOT-EVERYWHERE-001` | [Host](#host), [Store](#store), [Choosing the coordinator](#choosing-the-coordinator), [One right panel](#one-right-panel), [Phone](#phone) |
| `REQ-COORDINATOR-COPILOT-EVERYWHERE-002` | [Page context](#page-context), [Chip](#chip), [Server side](#server-side) |

## Host

`apps/web/app/coordinator/copilot/workspace-copilot-host.tsx` renders the
launcher and the panel. It is mounted once in the workspace layout that
wraps the board, task and Inbox routes (`spa-routes.tsx`), not per page, so
the panel survives navigation inside the workspace (`001.5`). It renders
nothing, and issues no request, unless all hold (`001.1`):

- `features.coordinator` and `features.coordinatorPhase2` are on;
- the viewer holds `workspace.manage` (`useWorkspaceTeamAccess`);
- the coordinator list for the workspace (already loaded by the sidebar's
  `use-coordinator-list`) is non-empty;
- the current route is not a settings route and not under
  `/workspaces/:id/coordinator/`, where the phase-1 copilot already renders.

The body is the phase-1 `CoordinatorCopilot` panel content with its
coordinator GET, open sequence, launcher busy state and profile messages
unchanged. Its header adds the switcher and **Open the coordinator page**
(a link to that coordinator's Needs you), and has no Expand (`001.4`).

## Store

The phase-1 copilot store (`hooks/domains/coordinator/copilot-store.ts`,
`{coordinatorId, open, chip, draft}`) is keyed by surface: the Coordinator
screens keep their instance and reset rules; the host gets a second instance
`{workspaceId, coordinatorId, open, chip, chipDismissedFor, draft}`. The
host instance resets to closed with no chip and no draft when the route's
workspace id changes (`001.5`). Both instances share the width key
`kandev.coordinatorCopilot.width`.

## Choosing the coordinator

Last used is stored in local storage under
`kandev.coordinatorCopilot.lastUsed.<workspaceId>` as a coordinator id. On
open, the host uses it when the list still contains it, else the first
coordinator in the list order (`created_at`, then `id`) (`001.2`). The
switcher is a select in the header, shown only with two or more
coordinators; choosing one writes last used and re-runs the phase-1 open
sequence for that coordinator, keeping the chip and clearing the draft
(`001.3`). A deleted last-used coordinator is ignored and overwritten at the
next choice.

## One right panel

The board's task preview and the copilot both render through
`RightSidePanel`. A small shell store `rightPanel: "preview" | "copilot" |
null` in the workspace layout decides which one shows: opening the copilot
sets `copilot` (the board sees `preview` lost and closes its preview through
its existing close path); opening a preview sets `preview`, which sets the
host's `open` to false (`001.6`). Pages without the preview only ever set
`copilot`.

## Phone

The host passes `mobileFullScreen` to `RightSidePanel`, the opt-in the
phase-1 panel added, so on phone width the panel is a full-screen sheet with
its Close control (`001.7`). The launcher sits above the mobile bottom
navigation.

## Page context

`usePageContext()` derives `{kind, id, label} | null` from the route only:

| Route | Context |
| --- | --- |
| task page `/workspaces/:id/tasks/:taskId` | `{kind: "task", id: taskId, label: <task identifier>}` from the task store |
| board `/workspaces/:id/workflows/:workflowId` (or the board's selected workflow) | `{kind: "workflow", id, label: <workflow name>}` |
| Inbox, anything else | `null` |

The label is display only; it never comes from typed text. A task not yet
in the store shows the chip with the id's short form until the task loads.

## Chip

- On open and on every route change, the host sets the chip to the page
  context unless `chipDismissedFor` equals the current route key; removing
  the chip sets `chipDismissedFor` to the route key, so it stays removed until
  the route changes (`002.1`, `002.2`). An **Ask about this** chip set on the
  Coordinator screens never reaches this instance.
- Chip text: "This task: <label>" or "This board: <label>", with a tooltip
  "Sent as an id; it reads the rest itself." and a remove button.
- Watched hint: the host reads the coordinator's `watches` from the
  coordinator GET it already holds; when scope is `selected` and the
  context's workflow (the task's `workflow_id`, or the workflow id) is not
  in it, the chip shows "Not watched by this coordinator" (`002.4`).
- `transformOutgoing` is the phase-1 function with the chip's `ref` set from
  the page context: it prefixes `About <label> [<kind>:<id>]: ` (`002.3`).
  `kind` is `task` or `workflow`; nothing else from the page is added.
- `user-message-body.tsx`'s coordinator branch already strips the bracketed
  reference into an "about <label>" tag, so `workflow` references render the
  same way.

## Server side

No new route or check. The standing instructions of
[copilot](copilot.md#standing-instructions) gain one sentence: a
`[workflow:<id>]` reference names a board, read with
`list_workflow_steps_kandev` and `list_tasks_kandev`. Every read the
coordinator makes with a chip's id passes the phase-1 workspace check and
the Watches filter of [permissions](permissions.md#watch-filter), so an id
from another workspace, an unwatched one, or one naming nothing reads as
not found (`002.4`, `002.5`). The prefix is text in a user message, so a
forged prefix typed by hand gains nothing more than the same reads.

## Security

- The host renders only for managers; the conversation route is
  `workspace.manage` as in phase 1.
- Only ids leave the page; titles and content are read by the coordinator
  through the guarded tools.

## Observability

No new logs. The existing conversation-open logs cover the panel.

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
