---
id: "10-copilot-everywhere"
title: "Copilot on every workspace page"
status: pending
wave: 2
depends_on:
  - "01-shared-interface"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COPILOT-EVERYWHERE-001
  - REQ-COORDINATOR-COPILOT-EVERYWHERE-002
acceptance_criteria:
  - AC-COORDINATOR-COPILOT-EVERYWHERE-001.1
  - AC-COORDINATOR-COPILOT-EVERYWHERE-001.2
  - AC-COORDINATOR-COPILOT-EVERYWHERE-001.3
  - AC-COORDINATOR-COPILOT-EVERYWHERE-001.4
  - AC-COORDINATOR-COPILOT-EVERYWHERE-001.5
  - AC-COORDINATOR-COPILOT-EVERYWHERE-001.6
  - AC-COORDINATOR-COPILOT-EVERYWHERE-001.7
  - AC-COORDINATOR-COPILOT-EVERYWHERE-002.1
  - AC-COORDINATOR-COPILOT-EVERYWHERE-002.2
  - AC-COORDINATOR-COPILOT-EVERYWHERE-002.3
  - AC-COORDINATOR-COPILOT-EVERYWHERE-002.4
  - AC-COORDINATOR-COPILOT-EVERYWHERE-002.5
system_design:
  - ../../specs/coordinator/system-design/copilot-everywhere.md
---

# Task 10: Copilot on Every Workspace Page (WP-9)

## Summary

Mount a workspace-level copilot host: a launcher on every workspace page, the
phase-1 copilot in the shared right panel with a coordinator switcher, one
right panel at a time with the board's task preview, a phone sheet, and the
page chip for tasks and boards.

## Precondition

Phase-1 [task 11, panel swap](../workspace-coordinator/task-11-panel-swap.md)
has merged. It adds `RightSidePanel` (extracted from
`components/kanban-with-preview.tsx`) and its `mobileFullScreen` opt-in.
Before starting, confirm `grep -rl RightSidePanel apps/web/components` finds
the component; if it does not, this work order is blocked, not to be built
around.

## In scope

- Host in `src/app-shell.tsx` inside `WorkspaceScopeProvider`, for the
  `kanban`, `taskDetail` and `needsYouInbox` route kinds, with the workspace
  taken from the store's `workspaces.activeId`, never from the URL, and the
  visibility rules (`001.1`).
- Last-used coordinator per workspace in local storage, fallback to the
  first in list order (`001.2`); switcher (`001.3`).
- Panel body is the phase-1 `CoordinatorCopilot` with Open the coordinator
  page and no Expand (`001.4`).
- Store keeps the panel open across pages of one workspace, closes on a
  workspace change (`001.5`); one-right-panel rule with the task preview
  (`001.6`); `mobileFullScreen` sheet (`001.7`).
- Page chip from the resolved `SpaRoute` and the store: "This task:
  <identifier>" once the task is loaded and of the host's workspace, "This
  board: <workflow>" from `workflows.activeId`, none on the Inbox and none
  while the label is loading; tooltip, remove until the page changes; the
  phase-1 `transformOutgoing` prefix (`002.1` to `002.3`).
- "Not watched by this coordinator" from the Watches in the coordinator
  read (`002.4` client half; the server not-found half is task 02's guard);
  a foreign or unknown id sends only the id (`002.5`, a server test that
  the read tools return not found for another workspace's id).

## Out of scope

- An Expand into Quick Chat (D11, not built).
- Any new coordinator read tool.

## ASCII UI preview

From [UI-08](plan.md#ui-08-copilot-on-every-workspace-page-entry-launcher-bottom-right):

```text
+---------------------------------------------+------------------------------+
| KAN-419 Retry failed invoices               | Coordinator: Planner    [v] x|
| ...task page...                             | Open the coordinator page    |
|                                             | [This task: KAN-419  x]      |
|                                             |  (Not watched by this        |
|                                             |   coordinator)               |
|                                             | [ Ask something...     ] [>] |
+---------------------------------------------+------------------------------+
Closed: launcher (o) bottom right.
```

Phone: full-screen sheet with Close; the chip sits above the composer.

## Mockup screenshots and scenarios

- [`assets/p2-03-task-page-copilot-context.png`](assets/p2-03-task-page-copilot-context.png)
- [`assets/p2-04-board-copilot-context.png`](assets/p2-04-board-copilot-context.png)
- Scenario `18-v21-copilot-anywhere` (launcher and chip) maps to
  `tests/coordinator/copilot-everywhere.spec.ts`.

## Acceptance

- The launcher shows exactly where `001.1` says, and nowhere with `phase2`
  off.
- Opening the panel on the board closes the task preview, and the reverse.
- A message sent with a chip is stored with the phase-1 prefix.

## Verification

```bash
cd apps/web && pnpm test -- hooks/domains/coordinator
cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check
make -C apps/backend test PKG=./internal/coordinator/...
cd apps/web && pnpm e2e:run tests/coordinator/copilot-everywhere.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/copilot-everywhere.spec.ts
```

Unit: the store's panel rules and the last-used fallback. E2E: open on the
board, check the board chip and that the preview closed; navigate to a task
page and assert the panel stays and the chip changes; remove the chip, send,
and assert the stored message has no prefix; switch coordinators; a reader
sees no launcher; phone opens a sheet.

## Likely files

- `apps/web/app/coordinator/copilot/workspace-copilot-host.tsx`,
  `coordinator-switcher.tsx`, `coordinator-copilot-chip.tsx`
- `apps/web/hooks/domains/coordinator/copilot-store.ts`,
  `use-coordinator-launcher.ts`, `use-page-context-chip.ts`
- `apps/web/components/kanban-with-preview.tsx` (one right panel)
- `apps/web/src/app-shell.tsx`, which mounts the host

## Dependencies

- Task 01 (flag, Watches in the coordinator read).
- Phase-1 task 11, panel swap (`RightSidePanel`, `mobileFullScreen`); see
  [Precondition](#precondition).

## Risks

- The panel and task preview share `RightSidePanel`; both owners write the
  one store field, so the rule cannot be bypassed by a second mount.
