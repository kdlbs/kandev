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

Precondition: phase-1 task 11 (panel swap) has landed. It adds
`RightSidePanel` and its `mobileFullScreen` opt-in
([copilot panel](copilot-panel.md)); neither exists before it, and this
design builds on both.

The app has no workspace-scoped layout route. Only the coordinator screens
carry the workspace in the path (`/workspaces/:id/coordinator/...`); the
board is the catch-all `resolveKanbanRoute` (`/?workspaceId=&workflowId=`,
both optional), task pages are `/t/:taskId` and `/tasks/:taskId` with no
workspace in the path, and the Inbox is `NEEDS_YOU_INBOX_HREF`
(`/needs-you-inbox`), which spans workspaces. So the workspace is never read
from the URL: **the host's workspace is the store's
`workspaces.activeId`**, the workspace the sidebar shows and
`use-coordinator-list` loads. The board route and the app bootstrap already
set it; a task page does not change it.

`apps/web/app/coordinator/copilot/workspace-copilot-host.tsx` renders the
launcher and the panel. It is mounted once in `src/app-shell.tsx`, inside
`WorkspaceScopeProvider` beside `<main>`, not per route, so the panel
survives every navigation that keeps `workspaces.activeId` (`001.5`). It
renders nothing, and issues no request, unless all hold (`001.1`):

- `features.coordinator` and `features.coordinatorPhase2` are on;
- `WorkspaceScopeProvider` resolves a workspace in `kanban` mode (an Office
  workspace, or none, shows no launcher);
- the viewer holds `workspace.manage` on that workspace
  (`useWorkspaceTeamAccess`);
- the coordinator list for that workspace (already loaded by the sidebar's
  `use-coordinator-list`) is non-empty;
- `resolveSpaRoute` for the current location is `kanban`, `taskDetail` or
  `needsYouInbox`. Every other kind (`settings`, `canvasSettings`,
  `coordinator`, `office`, the auth kinds and the remaining top-level pages)
  shows no launcher; `coordinator` is where the phase-1 copilot renders.

These three route kinds are what the requirements call a workspace page.

The body is the phase-1 `CoordinatorCopilot` panel content with its
coordinator GET, open sequence, launcher busy state and profile messages
unchanged. Its header adds the switcher and **Open the coordinator page**
(a link to that coordinator's Needs you), and has no Expand (`001.4`).

## Store

The phase-1 copilot store (`hooks/domains/coordinator/copilot-store.ts`,
`{coordinatorId, open, chip, draft}`) is keyed by surface: the Coordinator
screens keep their instance and reset rules; the host gets a second instance
`{workspaceId, coordinatorId, open, chip, chipDismissedFor, draft}`. The
host instance resets to closed with no chip and no draft when
`workspaces.activeId` differs from its `workspaceId` (the sidebar's
workspace switch, or a board link carrying another `workspaceId`); that is
the "workspace changes" of `001.5`. Moving between the board, task pages and
the Inbox never changes it, so the panel stays open. Both instances share the width key
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
null` in the app shell, beside the host, decides which one shows: opening the copilot
sets `copilot` (the board sees `preview` lost and closes its preview through
its existing close path); opening a preview sets `preview`, which sets the
host's `open` to false (`001.6`). Pages without the preview only ever set
`copilot`.

## Phone

The host passes `mobileFullScreen` to `RightSidePanel`, the opt-in phase-1
task 11 adds (see the precondition in [Host](#host)), so on phone width the panel is a full-screen sheet with
its Close control (`001.7`). The launcher sits above the mobile bottom
navigation.

## Page context

`usePageContext()` derives `{kind, id, label} | null` from the resolved
`SpaRoute` and the store, never from typed text:

| Route kind | Context |
| --- | --- |
| `taskDetail` (`/t/:taskId`, `/tasks/:taskId`) | `{kind: "task", id: taskId, label: <task identifier>}` once the task is in the task store and its `workspace_id` equals the host's workspace; otherwise `null` |
| `kanban` | `{kind: "workflow", id: workflows.activeId, label: <workflow name>}` once `workflows.activeId` is set and that workflow is loaded; otherwise `null` |
| `needsYouInbox` | `null` |

The board's workflow is `workflows.activeId`, which `kanban-route.tsx` sets
from the `workflowId` query parameter or the saved workflow filter, so a
board opened without the parameter still gets its chip. A task page for a
task of another workspace has no chip (the host still shows that
workspace's coordinator).

**While loading there is no chip.** Until the label resolves the context is
`null`, so nothing is sent with a message typed in the meantime and no short
id ever reaches a prefix (`002.3`). The chip appears when the label
resolves; `002.1`'s "when the panel opens or the page changes" is measured
from that moment.

The label is the one field of the page besides the id that leaves it: the
task identifier (the display identifier phase-1 cards show, never the
task's title) or the
workflow's name. ADR D12 records the workflow name as the one permitted
label; it is a name the manager chose for the board, not task content.

## Chip

- On open, on every route change, and when the page context turns from
  `null` to a value (its label loaded), the host sets the chip to the page
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
