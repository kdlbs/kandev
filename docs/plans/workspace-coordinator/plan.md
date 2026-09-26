---
created: 2026-09-26
status: draft
requirements:
  - REQ-COORDINATOR-COORDINATORS-001
  - REQ-COORDINATOR-COORDINATORS-002
  - REQ-COORDINATOR-COORDINATORS-003
  - REQ-COORDINATOR-COORDINATORS-004
  - REQ-COORDINATOR-COORDINATORS-005
  - REQ-COORDINATOR-COORDINATORS-006
  - REQ-COORDINATOR-NEEDS-YOU-001
  - REQ-COORDINATOR-NEEDS-YOU-002
  - REQ-COORDINATOR-NEEDS-YOU-003
  - REQ-COORDINATOR-NEEDS-YOU-004
  - REQ-COORDINATOR-NEEDS-YOU-005
  - REQ-COORDINATOR-NEEDS-YOU-006
  - REQ-COORDINATOR-NEEDS-YOU-007
  - REQ-COORDINATOR-NEEDS-YOU-008
  - REQ-COORDINATOR-COPILOT-001
  - REQ-COORDINATOR-COPILOT-002
  - REQ-COORDINATOR-COPILOT-003
  - REQ-COORDINATOR-COPILOT-004
  - REQ-COORDINATOR-COPILOT-005
  - REQ-COORDINATOR-PROPOSALS-001
  - REQ-COORDINATOR-PROPOSALS-002
  - REQ-COORDINATOR-PROPOSALS-003
  - REQ-COORDINATOR-PROPOSALS-004
  - REQ-COORDINATOR-PROPOSALS-005
system_design:
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/needs-you.md
  - ../../specs/coordinator/system-design/copilot.md
  - ../../specs/coordinator/system-design/proposals.md
legacy_specs: []
---

# Implementation Plan: Workspace Coordinator, Phase 1

## Overview

Phase 1 ships the workspace coordinator's core flow behind
`features.coordinator`: coordinators in workspace settings, a sidebar entry per
coordinator, Needs you and Queue, an attended copilot that reads the workspace,
and task proposals a manager approves, edits or rejects. It is built only on
Kandev main's own building blocks.

The phase map, gates and decisions are in
[ADR-2026-09-26-workspace-coordinator](../../decisions/2026-09-26-workspace-coordinator.md).
This plan covers phase 1 only. Phases 2 to 5 are written to this depth before
their gates open.

## Order

Shared interface first, then parallel work. Task 01 lands the contract
(tables, types, route shapes, constants, event, client); every later work
order builds against it, so they run side by side.

```text
WP-0 --+--> G0 (gates opening upstream PRs, not building)
       +--> task-05 popover shell and chat props --------------------+
       +--> task-01 shared interface --+--> task-02 settings tab      |
                                       +--> task-03 session, surface -+--> task-06 copilot wired --> task-08 proposal UI
                                       +--> task-04 Needs you, Queue -+                                  ^
                                       +--> task-07 approve/reject backend -----------------------------+
```

Critical path: WP-0, task 01, task 03, task 06, task 08. At most three agents
build at once; tasks 03 and 04 go first, as the two largest. Parallel work
orders touch disjoint files except `internal/backendapp/coordinator.go`, where
task 01 gives each later work order its own named registration function
(conversation for task 03, subscribers for task 04, decisions for task 07).

**G0 gates opening upstream PRs, not building.** Building starts when WP-0
has passed Review, not when it merges. Work orders are built and reviewed on
their branches while G0 is open; their upstream PRs open only after G0 is met,
as the ADR's G0 Status section records. An objection at G0 re-plans the
affected work orders.

**Stacking while G0 is open.** WP-0's PR merges only once G0 is met, and CI
requires a changed work order in a code PR, so until then tasks 01 and 05
start from WP-0's branch and task 01's dependents start from task 01's branch.
A work order with several predecessors starts from one of their branches with
the others' reviewed branches merged in, as its Dependencies section says.
When WP-0 merges, each branch rebases onto main, and again after each
predecessor merges.

