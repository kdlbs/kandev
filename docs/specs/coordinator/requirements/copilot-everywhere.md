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
- **Workspace page:** the board, a task page and the Inbox of a workspace.
- Other terms are defined in [copilot](copilot.md#terminology).

## Mockup

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
  the workspace has at least one coordinator, every workspace page shall show
  the launcher to a manager. Settings pages, the coordinator page (which has
  its own copilot), a workspace with no coordinator and a reader shall show
  no launcher.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.2:** When the manager opens the
  launcher, the panel shall open with the coordinator last used in that
  workspace in this browser, or the first coordinator in the phase-1 list
  order when none was used or it no longer exists.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.3:** When the workspace has more than
  one coordinator, the panel header shall offer a switcher; switching shall
  show that coordinator's conversation and remember it as last used.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.4:** The panel shall show the phase-1
  copilot for the chosen coordinator (the same conversation, messages, state
  line and composer) and **Open the coordinator page**, and shall have no
  Expand.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.5:** The panel shall stay open when
  the manager moves between pages of the same workspace, and shall close when
  the workspace changes or the manager closes it.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.6:** At most one right panel shall be
  open: opening the panel shall close the board's task preview, and opening
  the task preview shall close the panel.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-001.7:** On a phone-width screen, the
  panel shall open as a full-screen sheet with a close control.

### REQ-COORDINATOR-COPILOT-EVERYWHERE-002: Page context chip

**Intent:** The copilot knows what the manager is looking at, as an id only.

Mockup:

- [`docs/plans/workspace-coordinator-p2/assets/p2-03-task-page-copilot-context.png`](../../../plans/workspace-coordinator-p2/assets/p2-03-task-page-copilot-context.png): the chip in the composer.

#### Acceptance criteria

- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.1:** When the panel opens or the page
  changes on a task page, the composer shall show the chip "This task:
  <task identifier>"; on the board it shall show "This board: <workflow
  name>"; on the Inbox it shall show no chip.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.2:** The chip shall have the tooltip
  "Sent as an id; it reads the rest itself." and a remove control. A removed
  chip shall stay removed until the page changes.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.3:** When the manager sends a message
  with a chip, the client shall store the message with the phase-1 context
  prefix "About <label> [<kind>:<id>]: ", where `<label>` is the task
  identifier or the workflow name, `<kind>` is `task` or `workflow` and
  `<id>` is the entity id, and shall send nothing else from the page.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.4:** When the chip names a task or
  workflow the coordinator does not watch, the chip shall show "Not watched by
  this coordinator"; the message shall still send, and the coordinator's
  reads of that id shall return not found.
- **AC-COORDINATOR-COPILOT-EVERYWHERE-002.5:** A chip that names an id of
  another workspace, or of no entity, shall give the coordinator nothing
  beyond the id: its reads of that id shall return not found.

## Out of scope

- Expand to a full-page copilot (D11).
- Sending page content (titles, descriptions, diffs) with the context.
- A chip on the Inbox, settings pages or the coordinator page.
- A launcher for readers.
