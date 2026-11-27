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
Precondition: phase-1 [task 11, panel swap](../../../plans/workspace-coordinator/task-11-panel-swap.md), which adds `RightSidePanel` and its `mobileFullScreen` opt-in that this design builds on.

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
`use-coordinator-list` loads. The board route and the app bootstrap set it.
**A task page also sets it**: hydration writes `workspaces.activeId =
task.workspace_id` (`lib/ssr/session-page-state.ts`). Opening a task of another
workspace, from the Inbox for instance, therefore is a workspace change: the
host resets and the panel closes (`001.5`) and the launcher then offers that
workspace's coordinators. A `null` `workspaces.activeId` is not a workspace:
the host is ineligible and reset.

`apps/web/app/coordinator/copilot/workspace-copilot-host.tsx` wraps the shell's
`<main>` in `src/app-shell.tsx`, inside `WorkspaceScopeProvider`, and is
mounted for the life of the app, not per route. `RightSidePanel` takes the
page as its required `main` prop, so the host always renders it with
`mainSizing="fluid"` and `open` false when not eligible: the route tree is one
stable element and never remounts when the launcher appears, the panel opens
or the panel flips between inline and floating. The panel therefore survives
every navigation that keeps `workspaces.activeId` (`001.5`).

The host is **eligible**, and shows the launcher, only when all hold, checked
in this order so that an earlier failure issues no request (`001.1`):

1. `features.coordinator` and `features.coordinatorPhase2` are on;
2. `WorkspaceScopeProvider` resolves a workspace in `kanban` mode (an Office
   workspace, or none, shows no launcher);
3. the viewer holds `workspace.manage` on it, read as
   `hasScope(workspace?.scopes, SCOPE.workspaceManage)` from the scope the
   provider already holds, as the phase-1 call sites do. `useWorkspaceTeamAccess`
   is the Team Access card's hook (it lists members and the directory) and is
   not used;
4. the current location resolves to the route kind `kanban`, `taskDetail` or
   `needsYouInbox`. The host resolves it with the router's own resolver and
   passes the same feature options `SpaRoutes` passes
   (`coordinatorEnabled`, `needsYouInboxEnabled` and the rest), because with
   the defaults `/needs-you-inbox` and the coordinator paths resolve to
   `kanban`. Every other kind (`settings`, `canvasSettings`, `coordinator`,
   `office`, the auth kinds and the remaining top-level pages) shows no
   launcher; `coordinator` is where the phase-1 copilot renders. The router
   exports a small hook that returns the resolved kind so the host and
   `SpaRoutes` cannot disagree;
5. the workspace's coordinator list has loaded and is non-empty.

These three route kinds are what the requirements call a workspace page.