| Work order | Package | Size | Depends on | Result |
| --- | --- | --- | --- | --- |
| [task-01](task-01-shared-interface.md) | WP-1 | M | WP-0 | Every table, type, route shape, constant and event exists; CRUD, proposals read and stalls read work; flag off 404s |
| [task-02](task-02-settings-tab.md) | WP-1b | M | 01 | Coordinators added, edited and deleted in settings; flag off hides the tab |
| [task-03](task-03-session-tool-surface.md) | WP-2 | L | 01 | A coordinator session reads tasks and writes a proposal; no other write succeeds |
| [task-04](task-04-needs-you-queue-stalls.md) | WP-3 | L | 01 | Needs you and Queue render a real workspace; nothing writes |
| [task-05](task-05-popover-shell.md) | WP-4a | M | WP-0 | Popover shell and chat props exist; Configuration chat and task chat unchanged |
| [task-06](task-06-copilot-wired.md) | WP-4b | M | 03, 04, 05 | A manager asks the coordinator about the workspace from the Coordinator screens |
| [task-07](task-07-proposals-backend.md) | WP-5a | M | 01 | A pending or failed proposal is approved exactly once or rejected through the API; recovery holds |
| [task-08](task-08-proposals-ui.md) | WP-5b | M | 06, 07 | Proposals approved, edited and rejected on both surfaces; phase 1 complete |

Sizes: S under 1 day, M 1 to 3 days, L 3 to 7 days. Each work order is its own
PR, keeps `prod` off, and ships its tests. Every acceptance criterion of the
four requirement documents is owned by exactly one work order's frontmatter;
where a criterion spans surfaces, the owner's body names the work orders that
build the other surfaces.

## Backend

- `internal/coordinator` (task 01): store (`coordinators`,
  `coordinator_proposals`, `coordinator_stalls`) with all store methods,
  repository, service, CRUD, proposals read and stalls read routes, types for
  every route, `coordinator.updated` and its forwarder. Wired in
  `internal/backendapp/coordinator.go`; the store is always initialised and
  everything else is behind the flag.
- Conversation, session and tool surface (task 03): `TaskOriginCoordinator`,
  `SurfaceCoordinator` and `mcpmode.Coordinator` at every resolution site,
  refusal of the origin at the HTTP and MCP create entry points,
  `IsRestorableQuickChatTask`, `autoResumeEligibility`, `message.add`
  requiring `workspace.manage` on a coordinator task,
  `registerCoordinatorTools`, the guard in
  `internal/mcp/handlers/coordinator_authorization.go`, the exact-name
  permission policy and `AutoApprovePermissionsOverride=false`.
- Stall and `workspace.deleted` subscribers and startup pruning (task 04).
- Approve, reject, recovery and the `coordinator-proposal:` external-id prefix
  refusal (task 07).

## Frontend

- Typed client (task 01); settings tab and pages (task 02).
- `lib/coordinator/attention.ts` classification, the Coordinator screens,
  sidebar entries and badge (task 04).
- `ChatPopoverShell` extracted from `ConfigChatPanel`, the optional
  `QuickChatSessionView` props and the transcript tag (task 05);
  `QuickChatSessionKind` stays `"chat" | "config"`.
- The coordinator copilot controller, chip and store (task 06).
- Proposal store and card on both surfaces (task 08).

## ASCII UI previews

Structural choices are requirements; spacing and exact copy are illustrative.
All copy goes through `t()` in six locales. Components are Kandev's existing
primitives.

### UI-01: Needs you (entry: the coordinator's sidebar entry)

Desktop. The count strip is fixed above the scrolling list; the launcher is
fixed bottom right.

```text
+------------+------------------------------------------------------------------+
| Kandev     | Coordinator / Needs you                  [Configure]   Live       |
|            +--------------+-----------+-----------+-----------------+          |
| Home       | 4 Needs you  | 9 Working | 3 In rev. | 1 Ready to merge|  strip   |
| Inbox    2 +--------------+-----------+-----------+-----------------+          |
| Planner  1 | Needs you                     Coordinator: Planner (or selector)   |
| New Task   | +--------------------------------------------------------------+   |
|            | | KAN-418  Build  [Decide now]  4h 12m        [Ask about this] |   |
|            | | No activity for 4h 12m, and no agent is running             |   |
|            | | Why it is here   ...   What clears it   ...                   |   |
|            | | [Open task] [Show the evidence]                               |   |
|            | +--------------------------------------------------------------+   |
|            | | KAN-409  Build  [Review]  12m               [Ask about this] |   |
|            | | Split into a schema change and a migration                   |   |
|            | | Why: touched 31 files across two concerns for three days     |   |
|            | | Proposed by Planner . Policy: Create a card is propose-only |   |
|            | | [Approve] [Edit] [Reject]                                     |   |
|            | +--------------------------------------------------------------+   |
|            |                                                         ( * )  launcher
+------------+------------------------------------------------------------------+
```

