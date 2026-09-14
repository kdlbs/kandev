---
id: "03-regressions-and-docs"
title: "Verify scope and document recovery"
status: complete
wave: 3
depends_on:
  - 02-shared-task-errors
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006
  - REQ-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002
acceptance_criteria:
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.5
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.6
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.7
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.8
  - AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-006.9
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.1
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.2
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.3
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.4
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.5
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.6
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.7
  - AC-TASKS-TASK-LAUNCH-FAILURE-RECOVERY-002.8
system_design:
  - ../../specs/agents/system-design/session-recovery-failures.md
  - ../../specs/tasks/system-design/task-launch-failure-recovery.md
---

# Task 03: Verify scope and document recovery

## Summary

Exercise the combined experience and update public recovery guidance after implementation.

## In scope

- Extend the existing desktop/phone suites with retained session history and shared-task error scenarios.
- Named scenarios: `retains session failure after automatic recovery and output`, `preserves reading anchor while older history loads`, `keeps shared failure across session and Plan tabs`, `does not clear shared failure on sibling recovery`, and `legacy failure remains readable`.
- Include a manual retry, another failure stamp, delayed old event, reload with recovery boot outside the loaded page, duplicate delivery, and no-session shared error.
- Assert session error chronology and one action owner. Test short viewport, phone touch size, drawer bounds, focus return, safe areas, and no document overflow.
- Exercise the preview and Quick Chat hosts in the same focused suites using their existing navigation fixtures.
- Update `docs/public/tasks-and-workflows.md` as recovery how-to guidance. Explain retained session history and the task-wide strip.
- Reconcile affected older tests and completed-plan successor notes without rewriting historical test results.

## Out of scope

Broad QA/review, production data mutation, and new provider diagnostics.

## Acceptance

1. Combined desktop and phone scenarios pass against a fresh build, including mixed-scope errors and resumed agent output.
2. Rendered structure matches UI-01 through UI-04. Historical controls cannot affect a newer failure.
3. Public guidance describes shipped behavior. Plan and work-order results record actual commands and outcomes.

## ASCII UI preview

UI-01: Desktop task, shared failure plus an independently recovered session error.

```text
+--------------------------------------------------------+
| Task title / workflow                                  | fixed
| ! Workspace preparation failed. [Recovery details]      | shared
+--------------------------------------------------------+
| Session A | Session B | Plan | Pull request | Files      | tabs
+--------------------------------------------------------+
| Earlier conversation                                 ^ |
| ! Session resume failed. 10:18                        | |
|   Loading timed out. [Recovery details]                | | scrolls
|   Recovered.                                          | |
| Agent: I resumed work...                              v |
+--------------------------------------------------------+
| Message input                                          | fixed
+--------------------------------------------------------+
```

UI-02: Phone task Chat, same two independent failures.

```text
+--------------------------------+
| < Task title       Session A v | fixed task chrome
| ! Workspace preparation failed |
| [Recovery details]             | shared on every view
+--------------------------------+
| Earlier messages             ^ |
| ! Session resume failed      | |
| Loading timed out.           | | transcript scrolls
| Recovered. [Details]         | |
| Agent: I resumed work...     v |
+--------------------------------+
| Message input                  | safe-area clearance
| Chat | Plan | Files | More     | existing navigation
+--------------------------------+
```

UI-03: Session entry states, inside either transcript.

```text
Unresolved: ! Resume failed. [Resume] [Recovery details]
Pending:    ! Resume failed. Resuming... [Details]
Recovered:  ! Resume failed. Recovered. [Details]
Older:      ! Resume failed. [Details]   (no stale actions)
```

Expanded session details wrap inline. Phone actions stack with 44-pixel targets.
A failed new attempt appends a separate error entry. Same-stamp updates retain position.

UI-04: Shared details, desktop dialog / phone inset bottom drawer.

```text
+--------------------------------+
| Workspace preparation       X  |
| Affected repository: owner/repo|
| Safe cause and bounded details |
| [Valid recovery action]        |
+--------------------------------+
```

The phone drawer has safe-area clearance, an internally scrolling body, and 44-pixel controls.
The desktop dialog uses compact controls. Closing details does not clear the shared alert.
Views map to recovery criteria 006.4/6/7/8/9 and task criteria 002.4/5/6/7.
Fixed versus scrolling regions, scope, order, and retained history are requirements.
Copy and spacing are illustrative and must use localized strings and existing tokens.
The two errors coexist only because they represent different failures.

See the [combined plan](plan.md#ascii-ui-preview). Task 03 implements the views within its scope.

## Verification

Run from the repository root. Install workspace dependencies once before the first package command if this checkout lacks them.
Use `/tdd` for changed logic and `/e2e` for browser work. First establish the regression against current behavior.

```bash
(cd apps/web && pnpm e2e:run --project chromium tests/task/launch-failure-recovery.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`.
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts` and existing fixture/page-object helpers if necessary.
- `docs/public/tasks-and-workflows.md`.
- This package and the linked owning specs, after implementation matches the design.

## Dependencies

02-shared-task-errors.

## Inputs

Read both owning specs linked from the plan and the September 14 scope decision.
Use the existing launch-recovery tests as the fixture pattern.

## Risks

Preserve failure identity, sanitized details, existing recovery guards, and message ordering under reversed delivery.
The shared repository can contain other edits. Do not revert unrelated changes.

## Parallelism

`sequential`

## Results

Complete on September 14, 2026. The desktop and phone fixtures cover retained session history, ordinary scroll anchoring, shared task errors across session and Plan surfaces, sibling recovery, legacy readability, duplicate stamps, reload, delayed output, no-session recovery, phone drawer bounds, touch targets, and document overflow. Backend and composed frontend regressions also cover the real bootstrap and Office producers, persisted/provisional representation deduplication, cold-projector task scope, and stale snapshot retirement. Public session and task recovery guidance now matches the shipped scope and history behavior.

Validation passed:

- `pnpm e2e:run --host --no-build --project chromium e2e/tests/task/launch-failure-recovery.spec.ts`: 4 passed.
- `pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-launch-failure-recovery.spec.ts`: 3 passed.
- `node --test scripts/validate-public-docs.test.mjs` and `node scripts/validate-public-docs.mjs`: passed.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed.
- Review regression suites and the complete selected backend package run: passed.
