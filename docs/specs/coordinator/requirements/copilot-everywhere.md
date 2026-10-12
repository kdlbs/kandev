---
id: coordinator-copilot-everywhere
title: Copilot on every workspace page
status: draft
system: coordinator
owners:
  - kandev
created: 2026-09-29
last_updated: 2026-09-29
---

# Copilot on every workspace page Requirements

## Overview

Phase 1 put the copilot on the coordinator page. Phase 2 adds a launcher to
every workspace page and opens the same copilot in a right panel. The panel
can carry the page's context as a chip: the task or board the manager is
looking at, sent as an id that the coordinator reads for itself (D12). The
panel has no Expand (D11); Open the coordinator page stays the way to a full
view.

## Terminology

- **Launcher:** the button that opens the copilot panel.
- **Panel:** the right-hand copilot panel on a workspace page.
- **Page context:** `{kind, id}` with kind `task` or `workflow`.
- **Chip:** the removable label showing the page context in the composer.
- **Workspace page:** the board, a task page or the Inbox, shown while a
  workspace is the active workspace (the one the sidebar shows). The
  workspace is the active workspace, not read from the page's address.
  Opening a task page makes that task's workspace the active workspace.
- **All Workflows board:** the board with no workflow selected, showing every
  workflow of the workspace.
