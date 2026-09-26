---
id: "04-needs-you-queue-stalls"
title: "Needs you, Queue, stall records and sidebar"
status: pending
wave: 2
depends_on:
  - "01-shared-interface"
plan: "plan.md"
requirements:
  - REQ-COORDINATOR-COORDINATORS-006
  - REQ-COORDINATOR-NEEDS-YOU-001
  - REQ-COORDINATOR-NEEDS-YOU-002
  - REQ-COORDINATOR-NEEDS-YOU-003
  - REQ-COORDINATOR-NEEDS-YOU-004
  - REQ-COORDINATOR-NEEDS-YOU-005
  - REQ-COORDINATOR-NEEDS-YOU-006
  - REQ-COORDINATOR-NEEDS-YOU-007
  - REQ-COORDINATOR-NEEDS-YOU-008
acceptance_criteria:
  - AC-COORDINATOR-COORDINATORS-006.1
  - AC-COORDINATOR-NEEDS-YOU-001.1
  - AC-COORDINATOR-NEEDS-YOU-001.2
  - AC-COORDINATOR-NEEDS-YOU-001.3
  - AC-COORDINATOR-NEEDS-YOU-001.4
  - AC-COORDINATOR-NEEDS-YOU-001.5
  - AC-COORDINATOR-NEEDS-YOU-001.6
  - AC-COORDINATOR-NEEDS-YOU-002.1
  - AC-COORDINATOR-NEEDS-YOU-002.2
  - AC-COORDINATOR-NEEDS-YOU-002.3
  - AC-COORDINATOR-NEEDS-YOU-002.4
  - AC-COORDINATOR-NEEDS-YOU-002.5
  - AC-COORDINATOR-NEEDS-YOU-002.6
  - AC-COORDINATOR-NEEDS-YOU-002.7
  - AC-COORDINATOR-NEEDS-YOU-002.8
  - AC-COORDINATOR-NEEDS-YOU-002.9
  - AC-COORDINATOR-NEEDS-YOU-003.1
  - AC-COORDINATOR-NEEDS-YOU-003.2
  - AC-COORDINATOR-NEEDS-YOU-003.3
  - AC-COORDINATOR-NEEDS-YOU-004.1
  - AC-COORDINATOR-NEEDS-YOU-004.2
  - AC-COORDINATOR-NEEDS-YOU-004.3
  - AC-COORDINATOR-NEEDS-YOU-004.4
  - AC-COORDINATOR-NEEDS-YOU-004.5
  - AC-COORDINATOR-NEEDS-YOU-005.1
  - AC-COORDINATOR-NEEDS-YOU-005.2
  - AC-COORDINATOR-NEEDS-YOU-005.3
  - AC-COORDINATOR-NEEDS-YOU-005.4
  - AC-COORDINATOR-NEEDS-YOU-005.5
  - AC-COORDINATOR-NEEDS-YOU-006.1
  - AC-COORDINATOR-NEEDS-YOU-006.2
  - AC-COORDINATOR-NEEDS-YOU-006.3
  - AC-COORDINATOR-NEEDS-YOU-006.4
  - AC-COORDINATOR-NEEDS-YOU-006.5
  - AC-COORDINATOR-NEEDS-YOU-007.1
  - AC-COORDINATOR-NEEDS-YOU-007.2
  - AC-COORDINATOR-NEEDS-YOU-007.3
  - AC-COORDINATOR-NEEDS-YOU-007.4
  - AC-COORDINATOR-NEEDS-YOU-007.5
  - AC-COORDINATOR-NEEDS-YOU-008.1
  - AC-COORDINATOR-NEEDS-YOU-008.2
system_design:
  - ../../specs/coordinator/system-design/needs-you.md
  - ../../specs/coordinator/system-design/coordinators.md
  - ../../specs/coordinator/system-design/proposals.md
---

# Task 04: Needs You, Queue, Stall Records and Sidebar (WP-3)

## Summary

Render each coordinator's Needs you and Queue from task facts, stall records
and open proposals, with the count strip, sidebar entries and badge. Proposal
cards render read-only from task 01's proposals route until task 08 adds the
decisions. Nothing on these screens writes. Runs in parallel with tasks 02, 03
and 07.

## In scope

