---
id: "02-browser-proof-and-docs"
title: "Prove desktop and phone order"
status: completed
wave: 2
depends_on:
  - "01-task-wide-running-rank"
plan: "plan.md"
requirements:
  - REQ-UI-SIDEBAR-RUNNING-ACTIVITY-001
acceptance_criteria:
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.2
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.6
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.7
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.8
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.14
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.15
  - AC-UI-SIDEBAR-RUNNING-ACTIVITY-001.16
system_design:
  - ../../specs/ui/system-design/sidebar-running-first-activity-sort.md
---

# Task 02: Prove desktop and phone order

## Summary

Extend the existing sort-chain browser scenarios to cover running secondary
sessions. Update the public Running reference to describe task-wide state.
Keep the existing editor, persistence, grouping, and navigation scenarios.

## In scope

- Seed a running task with no primary and an idle orange task.
  Use the user's chain: Running, Red, Orange, Last activity, all descending.
- Add a waiting-primary/running-secondary scenario and assert the real row order.
  Let the idle peer win color/activity comparisons so the runtime assertion matters.
- Prove initial load, reload, secondary start, and final running-session stop.
  Keep the active task ID, session, URL, and conversation stable during reordering.
- On phone, open the real task picker with touch and scope assertions to its drawer.
  Use the configured `mobile-chrome` device and the shared predicate.
- Reuse API seeding, page objects, and causal waits. Assert session state and
  primary flags as fixture preconditions before DOM order.
  Restore saved views and color patches in `finally`.
- Update the Running table row in `docs/public/tasks-and-workflows.md`.
  This reference edit explains any running session and included descendant promotion.
  Keep internal implementation out of the product wording.
- Record this package's final counts. Preserve the original package's historical
  results. Promote the paired draft requirement/design after both work orders pass.

## Out of scope

No UI redesign, new labels, screenshots, primary election, or live-instance repair.
No broad review or full E2E suite. Task 01 owns production ranking changes.

## Acceptance

1. Desktop and phone put the secondary-running task above the idle preferred-color
   peer on initial load, after reload, and after an accepted runtime change.
2. A final running-session stop restores idle ordering through existing refresh.
   The current conversation remains selected throughout both flows.
3. The public reference matches the amended contract. Exact final checks and
   work-order/specification statuses describe the completed result.

## ASCII UI preview

UI-01 excerpt from [the complete preview](plan.md#ascii-ui-preview):

```text
Desktop Tasks
  [running] Projects
  [idle]    Resume (orange)
```

UI-02 excerpt:

```text
Task title [v]
  +-----------------------------------+
  | Tasks                  All tasks v | fixed
  | [running] Projects                | scroll
  | [idle]    Resume (orange)          |
  +-----------------------------------+
```

The existing phone drawer supplies header, safe areas, and scrolling.
Only task order changes. Labels illustrate existing status icons.
AC .2, .6-.8, .14-.16 apply. These specs supply rendered proof.

## Verification

Run these commands sequentially from the repository root.
Each managed browser command rebuilds the backend and frontend first.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/sidebar-running-first-activity-sort.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-sidebar-running-first-activity-sort.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Record each project's discovered and passed counts. Do not use `--no-build`
for final proof. If helpers need new test-only seeding, cover their server handler.
Add its exact targeted Go command before implementation.
Use causal event/HTTP waits for transitions instead of arbitrary sleeps.

## Files likely touched

- `apps/web/e2e/tests/task/sidebar-running-first-activity-sort.spec.ts`
- `apps/web/e2e/tests/task/mobile-sidebar-running-first-activity-sort.spec.ts`
- `apps/web/e2e/tests/task/sidebar-running-first-activity-sort-helpers.ts`
- `docs/public/tasks-and-workflows.md`
- This plan and both work-order Results/status sections
- `docs/specs/ui/requirements/sidebar-running-first-activity-sort.md` lifecycle status
- `docs/specs/ui/system-design/sidebar-running-first-activity-sort.md` lifecycle status

## Dependencies

Task 01 must pass its targeted checks first.
Browser proof tests the integrated rank without changing production logic here.

## Risks

The fixture must retain its absent/idle primary instead of auto-electing the
running session. Fabricated spinner data can mask an incorrect runtime projection.
Worker-scoped views and colors require cleanup on failed assertions too.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/ui/requirements/sidebar-running-first-activity-sort.md)
- [Design](../../specs/ui/system-design/sidebar-running-first-activity-sort.md#task-wide-running-projection)
- [Task 01](task-01-task-wide-running-rank.md)
- Existing desktop/mobile sort-chain specs, shared helper, and `SessionPage`
- `.agents/skills/e2e/SKILL.md` and its UI state/cleanup reference
- `.agents/skills/mobile-parity/SKILL.md`
- `.agents/skills/docs-maintainer/SKILL.md`

## Results

Completed. The managed `chromium` project discovered and passed 2 tests; the
managed `mobile-chrome` project discovered and passed 2 tests. Both commands
rebuilt the Go backend and production Vite assets before running. The new
desktop and phone scenarios used the exact Running, Red, Orange, Last activity
descending chain and causal task-summary revision waits. Both surfaces proved
the no-primary running task on initial load and after reload, the waiting
primary/running secondary moving ahead after start, and idle ordering returning
after the final running secondary became waiting. The task URL, active session,
and conversation remained unchanged. Existing editor, pagination, grouping,
and navigation scenarios also passed.

Public-doc tests passed (62), all 47 published pages validated, the specification
catalog validated (361 decisions and 1425 specifications), all 36 spec-linter
tests passed, full spec lint passed, and `git diff --check` passed. The paired
requirement is now active and the system design is current. PostgreSQL evidence is
recorded in Task 01.
