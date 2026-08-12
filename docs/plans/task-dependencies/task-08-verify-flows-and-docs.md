---
id: "08-verify-flows-and-docs"
title: "Verify browser flows and documentation"
status: not_started
wave: 8
depends_on: ["04-failed-predecessor-halt", "06-kanban-dependency-ui"]
plan: "plan.md"
spec: "../../specs/task-dependencies/spec.md"
---

# Task 08: Verify Browser Flows and Documentation

## Acceptance

Browser E2E, covering the spec's scenarios end to end rather than in units:

- **Desktop chain**: seed A → B → C with start-when-unblocked intents; complete
  A and assert B starts and C does not; complete B and assert C starts and A is
  not restarted.
- **Desktop gate**: a task with a pending predecessor moved into an auto-start
  step gains no session.
- **Desktop WIP interaction**: a dependent queued for a full WIP step gains no
  session when its dependency resolves, and starts exactly once after capacity
  opens.
- **Desktop failure**: fail A and assert B stays blocked with the failed reason,
  gains no session, and shows the notification; retry A to success and assert B
  starts exactly once.
- **Desktop cycle**: attempting to close a cycle from the picker surfaces the
  cycle path and creates no edge.
- **Desktop chip**: open task B where B depends on A and D depends on B; assert
  the dependency chip appears in the status row above the composer, lists A under
  blocked-by and D under blocks with their states, and navigates to A on click.
  Assert the chip is absent on a task with no edges.
- **Desktop chip with no PR**: a blocked task with no PR, no todos, and no queued
  prompts still renders the status row with the chip.
- **MCP chain**: drive `create_task_kandev` three times with `blocked_by` and the
  default `start_agent`, assert no session starts on creation, then complete the
  first and assert the second starts and the third does not. Then exercise
  `add_task_dependency_kandev` / `remove_task_dependency_kandev` against a live
  task and assert the board reflects both.
- **Desktop Dependencies view**: create an edge by dragging, see the connector
  and the blocked badge appear, then delete the connector and see the task
  unblock.
- **Mobile**: the blocked badge is readable in a focused column with no
  horizontal page overflow and usable tap targets; the "Depends on" picker is
  reachable; tapping the dependency chip opens the drawer (not a hover card); the
  Dependencies view is not offered.

Seed tasks **without** an agent and locate them by task id. `createTaskWithAgent`
on a start step moves the card mid-test and races column assertions.

Documentation:

- `docs/public/tasks-and-workflows.md` documents declaring a dependency, what
  counts as resolution (successful completion or a final Done/Complete/
  Completed/Approved step), that a failed or cancelled predecessor halts the
  chain and needs human action, that dependencies gate automated starts but not
  manual ones, and that resolution does not bypass WIP limits.
- `docs/public/workflow-tips.md` gains a worked A → B → C example if the
  auto-start interaction needs one.
- `docs/features.md` lists task dependencies in the live feature inventory.
- The spec's `status` moves to `shipped` and its Open questions section is
  empty or deleted.
- `docs/specs/INDEX.md` and this plan's statuses are accurate.
- `apps/web/AGENTS.md` is untouched: Task 07's board view was dropped, so no
  view-registry convention changed.

## TDD sequence

1. Write each E2E spec red against the implemented backend/frontend, one flow at
   a time, and fix what they catch.
2. Run the mobile project explicitly; do not infer mobile behavior from the
   desktop run.
3. Update public docs and the spec/plan statuses last, once behavior is settled.

## Verification

```bash
cd apps/web
pnpm e2e:raw --grep 'dependenc'
pnpm e2e:raw --project=mobile-chrome --grep 'dependenc'
cd ../backend && go test -tags fts5 ./... -count=1
```

A full backend suite in a task worktree fails roughly four packages with
`parent directory cannot be accessed`; that is the sandbox, not this change.
Confirm those are the only failures and that they reproduce on a clean checkout
of `main`.

## Files likely touched

- `apps/web/e2e/tests/task/task-dependencies.spec.ts` (new)
- `apps/web/e2e/helpers/` (seeding helper for edges/intents)
- `docs/public/tasks-and-workflows.md`
- `docs/public/workflow-tips.md`
- `docs/features.md`
- `docs/specs/task-dependencies/spec.md`
- `docs/specs/INDEX.md`
- `docs/plans/task-dependencies/plan.md`

## Dependencies

Tasks 04 and 07 — the last backend and frontend behaviors respectively.

## Parallelism

`sequential`

## Output contract

Mark this task `in_progress` before the first E2E run and `done` only after the
listed commands pass. Record each E2E spec name, the mobile run result, the
exact set of pre-existing backend failures with evidence they are environmental,
and the docs updated in this file and `plan.md`.