Phone at 390px. The strip stays in view; cards stack; actions wrap with 44px
targets; the coordinator entries are in the menu rows.

```text
+--------------------------------+
| [=] Coordinator: Planner   [v] |
| 4 Needs you | 9 Working        |  strip (sticky)
| 3 In review | 1 Ready to merge |
|--------------------------------|
| KAN-418  Build   [Decide now]  |
| 4h 12m                         |
| No activity for 4h 12m ...     |
| Why it is here  ...            |
| What clears it  ...            |
| [Open task]                    |
| [Show the evidence]            |
| [Ask about this]               |
|--------------------------------|
| KAN-409  Build   [Review]      |
| ...                            |
|                          ( * ) |
+--------------------------------+
```

Criteria: `AC-COORDINATOR-NEEDS-YOU-002.*`, `003.*`, `006.*`, `008.1`.

### UI-02: The copilot (Coordinator screens)

420 by 550 pixels, bottom right, no Expand; full width on a phone.

```text
                                     +---------------------------------+
                                     | * Coordinator: Planner      [x] |
                                     |---------------------------------|
                                     | You: split KAN-418 into two     |
                                     | ( ) list_tasks_kandev           |
                                     | Planner: I proposed it.         |
                                     | ! create_task  Pending Approval |
                                     |   [Edit] [Reject] [Approve]     |
                                     |---------------------------------|
                                     | [about KAN-418 x]               |
                                     | Ask the coordinator...     [>]  |
                                     | sent as "About KAN-418: ..."    |
                                     +---------------------------------+
                                                                 ( * )
```

Criteria: `AC-COORDINATOR-COPILOT-004.*`, `005.*`.

### UI-03: Proposal card states (Needs you and chat)

```text
pending, can manage       approving                 failed
! create_task             ! create_task             ! Pending Approval
  Pending Approval          Approval in progress      Could not create the task:
  <title, workflow, step>   Edits are locked.         <error>. Nothing was created.
  [Approve] [Edit] [Reject]                           [Approve] [Edit] [Reject]
approved (chat)           rejected (chat)
v Approved: KAN-432       x Rejected: <reason>
```

Edit opens title, description, workflow, step (eligible steps only) and
repository in place, with **Approve with edits** and **Cancel**. Reject opens
an optional reason with **Confirm reject** and **Cancel**. Criteria:
`AC-COORDINATOR-PROPOSALS-005.*`.

### UI-04: Empty and error states

```text
Nothing needs you                     No coordinator in this workspace yet   Could not load this workspace's tasks.
That is the working state. 9 cards    Questions from your agents still      Showing what was loaded at 08:58.
are running.                          wait on their tasks.                   [Try again]
[See what is running]                 [Add a coordinator]
```

Criteria: `AC-COORDINATOR-NEEDS-YOU-007.*`.

### UI-05: Queue

```text
Coordinator / Queue                       [Configure]
[strip as UI-01]
Working (9)
  KAN-421  Build   agent running   2m ago
  KAN-430  Build   position underivable: session unreadable
In review (3)
  KAN-402  Review  PR open . 2 unresolved . CI passing
Ready to merge (1)
  KAN-399  Review  PR ready . 0 unresolved . CI passing
> Done (12)
> Other (4)
```

Rows open the task and have no other action. Criteria:
`AC-COORDINATOR-NEEDS-YOU-004.*`.

### UI-06: The Coordinators settings tab

```text
home > Settings > Workspaces > Software Factory > Coordinators
Overview Repositories Workflows Canvases Integrations Automations Secrets [Coordinators]
(o) Coordinators                                            [+ Add coordinator]
Coordinators read this workspace's boards, tell you what needs you and why, and propose work you approve.
+--------------------------------------+
| Planner                              |
| Agent profile: Claude . worktree     |
| Relay ships consent features; ...    |
| [Open]  Configure                    |
+--------------------------------------+
Configure / + Add coordinator -> the coordinator's page:
  < All coordinators
  Name [____]  Agent profile [v]  Executor [v]  Context [____]
  Note: the profile's auto-approve is ignored for coordinators.
  new: [Add coordinator]     existing: settings save bar (Discard, Save) + [Delete coordinator]
```