- Other terms are defined in [copilot](copilot.md#terminology).

## Mockup

Where a mockup and an acceptance criterion disagree, the criterion governs.
The mockups show an Expand icon, a chip that carries a task title without the
"This task:" prefix and a visible caption instead of a tooltip; none of those
is built.

- [`docs/plans/workspace-coordinator-p2/assets/p2-03-task-page-copilot-context.png`](../../../plans/workspace-coordinator-p2/assets/p2-03-task-page-copilot-context.png): task page with the panel and the task chip.
- [`docs/plans/workspace-coordinator-p2/assets/p2-04-board-copilot-context.png`](../../../plans/workspace-coordinator-p2/assets/p2-04-board-copilot-context.png): board with the panel and the board chip.

## Requirements

### REQ-COORDINATOR-COPILOT-EVERYWHERE-001: Launcher and panel

**Intent:** A manager asks the copilot from where they are working.

**User story:** As a workspace manager, I want to open my coordinator's
copilot on the board or a task, so that I do not leave my work to ask it.

Mockup:

- [`docs/plans/workspace-coordinator-p2/assets/p2-04-board-copilot-context.png`](../../../plans/workspace-coordinator-p2/assets/p2-04-board-copilot-context.png): launcher and panel on the board.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.1:** While the phase-2 flag is on and
  the active workspace is a board-style workspace with at least one
  coordinator, every workspace page shall show the launcher to a manager.
  Settings pages, the coordinator page (which has its own copilot), an Office
  workspace, a workspace with no coordinator and a reader shall show no
  launcher. While the first read of the coordinator list, made when the
  launcher becomes eligible or the active workspace changes, is loading or has
  failed, no launcher shall show and no panel shall open. A later re-read (made
  when the panel opens) that is in flight or fails shall keep the previous list,
  so the launcher and an open panel stay. The closed launcher shall be named
  "Ask your coordinator" (it names no coordinator, because none is chosen
  until the panel opens), shall show no working state, and shall cause no
  read of a single coordinator while the panel is closed (the list read of
  001.1 still happens).
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.2:** When the manager opens the
  launcher, the panel shall open with the coordinator last used in that
  workspace in this browser, or the first coordinator in the phase-1 list
  order when none was used or it no longer exists; a last-used choice that no
  longer exists shall be forgotten when the panel opens.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.3:** When the workspace has more than
  one coordinator, the panel header shall offer a switcher; switching shall
  show that coordinator's conversation with an empty composer (text typed for
  the previous coordinator is not carried over) and remember it as last used.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.4:** The panel shall show the phase-1
  copilot for the chosen coordinator (the same conversation, messages, state
  line and composer) and **Open the coordinator page**, and shall have no
  Expand.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.5:** The panel shall stay open when
  the manager moves between workspace pages while the active workspace stays
  the same. It shall close, and the text typed in its composer shall be
  discarded, when the active workspace changes (including by opening a task of
  another workspace), when the manager leaves the workspace pages (Settings,
  the coordinator page, any other page) or when the launcher stops showing for
  any reason of 001.1. When the manager closes it, the typed text shall stay
  for that coordinator until one of those cases happens. A closed panel shall
  not reopen by itself: returning to a workspace page, and a page reload, show
  it closed. The panel follows the active workspace only: when a task page
  fails to load and the active workspace does not change, the panel stays open
  for that workspace and shows no chip.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.6:** At most one right panel shall be
  open: opening the panel shall close the board's task preview, and opening
  the task preview shall close the panel. A preview that opens by the
  manager's action or by an address carrying a task shall close the panel;
  a preview that only restores from the browser's saved state when the board
  appears shall not close an open panel, and closing the preview shall not
  reopen the panel. A preview closed only to make room for the panel shall not
  count as the manager closing it, and a panel that closes for any reason shall
  leave no stale claim on the right panel, so the saved preview restores the
  next time the board appears.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.7:** On a phone-width screen (the
  application's mobile breakpoint), the panel shall open as a full-screen
  sheet with a close control, covering the bottom navigation, and the
  launcher shall sit above the bottom navigation in both position and stacking,
  clearing its height and the device's bottom inset. On a task page that shows
  the walkthrough launcher, the copilot launcher shall sit directly above it
  so that neither covers the other.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.8:** When the coordinator shown in
  the panel no longer exists, the panel shall switch to the first remaining
  coordinator in list order and forget the stale last-used choice; when none
  remains, it shall close and the launcher shall disappear. When the panel
  reports the coordinator gone, the host shall re-read the coordinator list
  once, and "remaining" and "first" are measured against that re-read (against
  the previous list without the gone coordinator if the re-read fails). Only a
  choice made in the switcher shall write the last-used coordinator.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.9:** A response to a coordinator list
  read, a conversation open or a coordinator read shall be ignored when the
  active workspace, the coordinator or the page it was requested for has since
  changed, or when a later request of the same kind has been made; an ignored
  response shall not open, close or switch the panel, replace the chosen
  coordinator or change the chip or its hint.

### REQ-COORDINATOR-COPILOT-EVERYWHERE-002: Page context chip

**Intent:** The copilot knows what the manager is looking at, as an id only.

Mockup:

- [`docs/plans/workspace-coordinator-p2/assets/p2-03-task-page-copilot-context.png`](../../../plans/workspace-coordinator-p2/assets/p2-03-task-page-copilot-context.png): the chip in the composer.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.1:** When the panel opens or the page
  changes on a task page (the identifier and workspace being those the task
  page's own load put in the client's task store, on every way of reaching the
  page), the composer shall show the chip "This task:
  <task identifier>"; on the board with one workflow selected it shall show
  "This board: <workflow name>". It shall show no chip on the Inbox, on the
  All Workflows board (on every screen size), until the task or workflow has
  loaded, for a task that has no identifier, and for a task or workflow that
  does not belong to the active workspace.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.2:** The chip shall have the tooltip
  "Sent as an id; it reads the rest itself." (on its keyboard-focusable
  label) and a separate remove control. A removed chip shall stay removed,
  including if it is removed before its label has loaded, until the page
  changes. The page is the same while only its address query or hash changes,
  or while the same task is reached by either task address; it changes when
  the task, the selected workflow or the kind of page changes.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.3:** When the manager sends a message
  with a chip, the client shall store the message with the phase-1 context
  prefix "About <label> [<kind>:<id>]: ", where `<label>` is the task
  identifier or the workflow name alone (not the chip's "This task:" text), `<kind>` is `task` or `workflow` and
  `<id>` is the entity id, and shall send nothing else from the page. A chip
  that appears, changes or is removed shall not restart the conversation, open
  it again or discard the composer's text. A workflow name or task identifier
  containing ": " or a line break shall be sent with each run of blanks
  collapsed and each ": " replaced by " - ", as phase 1 does.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.4:** When the chip names a task or
  workflow the coordinator does not watch, the chip shall show "Not watched by
  this coordinator"; the message shall still send, and the coordinator's
  reads of that id shall return not found. The hint shall show only when the
  coordinator's read has loaded and its Watches select workflows and omit the
  chip's workflow (for a task, the workflow it is in); it shall not show when
  the Watches are all workflows, absent or not yet loaded once for this
  coordinator, and it shall follow the Watches as re-read when the panel opens,
  the page changes or the coordinator is switched. While such a re-read is in
  flight or has failed, the hint shall keep showing what the last loaded Watches
  of that coordinator gave.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.5:** A chip that names an id of
  another workspace, or of no entity, shall give the coordinator nothing
  beyond the id: its reads of that id shall return not found.

## Out of scope

- Expand to a full-page copilot (D11).
- Sending page content (titles, descriptions, diffs) with the context.
- A chip on the Inbox, the All Workflows board, settings pages or the
  coordinator page. Naming the focused workflow of the All Workflows board on a
  phone is a possible later addition.
- A launcher on Settings pages, the coordinator page or an Office workspace.
- Refreshing the launcher's coordinator list on a push event: it is re-read when
  the launcher becomes eligible, when the active workspace changes and when the
  panel opens.
- A launcher for readers.
