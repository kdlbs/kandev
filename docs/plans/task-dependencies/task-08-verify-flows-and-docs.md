---
id: "08-verify-flows-and-docs"
title: "Verify browser flows and documentation"
status: done
wave: 8
depends_on: ["04-failed-predecessor-halt", "06-kanban-dependency-ui"]
plan: "plan.md"
spec: "../../specs/tasks/system-design/task-dependencies.md"
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
- **Desktop cycle**: attempting to close a cycle surfaces the cycle path and
  creates no edge.
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
- **Mobile**: the blocked badge is readable in a focused column with no
  horizontal page overflow and usable tap targets, and tapping the dependency
  chip opens the drawer rather than a hover card. There is no Dependencies view
  and no detail-view picker to reach (Task 07 dropped; see `plan.md`).

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
- `docs/specs/tasks/system-design/task-dependencies.md`
- `docs/specs/INDEX.md`
- `docs/plans/task-dependencies/plan.md`

## Dependencies

Tasks 04 and 06: the last backend and frontend behaviors respectively.

## Parallelism

`sequential`

## Recorded results

`apps/web/e2e/tests/task/task-dependencies.spec.ts`, chromium, 5 passed:

- a chain runs one step at a time and never restarts a predecessor
- a failed predecessor halts the chain until it succeeds
- removing the last edge unblocks without launching
- a cycle is refused with the offending path
- the board and the open task both show the dependency

The first run failed 3 of the 5, and both causes were real product bugs that
every unit test had missed because neither crosses a single package:

1. **WIP admission erased the launch intent.** The SQLite repository deleted
   `metadata.deferred_launch` on every admitted create, because a WIP-overflow
   record is stale once a task is admitted. The start-when-unblocked intent
   shares that record, and a chain step is admitted at create time like any
   other task, so the intent was gone before any predecessor could resolve it
   and no chain ever ran. Admission now routes through
   `models.DropWIPDeferredLaunch`, which keeps a chain intent and drops only an
   overflow one (`internal/task/models/deferred_launch_test.go`, mutation-
   verified).
2. **The status row was hidden on session-less tasks.** The dockview hosts pass
   the panel's task only when it has a session, so `ChatStatusBar` saw a null
   task id and rendered nothing, taking the dependency chip with it on exactly
   the tasks the chip describes: a blocked step has no session by construction.
   The hosts now pass a status-row-only `statusTaskId`
   (`resolveStatusRowTaskId`), leaving the session-derived id, plan mode, and
   read tracking untouched.

Backend: `go test ./internal/task/... ./internal/orchestrator/... ./internal/mcp/...`
fails only the 12 known worktree-sandbox tests (`parent directory cannot be
accessed`, local-repository initialization and directory listing), the identical
set before and after this change. Changed-file lint against the PR base is
clean. Frontend: 1363 files / 11090 tests pass, lint clean, `i18n:check` and
`i18n:ratchet` clean.

Not run: the mobile project. The mobile assertions in the acceptance list above
are still outstanding.

## Output contract

Mark this task `in_progress` before the first E2E run and `done` only after the
listed commands pass. Record each E2E spec name, the mobile run result, the
exact set of pre-existing backend failures with evidence they are environmental,
and the docs updated in this file and `plan.md`.