Criteria: `AC-COORDINATOR-COORDINATORS-004.*`, `005.1`, `005.2`, `005.3`.

## Mockup screenshots

The phase 1 screenshots from the analysis mockup (`shots-v2.1`) are carried
into [`assets/`](assets/). They are the visual reference; the acceptance
criteria govern, and the prototype banner, demo controls and `P1`/`WC-`
labels are mockup chrome.

| Screenshot | Shows | Cited by | Work orders |
| --- | --- | --- | --- |
| [`assets/p1-01-needs-you.png`](assets/p1-01-needs-you.png) | Needs you: strip, question, stall and error items, sidebar badge | needs-you, proposals, ADR | 04, 08 |
| [`assets/p1-02-ask-about-this.png`](assets/p1-02-ask-about-this.png) | copilot popover with the Ask about this chip and draft | copilot | 05, 06 |
| [`assets/p1-03-queue.png`](assets/p1-03-queue.png) | Queue groups and rows | needs-you | 04 |
| [`assets/p1-04-settings-coordinators-list.png`](assets/p1-04-settings-coordinators-list.png) | Coordinators settings tab, list | coordinators | 02 |
| [`assets/p1-05-chat-create-task-proposal.png`](assets/p1-05-chat-create-task-proposal.png) | a proposal card pending approval in the chat | copilot, proposals | 03, 06, 08 |
| [`assets/p1-06-settings-coordinator-editor.png`](assets/p1-06-settings-coordinator-editor.png) | coordinator editor: name, profile, executor, context | coordinators | 02 |

## Mockup scenario to repo test

| Mockup spec (phase 1 view) | Repo test | Work order |
| --- | --- | --- |
| `18-v21` settings test | settings: add, edit, delete, permissions | 02 |
| `01-morning-check` | screens, strip, item card structure, sidebar entry | 04 |
| `06-stalled-child` (evidence) | stall card and evidence from a real `task.stalled` | 04 |
| `07-empty-is-success` | UI-04's three states | 04 |
| `14-first-run-setup` (uncoordinated part) | no coordinator: Inbox kept, generic entry, no strip | 04 |
| `18-v21` phase 1 list assertions | phase 1 kinds and actions only | 04 |
| `17-ask-the-copilot`, `18-v21` Ask about this | copilot, draft, answer | 06 |
| `05-rule-on-a-proposal`, `18-v21` "one write class" | approve, edit, reject on both surfaces | 08 |

Repo tests drive the mock agent with `e2e:` scripts and assert product
surfaces, not agent wording.

## Verification strategy

- Go tests beside the code in `internal/coordinator`, `internal/mcp`,
  `internal/agentctl`, `internal/task` and `internal/orchestrator`; store and
  upgrade conformance on SQLite and PostgreSQL.
- Vitest for `attention.ts`, the API client, the settings tab gating, the
  proposal store and the transcript renderer.
- Playwright in `apps/web/e2e/tests/coordinator/`, in the default project, the
  `auth` project for reader cases and `mobile-chrome` for 390px checks, with
  axe scans on the Coordinator screens.
- Every work order also runs `cd apps/web && pnpm run i18n:check` when it adds
  copy.
- Public docs `docs/public/coordinator.md` through `/docs-maintainer` land with
  task 08, including what the coordinator's agent can still do through its own
  tools.

## Risks

| Risk | Mitigation |
| --- | --- |
| A maintainer prefers the plugin placement of ADR-2026-08-31 | G0 before any upstream PR opens; the ADR supersedes one clause openly and keeps the Host half; an objection re-plans the affected work orders |
| A tool slips past the guard | Task 03's table test walks every registered Kandev MCP action for a coordinator principal |
| The agent reaches past its Kandev surface through its own tools | Phase 1 is attended; the ADR records the residual; containment is a G4 condition |
| A `coordinator` surface, mode or origin name is taken on main when task 03 starts | Task 03 checks first and renames if taken (D7) |
| Open PRs touching `internal/mcp` land first | Whichever lands second rebases; no dependency either way |

## Definition of done (phase 1)

The flag is on in e2e; the tests above pass; task 03's tests prove the
coordinator's Kandev surface can only read and propose, that Kandev
auto-approves nothing else for it, and that nothing but a manager's message
starts a turn; public docs are updated.
