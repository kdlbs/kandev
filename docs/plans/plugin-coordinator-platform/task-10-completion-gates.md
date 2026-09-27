---
id: "10-completion-gates"
title: "Native completion criteria and evidence gates"
status: complete
wave: 10
depends_on: ["01-exact-host-foundation", "06-workspace-observations", "07-task-commands", "09-task-claims"]
plan: "plan.md"
requirements:
  - REQ-TASKS-COORDINATION-002
acceptance_criteria:
  - AC-TASKS-COORDINATION-002.1
  - AC-TASKS-COORDINATION-002.2
  - AC-TASKS-COORDINATION-002.3
system_design:
  - ../../specs/tasks/system-design/coordination-controls.md
---

# Task 10: Native completion criteria and evidence gates

## Summary

Add optional task criteria and evidence that every completion path enforces. Preserve completion behavior for tasks without criteria.

## In scope

- Add revisioned criteria/evidence/history and exact set/verify commands. Bind verification to typed subject revisions and require native human confirmation to remove or weaken unmet criteria.
- Integrate the shared completion guard into manual, bulk, queued, workflow signal, agent, and automation moves; recheck at commit.
- Add native evidence/blocker inspection and one-action human override with reason. Cover reopen, stale PR head, criteria edit races, and absent plugin.

## Out of scope

- Work owned by other orders, publishing external repositories, and unapproved production rollout.
- Coordinator-specific host roles, private API shortcuts, and changes to unrelated v1 behavior.

## Acceptance

- All completion paths reject unmet or stale criteria and tasks without criteria remain compatible.
- Concurrent edits invalidate affected evidence and a queued completion rechecks current criteria.
- Desktop and phone show blockers and allow a reasoned human override bound to one move.

## ASCII UI preview

See the [combined preview](plan.md#ascii-ui-preview). These excerpts keep its stable labels.

### UI-03: Task management and completion

```text
Desktop: task detail > management and completion
+----------------------------------------------------------------+
| Manager: Delivery lead                       [Transfer] [Release]|
| Completion: 1 of 2 verified                                      |
| [ok] Regression test passes     [Evidence]                        |
| [!] Review resolved             [Inspect blocker]                |
| Cannot complete: current review evidence is missing               |
| [Record human override...]                      [Complete: off]   |
+----------------------------------------------------------------+
Phone: task detail > completion
+------------------------------+
| Manager: Delivery lead       |
| [Transfer] [Release]         |
| Completion: 1 of 2           |
| [ok] Regression test passes  |
| [Evidence]                  |
| [!] Review unresolved       |
| [Inspect blocker]           |
| [Record override...]        |
+------------------------------+
```

Show claims independently from the worker assignee. Evidence/transfer opens a drawer on phone and a dialog on desktop. An override requires a reason and targets one observed move. Stale evidence and an unavailable manager remain visible; no hidden automatic takeover.

Applicable criteria: `AC-TASKS-COORDINATION-002.1`, `AC-TASKS-COORDINATION-002.2`, `AC-TASKS-COORDINATION-002.3`.

## Verification

Add the named tests before implementation using TDD. These commands run after the work order is implemented; they are not results from planning.

```bash
(cd apps/backend && go test -race ./internal/task/service ./internal/workflow/... -run 'TestCompletionGate|TestCompletionEvidence' -count=1)
(cd apps/web && pnpm run typecheck && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/plugins/task-completion-evidence.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/plugins/mobile-task-completion-evidence.spec.ts)
```

Evidence to create:

- `apps/backend/internal/task/service/completion_gates_test.go`: `TestCompletionGateEntryPoints`.
- `apps/web/e2e/tests/plugins/task-completion-evidence.spec.ts`: `stale evidence, blocked move and override`.
- `apps/web/e2e/tests/plugins/mobile-task-completion-evidence.spec.ts`: `phone inspect and override`.

Follow the [plan verification rules](plan.md#tests), including expected test presence, generated contracts, and public documentation checks.

## Files likely touched

- `apps/backend/internal/task/service/`
- `apps/backend/internal/task/repository/`
- `apps/backend/internal/workflow/`
- `apps/backend/internal/task/statussummary/`
- `apps/backend/internal/plugins/host_write.go`
- `apps/web/components/task/`
- `apps/web/src/locales/`

New test paths above are deliverables. Existing directories indicate the owning boundary; inspect scoped AGENTS.md before editing.

## Dependencies

- [Task 01](task-01-exact-host-foundation.md)
- [Task 06](task-06-workspace-observations.md)
- [Task 07](task-07-task-commands.md)
- [Task 09](task-09-task-claims.md)

## Risks

A UI-only gate is bypassable. Locate and unify all final transition commits before shipping the feature.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/coordination-controls.md) and [design](../../specs/tasks/system-design/coordination-controls.md).
- [Ownership decision](../../decisions/2026-09-25-plugin-coordination-platform.md).
- [Source baseline and fork mapping](plan.md#source-baseline-and-fork-mapping).

## Results

Implemented revisioned task completion criteria, typed evidence with task-revision freshness, audit history, exact Host set/verify calls, and a shared final transition guard. Tasks without criteria retain the existing completion behavior. Manual and deferred workflow moves use the same repository guard; stale evidence and concurrent criteria changes are rechecked at commit. The native task detail view shows evidence status and blockers, and supports a reasoned one-move human override on desktop and phone. Workflow snapshot projection now preserves the terminal-step completion flag required by the override selector.

Validation passed:

- `go test -race ./internal/task/service ./internal/workflow/... -run 'TestCompletionGate|TestCompletionEvidence' -count=1`
- `go test ./internal/plugins ./pkg/pluginsdk ./internal/backendapp -run 'Test.*CompletionGate|TestExactTaskCompletion' -count=1`
- `make -C apps/backend proto` and `make -C apps/backend build`
- Web completion-gate API and workflow-snapshot tests; `pnpm run typecheck`; scoped ESLint; `pnpm run i18n:check`
- Desktop completion-evidence E2E and phone completion-evidence E2E, one test each
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check`