- Backend, in the subscribers registration function of
  `backendapp/coordinator.go`: `task.stalled` subscriber (coordinated
  workspaces only, through task 01's fenced stall upsert, publishing
  `coordinator.updated` on a change), startup pruning, and the
  `workspace.deleted` subscriber. The stalls table and route are task 01's.
- Web: routes and resolver; `lib/coordinator/attention.ts`; `app/coordinator/`
  screens (header, selector, strip, item card with phase 1 actions, Queue
  groups, empty, missing and error states, with the tasks input's failure
  and load time read from `workspaceContextRead` and partial snapshots
  classified); sidebar entries in `app-sidebar-primary-nav.tsx` and
  `MobileRequiredRows`; the badge store and `coordinator.updated` handler;
  six locales. With the flag off no sidebar entry or Coordinator route renders.
- The header's **Configure** action links unconditionally to the settings
  coordinator page at `/settings/workspaces/:id/coordinators/:coordinatorId`
  (declared in the
  [coordinators design](../../specs/coordinator/system-design/coordinators.md#settings-ui),
  built by task 02). A component test asserts the href regardless of whether
  task 02 has merged into this branch; the page itself is task 02's to build.
- Go tests seed proposal rows through task 01's store, since
  `propose_task_kandev` (task 03) may not have landed.
- Whichever of tasks 03, 04 and 07 merges last into task 03's no-turn-start
  table adds the rows for the paths owned by the other two (task 04's stall
  and `workspace.deleted` subscribers here), so the table is complete
  regardless of merge order.
- The `@axe-core/playwright` dependency (new to `apps/web/package.json`) and a
  shared `apps/web/e2e/helpers/axe.ts` wrapping `AxeBuilder`, used by
  `mobile-needs-you.spec.ts` below to satisfy
  `AC-COORDINATOR-NEEDS-YOU-008.2`.

## Out of scope

- The copilot launcher and **Ask about this** behaviour (task 06); the button
  renders and is wired in task 06.
- Approve, Edit and Reject (task 08).
- Resume, merge prompts, answering in place (later phases).
## ASCII UI preview

From [plan UI-01, UI-04 and UI-05](plan.md#ascii-ui-previews):

```text
| Home       | 4 Needs you  | 9 Working | 3 In rev. | 1 Ready to merge|  strip   |
| Inbox    2 +--------------+-----------+-----------+-----------------+          |
| Planner  1 | Needs you                     Coordinator: Planner (or selector)   |
| New Task   | +--------------------------------------------------------------+   |
|            | | KAN-418  Build  [Decide now]  4h 12m        [Ask about this] |   |
|            | | No activity for 4h 12m, and no agent is running             |   |
|            | | Why it is here   ...   What clears it   ...                   |   |
|            | | [Open task] [Show the evidence]                               |   |
```

```text
Nothing needs you                     No coordinator in this workspace yet   Could not load this workspace's tasks.
That is the working state. 9 cards    Questions from your agents still      Showing what was loaded at 08:58.
are running.                          wait on their tasks.                   [Try again]
[See what is running]                 [Add a coordinator]
```

```text
Working (9)
  KAN-430  Build   position underivable: session unreadable
In review (3)
  KAN-402  Review  PR open . 2 unresolved . CI passing
> Done (12)
> Other (4)
```

The phone view is in the plan's UI-01 phone preview.

## Mockup screenshots and scenarios

Screenshots (visual reference; the acceptance criteria govern):

- [`docs/plans/workspace-coordinator/assets/p1-01-needs-you.png`](assets/p1-01-needs-you.png)
- [`docs/plans/workspace-coordinator/assets/p1-03-queue.png`](assets/p1-03-queue.png)

Mockup scenario specs to port (in the workspace-coordinator analysis
mockup's `mockup/e2e/tests/`, outside this repository; see the plan's [Mockup scenario to repo test](plan.md#mockup-scenario-to-repo-test)):

- `01-morning-check.spec.ts`: screens, count strip, item card structure, sidebar entry.
- `06-stalled-child.spec.ts` (evidence): stall card and evidence from a real `task.stalled`.
- `07-empty-is-success.spec.ts`: the empty, missing and error states.
- `14-first-run-setup.spec.ts` (uncoordinated part): no coordinator, Inbox kept, generic entry, no strip.
- `18-v21-copilot-anywhere.spec.ts`, phase 1 list assertions: phase 1 kinds and actions only.

## Acceptance

- Both screens render a real workspace with every classification group, in the
  specified order, and update without reload when task facts change.
- A real `task.stalled` (not seeded) produces a stall card with its evidence in
  a coordinated workspace, and nothing in an uncoordinated one.
- With no coordinator the Inbox row is unchanged, the generic entry has no
  badge and there is no strip; phone and axe checks pass.

## Verification

```bash
cd apps/backend && go test ./internal/coordinator/... ./internal/backendapp/...
cd apps/web && pnpm test -- lib/coordinator/attention.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check
cd apps/web && pnpm e2e:run tests/coordinator/needs-you.spec.ts tests/coordinator/stall.spec.ts tests/coordinator/empty-states.spec.ts
cd apps/web && pnpm e2e:run --project=mobile-chrome tests/coordinator/mobile-needs-you.spec.ts
```

The `mobile-chrome` project matches on the `mobile-*.spec.ts` filename prefix
(`apps/web/e2e/playwright.config.ts`), not on project scope, so the 390px and
axe assertions live in their own `mobile-needs-you.spec.ts` file rather than a
rerun of `needs-you.spec.ts` under a different project. That file also carries
the accessibility scan required by `AC-COORDINATOR-NEEDS-YOU-008.2`, using the
`@axe-core/playwright` `AxeBuilder` helper at `apps/web/e2e/helpers/axe.ts`
(new in this task) and asserting no critical violation on both screens.

The stall spec restarts the e2e backend with
`KANDEV_TASK_STALL_DETECTION_THRESHOLD` set to seconds and a task holding an
execution-less active session, so the one-minute sweep publishes
`task.stalled`. Vitest covers every group, each precedence pair, equal
timestamps, a missing `last_activity_at`, an absent `statusSummary` with and
without a stall row (stall item, and Other with "session unreadable"), an
absent `statusSummary` with `state == "COMPLETED"` (Done, not Other: the
Done rule reads the task's own state, not the status summary), and the
error why-text with an active error, with only a task error that has a
preview ("The task failed"), and with only an empty task error.

Go tests cover the stall upsert: a newer `last_event_at` replaces the row and
publishes `coordinator.updated`; an equal one (redelivery) and an earlier one
(out of order) change nothing and publish nothing, on SQLite and PostgreSQL
(`AC-COORDINATOR-NEEDS-YOU-005.1`).

Go tests cover the `workspace.deleted` subscriber
(`AC-COORDINATOR-COORDINATORS-006.1`): deleting a workspace with a coordinator
seeded with proposals in every status and stall rows removes the coordinator,
its proposals and its stall records in one transaction, and its conversation
tasks are deleted with the workspace's other tasks; a repeated deletion event
for the same workspace changes nothing. This test seeds its own coordinator,
proposal and stall rows through task 01's store rather than depending on task
07's approve flow.

A component test covers the tasks input: with one workflow snapshot loaded and
another failing, the lists and counts show the loaded workflow's tasks and the
banner shows; the load time is absent before the first success; **Try again**
calls `requestWorkspaceContextRefresh` and only the failed workflow is
re-fetched; with no snapshot loaded and a failed read, the banner replaces the
lists and strip.

## Likely files

- `apps/backend/internal/coordinator/{stalls,workspace_deleted}.go` and tests
- `apps/web/src/spa-routes.tsx`
- `apps/web/lib/coordinator/attention.ts` and test
- `apps/web/app/coordinator/`
- `apps/web/components/app-sidebar/app-sidebar-primary-nav.tsx`
- `apps/web/components/navigation/mobile-sidebar-layout-navigation.tsx`
- `apps/web/src/locales/*/`
- `apps/web/e2e/tests/coordinator/`, including `mobile-needs-you.spec.ts`
- `apps/web/e2e/helpers/axe.ts`

## Dependencies

- Task 01 (stalls and proposals tables, routes, `coordinator.updated`,
  `open_proposals`, client). While G0 is open the branch starts from task 01's
  branch and rebases onto main after each predecessor merges.

## Risks

- A stall-driven e2e test is slow; keep it to one spec and avoid sleeps by
  waiting on the rendered card.
- Workflow snapshots must be loaded for every workflow of the workspace, or
  counts undercount.