**The coordinator list.** The host calls `useCoordinatorList` once (the phase-1
hook holds its state in the component and has no shared cache, so this is the
host's own read) and passes it a workspace id only once checks 1 to 4 hold, so
nothing is requested before then. The list is read when the host becomes
eligible by checks 1 to 4 (entering a workspace page from any other page), when
`workspaces.activeId` changes and when the panel opens; a coordinator created
or deleted elsewhere therefore appears or disappears at the next of those, and
not by push. While the list is undefined (loading) or its first read failed
with no earlier value, check 5 fails: no launcher, and the host does nothing
else. A re-read made when the panel opens keeps the previous list while it is in
flight and when it fails, so check 5 keeps holding and the panel stays open;
only the first read after eligibility or a workspace change gates the launcher
(`001.1`). Every list read, open sequence and coordinator GET carries the
`(workspaceId, coordinatorId, route key)` it was made for and a per-kind
sequence number; a response whose triple no longer matches the host's, or that
is not the latest request of its kind, is dropped without effect (`001.9`).
Opening the panel chooses the coordinator from the list as it is and starts the
open sequence at once, without waiting for the re-read; if the re-read then no
longer holds that coordinator, `001.8` switches and the first open sequence's
result is dropped by the rule above. A list read that returns empty
while the panel is open closes the panel and the launcher goes ([Choosing the
coordinator](#choosing-the-coordinator), `001.8`).

Losing eligibility for any reason (flag off, scope revoked, a non-workspace
route, an empty list, a workspace change) sets the host instance to closed with
no chip and no draft. It does not keep `open` for later, so the panel never
reappears by itself when eligibility returns; it reappears only when the manager
opens it (`001.5`). A page reload starts closed because the host instance is
not persisted.

The body is the phase-1 `CoordinatorCopilot` panel content with its
coordinator GET, open sequence, launcher busy state and profile messages.
Its header adds the switcher and **Open the coordinator page** (a link to that
coordinator's Needs you), and has no Expand (`001.4`). Following that link
leaves the workspace pages, so the panel is closed (above); the coordinator
page's own copilot opens by its own rules. The mockups' Expand icon is not
built, and where a mockup and an AC disagree the AC governs.

**The launcher.** The closed launcher is the host's own control, not the
phase-1 one: its accessible name and tooltip are the new string "Ask your
coordinator", it names no coordinator and shows no working state, and no
coordinator GET is made while the panel is closed. The coordinator is chosen
when the panel opens (see [Choosing the coordinator](#choosing-the-coordinator)).
It is fixed at the bottom right; on a task page that shows `WalkthroughOverlay`'s
launcher (same corner) it sits directly above that launcher, and on a phone its
offset clears the bottom navigation's height plus the safe-area inset, with a
stacking order above it (`001.7`). The launcher's strings go through `t()` in
all six locales.

**Task page load failure.** The host follows `workspaces.activeId` only. If a
task page fails to load, `activeId` is unchanged: the panel stays open for the
previous workspace, its coordinators are that workspace's, and the page context
is `null` (no chip) because the task never reaches the store (`001.5`).

**Reuse boundary.** `CoordinatorCopilot` today owns both its launcher and its
`RightSidePanel`, and reads a module singleton store. The host cannot render
it as panel content unchanged. Build splits it: the panel content (header,
conversation body, composer, state line) becomes a component that takes the
store handle and the header slots, and the phase-1 `CoordinatorCopilot` becomes
a thin wrapper of that content with its own launcher and panel and the
singleton store. The host is a second wrapper, with the host's launcher, the
host's `RightSidePanel` and the host store. Phase-1 behaviour and every
phase-1 test are unchanged.

## Store

`hooks/domains/coordinator/copilot-store.ts` today exports one module singleton
(`{coordinatorId, open, chip, draft}` with `keepOnlyFor`), which
`useCoordinatorCopilot`, `CoordinatorCopilot` and
`CoordinatorCopilotResetBridge` (`src/app-shell.tsx`) are hard-wired to. Build
turns it into a `createCopilotStore()` factory and keeps the existing
export as the instance the factory made for the Coordinator screens, and `useCoordinatorCopilot` takes the
store handle as a parameter (default: the singleton). The Coordinator screens
keep that instance, its reset rules and the bridge, which acts only on it; the
bridge's `keepOnlyFor(null)` on every non-coordinator path must never touch the
host's instance. The host gets a second instance of the same slot
(`{coordinatorId, open, chip, draft, draftsSwept}`) plus
`{workspaceId, pageChip, chipDismissedFor}`. On the host instance `chip` and
`draft` (phase 1's one-shot **Ask about this** seed) are never set.

`pageChip` is a field of its own. The phase-1 `chip` (**Ask about this**) is
never set on the host instance, and the phase-1 controller's `chip` effects
(which re-run the open sequence and bump `askKey`, re-keying the composer view)
run on `chip` only. A page chip that appears, changes or is removed therefore
never re-runs the open sequence and never remounts the composer, so moving from
task to task with the panel open does not POST the conversation again
(`002.3`). The open sequence runs only on open, on a coordinator switch and on
the phase-1 Retry.

The host instance resets to closed with no chip and no draft whenever it loses
eligibility or `workspaces.activeId` differs from its `workspaceId` (the
sidebar's workspace switch, a board link carrying another `workspaceId`, or a
task page of another workspace); `workspaceId` is set when the manager opens the
panel. Moving between the board, task pages and the Inbox in one workspace
changes none of these, so the panel stays open. Both instances share the width
key `kandev.coordinatorCopilot.width`. **Typed text.** "Draft" in `001.3` and `001.5` means the text the manager typed
in the composer, which phase 1 keeps per conversation session in browser
storage and wipes through `draftsSwept`, once per slot lifetime, before the
composer mounts. A host reset (eligibility lost, workspace change) returns the
host instance to the initial slot with `draftsSwept` false, so the next open
sweeps the typed text for that session. A plain manager close leaves the slot
and its typed text as they are, and reopening shows it. A coordinator switch
gives the new coordinator a fresh slot (`coordinatorId` differs), so its
composer starts empty; the previous coordinator's stored text is not carried
over.

## Choosing the coordinator

Last used is stored in local storage under
`kandev.coordinatorCopilot.lastUsed.<workspaceId>` as a coordinator id. On
open, the host uses it when the list still contains it, else the first
coordinator in the list order (`created_at`, then `id`) and removes the stale
stored value (`001.2`). An
unreadable, malformed or unwritable storage value is treated as absent, and a
failed write is ignored (the choice then lasts for the open panel only). The
switcher is a select in the header, shown only with two or more coordinators;
choosing one writes last used, sets the host's `coordinatorId` and re-runs the
phase-1 open sequence for that coordinator, keeping the page chip and clearing
the draft (`001.3`). Only that choice writes last used: the fallback, the phase-1
Coordinator screens and sending a message do not.

When the panel's coordinator is gone (the open sequence or the coordinator GET
reports it unknown, or a list re-read no longer holds it), the host clears that
stale last-used value and, unless the signal already came from a list re-read,
re-reads the coordinator list once (the list held in the host is otherwise
stale, since it is not pushed). "Remaining" is measured against that re-read;
if the re-read fails it is the previous list minus the gone id. The host
switches to the first remaining coordinator in list order and runs the open
sequence for it; with none remaining it replaces the list with the empty result,
closes, and check 5 of [Host](#host) fails, so the launcher disappears
(`001.8`).

## One right panel

The board's task preview and the copilot both render through
`RightSidePanel`. A small shell store `rightPanel: "preview" | "copilot" |
null` in the app shell, beside the host, decides which one shows: it is the one
writer. Opening the copilot sets `copilot`, and the board closes its preview
through its existing close path when it sees `preview` lost; opening a preview
sets `preview`, which sets the host's `open` to false (`001.6`). Closing either
one, by its control or Escape, sets `null`; it never reopens the other. Pages
without the preview only ever set `copilot`. The host instance's `open` and
`rightPanel === "copilot"` are written only together, by the one shell setter,
and are never independent; an ineligible host is closed on both. Every path that writes `null` is a copilot close and none is a forced
one: the close control and Escape, the host reset (eligibility lost, workspace
change, leaving the workspace pages), no coordinator remaining (`001.8`) and
following Open the coordinator page; each sets `null` only while `rightPanel` is
`copilot`, so it never overwrites `preview`. A preview closed to make room for
the panel goes through the board's close path with the shell store told to
ignore that one close, so it writes nothing and `rightPanel` stays `copilot`
(`001.6`).

The board's preview also restores itself on mount from the browser's saved
state (`useKanbanPreview`), and opens from a `?taskId=` address. The rule:
a preview open request made by the manager or by an address carrying a task
(`?taskId=` on navigation or first load) sets `preview` and closes the panel.
The passive restore from saved state when the board appears never displaces an
open panel: it runs only when `rightPanel` is not `copilot`. After a page
reload the panel is closed, so a saved preview restores as before.

## Phone

The host passes `mobileFullScreen` to `RightSidePanel`, the opt-in phase-1
task 11 adds (see the precondition in [Host](#host)), so on phone width (the
application's mobile breakpoint, `useResponsiveBreakpoint().isMobile`) the
panel is a full-screen sheet with its Close control (`001.7`). The sheet
stacks above the session mobile bottom navigation, which is also fixed at the
bottom of the viewport, so the composer is never covered. The closed launcher
sits above the mobile bottom navigation.

## Page context

`usePageContext()` derives `{kind, id, label} | null` from the resolved route
kind and the store, never from typed text. "The task store" is `kanban.tasks`
and the workflow list is `workflows.items`:

| Route kind | Context |
| --- | --- |
| `taskDetail` (`/t/:taskId`, `/tasks/:taskId`) | `{kind: "task", id: taskId, label: <task identifier>}` once the task is in `kanban.tasks`, its `workspaceId` is present and equals the host's workspace, and it has an `identifier`; otherwise `null` |
| `kanban` | `{kind: "workflow", id: workflows.activeId, label: <workflow name>}` once `workflows.activeId` is a non-null id of a workflow in `workflows.items` whose `workspaceId` equals the host's workspace; otherwise `null` |
| `needsYouInbox` | `null` |

`workflows.activeId === null` is the persisted **All Workflows** selection, not
"loading" (`lib/kanban/resolve-workflow.ts`); the context is `null` there, on
every screen size. On a phone the All Workflows board shows one focused workflow
(`mobileKanban.focusedWorkflowId`), but the chip still follows only
`workflows.activeId`; naming the focused workflow is out of scope. A stale
`workflows.activeId` left over from another workspace is not in that workspace's
items, so it yields `null`.

**Where the task label comes from.** The task's `identifier` and `workspaceId`
in `kanban.tasks`, as the task page's own load puts them there. Today neither
hydration path fills them: `snapshotToState` (`apps/web/lib/ssr/mapper.ts`)
sets `workspaceId` but not `identifier`, and the boot mapper in
`apps/backend/internal/backendapp/boot_state_routes.go` (a camelCase
whitelist) lists neither. Build adds `identifier` to `snapshotToState` and
`identifier` and `workspaceId` to the boot whitelist, from the task DTO fields
of the same names, each with a test that a task page reached by in-app
navigation and one reached by a page load both yield the chip
(`002.1`); the task page reads no other source for the label.

A task without an `identifier` gives `null`: the phase-1 cards fall back to the
title, but ADR D12 forbids sending a title, so the task chip does not use that
fallback. A task page for a task of another workspace makes that workspace the
active workspace (see [Host](#host)), so the panel closes; the workspace check
above guards only the instant before that hydration lands, when the task is in
the store but `workspaces.activeId` still names the previous workspace.

**While loading there is no chip.** Until the label resolves the context is
`null`, so nothing is sent with a message typed in the meantime and no short
id ever reaches a prefix (`002.3`). The chip appears when the label
resolves; `002.1`'s "when the panel opens or the page changes" is measured
from that moment. When the route key changes the previous chip clears in the
same render, so a message is never sent with the previous page's chip.

The label is the one field of the page besides the id that leaves it: the
task identifier (the display identifier phase-1 cards show, never the
task's title) or the
workflow's name. ADR D12 records the workflow name as the one permitted
label; it is a name the manager chose for the board, not task content. If the
task or workflow is renamed while the chip shows, the label follows the store;
if it is deleted, the context becomes `null` and the chip clears.

## Chip

The **route key** is the page's identity for the chip: `"<route kind>:<context
id>"`, where the route kind is the resolved route kind and the context
id is the task id on `taskDetail` (either task address gives the same key),
`workflows.activeId` on `kanban` (empty for All Workflows), and empty on
`needsYouInbox`. So moving to another task, or switching the
board's workflow, is a page change; a query or hash change that keeps both
parts (a filter, a search, an opened preview) is not. A workspace change
resets the host instance, `chipDismissedFor` included ([Store](#store)).
"The page changes" in `002.1` and `002.2` means the route key changes.

- On open, on every route key change, and when the page context turns from
  `null` to a value (its label loaded), the host sets `pageChip` to the page
  context unless `chipDismissedFor` equals the current route key; removing
  the chip sets `chipDismissedFor` to the route key in the same update, even
  before its label has loaded, so a late label never restores it. It stays
  removed until the route key changes (`002.1`, `002.2`). An **Ask about this**
  chip set on the Coordinator screens never reaches this instance.
- Chip text: "This task: <label>" or "This board: <label>". The tooltip
  "Sent as an id; it reads the rest itself." is on the chip's label element, which
  is focusable so keyboard users reach it; the remove button is a separate control.
- Watched hint: the host reads the coordinator's `watches` from the
  coordinator GET it already holds, and re-fetches that GET when the panel opens,
  when the route key changes while open and when the coordinator is switched (the
  host is mounted for the whole session, so a once-per-id read would go stale
  after a Watches edit in Settings). The hint "Not watched by this coordinator"
  shows only when the read has loaded, `watches.scope` is `selected`, and the
  context's workflow (the workflow id, or the task's `workflowId` from
  `kanban.tasks`) is not in `watches.workflow_ids`. It does not show when the
  scope is all workflows, when `watches` is absent, when no read of this
  coordinator has loaded yet, or for a task with no known workflow (`002.4`).
  While a re-fetch is in flight or has failed, the last loaded `watches` of the
  same coordinator keeps deciding the hint; a response for an earlier route key,
  coordinator or workspace is dropped (`001.9`). It is text
  beside the chip, not a change to the chip's label or to what is sent.
- `transformOutgoing` is the phase-1 function with the reference taken from
  `pageChip`: it prefixes `About <label> [<kind>:<id>]: ` (`002.3`), where the
  label is the chip's `label` field alone (the identifier or workflow name, not
  the rendered "This task:" text), not its id as on the Coordinator screens.
  Build passes the label to it through `normalizeCopilotItemId`, so a workflow
  name containing ": " or a line break cannot break the parser. `kind` is
  `task` or `workflow`; nothing else from the page is added. The prefix is
  applied to the stored text of a sent message only; an empty message is not sent,
  as in phase 1.
- Transcript rendering: `user-message-body.tsx` matches the bracketed reference
  with `COORDINATOR_REFERENCED_PREFIX_RE`, which accepts only
  `task|proposal|stall`, and `CopilotItemRefKind` in
  `lib/coordinator/copilot-id.ts` has the same three kinds. **Build widens both to
  include `workflow`**, with unit cases for a workflow reference (the tag reads
  "about <label>" and the bracket does not show), for a label with ": " and for
  the three existing kinds unchanged. Nothing else in the transcript changes.

## Server side

No new route or check. The standing instructions of
[copilot](copilot.md#standing-instructions) (`internal/coordinator/prompt.go`)
gain one sentence: a `[workflow:<id>]` reference names a board, read with
`list_workflow_steps_kandev` and `list_tasks_kandev`. This work order owns that
change and its `prompt_test.go` case. Standing instructions are sent at
conversation start only, so a phase-1 conversation that is still open when this
ships never received the sentence; a `[workflow:<id>]` chip sent into it names an id
its agent was not told how to read. That is accepted: the agent can still read
the id with the task tools' own descriptions, and a new conversation carries
the sentence. Every read the coordinator makes with a chip's id passes the
phase-1 workspace check and the Watches filter of
[permissions](permissions.md#watch-filter), so an id from another workspace, an
unwatched one, or one naming nothing reads as not found (`002.4`, `002.5`); a
server test asserts that for both a task id and a workflow id. The prefix is text
in a user message, so a forged prefix typed by hand gains nothing more than the
same reads.

## Security

- The host renders only for managers; the conversation route is
  `workspace.manage` as in phase 1.
- Only ids leave the page; titles and content are read by the coordinator
  through the guarded tools.

## Observability

No new logs. The existing conversation-open logs cover the panel.

## Related decisions

- [Coordinator phase 2, a person approves everything](../../../decisions/2026-09-29-coordinator-phase-2-control.md)
