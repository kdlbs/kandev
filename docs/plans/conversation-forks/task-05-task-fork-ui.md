---
id: "05-task-fork-ui"
title: "Deliver task and child-task forks"
status: done
wave: 5
depends_on: ['04-agent-fork-ui']
plan: "plan.md"
requirements:
  - REQ-TASKS-CONVERSATION-FORK-001
  - REQ-TASKS-CONVERSATION-FORK-003
  - REQ-TASKS-CONVERSATION-FORK-004
  - REQ-TASKS-CONVERSATION-FORK-005
  - REQ-TASKS-CONVERSATION-FORK-006
acceptance_criteria:
  - AC-TASKS-CONVERSATION-FORK-001.1
  - AC-TASKS-CONVERSATION-FORK-001.3
  - AC-TASKS-CONVERSATION-FORK-003.1
  - AC-TASKS-CONVERSATION-FORK-003.2
  - AC-TASKS-CONVERSATION-FORK-003.6
  - AC-TASKS-CONVERSATION-FORK-004.1
  - AC-TASKS-CONVERSATION-FORK-004.2
  - AC-TASKS-CONVERSATION-FORK-004.3
  - AC-TASKS-CONVERSATION-FORK-004.4
  - AC-TASKS-CONVERSATION-FORK-004.5
  - AC-TASKS-CONVERSATION-FORK-004.6
  - AC-TASKS-CONVERSATION-FORK-004.7
  - AC-TASKS-CONVERSATION-FORK-004.8
  - AC-TASKS-CONVERSATION-FORK-005.2
  - AC-TASKS-CONVERSATION-FORK-005.4
  - AC-TASKS-CONVERSATION-FORK-006.1
  - AC-TASKS-CONVERSATION-FORK-006.2
  - AC-TASKS-CONVERSATION-FORK-006.3
  - AC-TASKS-CONVERSATION-FORK-006.4
  - AC-TASKS-CONVERSATION-FORK-006.5
system_design:
  - ../../specs/tasks/system-design/conversation-forks.md
---

# Task 05: Deliver task and child-task forks

## Summary

Complete the new-task and child-task destinations using the shared fork state. Add user guidance for conversation context, size estimates, and workspace behavior.

## In scope

- Enforce AC-004.8: agents share the source execution workspace, new tasks use a separate workspace, and child tasks offer both modes.

- Own TaskCreateDialog and NewSubtaskDialog integration with the shared snapshot hook and all destination picker choices.
- Retain existing repository/base defaults for separate workspaces, with no fork-specific source-commit default.
- Preserve selected source session, repository settings, explicit parent, workflow, executor, and model choices.
- Carry the chip through create-without-start, later start, errors, and reload. Preserve the same submission ID for network retries.
- Add desktop/mobile task and child-task E2E coverage, including shared versus new workspace choices without historical file restoration.
- Update public task/workflow guidance and reconcile agent docs, root README, and screenshot descriptions if affected.
- Reconcile package statuses with actual implementation and exact test results after all work orders pass.

## Out of scope

New child-task workspace modes, changing repository defaults, cross-workspace transfer, and additional fork scope.

## Acceptance

- All three picker choices work. Task and child-task forms retain their normal defaults and submit the same frozen context reference.
- Create-without-start survives reload, later launch uses the accepted snapshot, and duplicate network submission creates only one destination.
- Desktop and phone E2E prove parent identity, selected workspace behavior, context preview, and recovery. Public docs describe only verified behavior.

## ASCII UI preview

Use [the combined preview](plan.md#ascii-ui-preview) for UI-01 through UI-04.
The excerpt preserves the same structural contract and the acceptance references in this work order.

```text
UI-02 desktop
[Conversation: messages | estimated tokens | Preview | x]
[New instruction                                      ]
[Profile] [Model] [Executor]             [Primary action]

UI-02 phone, full-height surface
[Back] Destination                         fixed
[Conversation | View | x]
[New instruction]                          scroll body
[Profile >] [Model >] [Executor >]
[Primary action]                           fixed + safe area

UI-03 phone: View replaces the creation body
[Back] Conversation preview                fixed
[Range >] [Section v]
[Complete historical content]              scroll body
[Apply selection]                          fixed + safe area

UI-04
[Expired snapshot] [Rebuild] [x]            input preserved
```

Workspace row on desktop and phone: new agent = Shared, new task = Separate.
Child-task forms offer [Share parent workspace | Separate workspace].
Use tap targets of at least 44 pixels on phone and coarse pointers.
Use one active vertical scroll owner. Back returns to creation without losing input.
Compare rendered desktop and phone surfaces against these labels during the assigned E2E checks.

## Verification

Run from the repository root. Add failing behavioral tests before production changes.
All new test names and files are specified in the plan's coverage table.
A missing test file or selector alone is not behavioral RED evidence.

```bash
(cd apps/web && pnpm exec vitest run components/task/conversation-fork-destinations.test.tsx components/task-create-dialog-submit.test.tsx components/task/new-subtask-form-state.test.ts components/task/new-subtask-dialog-context.test.ts components/task/use-subtask-submit.test.ts lib/api/domains/kanban-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/session/conversation-fork-tasks.spec.ts tests/session/conversation-fork-agent.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-conversation-fork-tasks.spec.ts tests/session/mobile-conversation-fork-agent.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The managed E2E runner builds current backend and web artifacts. Run these commands sequentially, with no worker overrides.
Capture the rendered preview during these runs and record any structural mismatch.
Run targeted ESLint on the TS/TSX files changed by this work order before completion.

## Files likely touched

- `apps/web/components/task-create-dialog-types.ts`, `task-create-dialog-submit.tsx`, and its tests
- `apps/web/components/task/new-subtask-dialog.tsx`, `new-subtask-form-state.ts`, `use-subtask-submit.ts`
- `apps/web/components/task/conversation-fork-flow.tsx` and shared context hook from Task 04
- `apps/web/lib/api/domains/kanban-api.ts` and its task-create request type
- `apps/web/components/task/conversation-fork-destinations.test.tsx` (new)
- `apps/web/e2e/tests/session/{conversation-fork-tasks,mobile-conversation-fork-tasks}.spec.ts` (new)
- `apps/web/e2e/tests/session/conversation-fork-helpers.ts`
- `apps/web/src/locales/` touched catalogs and generated variants
- `docs/public/tasks-and-workflows.md`; related agent, README, and screenshot guidance only where claims change
- This package and its paired specs for final statuses and results

## Dependencies

Task 04 must pass before this work starts.

## Risks

Existing subtask defaults can choose the primary session. Fork source must always come from the selected message. Delayed task start must not rebuild context.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/tasks/requirements/conversation-forks.md) and [system design](../../specs/tasks/system-design/conversation-forks.md).
- `new-subtask-dialog.tsx`, `use-subtask-submit.ts`, `task-create-dialog-submit.tsx`, additional-session workspace reuse contract, and plan UI-01 through UI-04.

## Results

New-task and child-task destinations use the shared frozen snapshot and preview. New tasks retain their workspace, workflow, repository, and base defaults, then open the created task. Child tasks preserve their parent and expose shared or separate execution-workspace choices. Create-without-start, reload, provenance, later first launch, and mobile phone flows are covered. Public guidance now explains the cutoff, optional evidence, copied attachments, informational estimates, workspace behavior, and delayed launch.

Validation passed:

- Focused frontend tests: 141 tests passed across 14 files, including create payloads, new-task and subtask context, task destinations, and launch helpers.
- Managed desktop E2E: create-only new task survived reload and used its snapshot on first launch; child task used the selected shared workspace. The agent fork scenario passed in the same run.
- Managed mobile E2E: create-only new task survived reload and first launch; child task used the selected separate execution workspace. The agent fork preview and delivery scenario passed in the same run.
- `pnpm run typecheck`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet` passed. ESLint had no errors.
- `node --test scripts/validate-public-docs.test.mjs` passed 62 tests; `node scripts/validate-public-docs.mjs` validated 47 published pages.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, and `git diff --check` passed.

`pnpm run i18n:zh-hant` passed after the existing `workflows:openAgentSettings` phrase was added to the reviewed converter overrides. Fork entries pass the six-catalog completeness check and new-code ratchet.

### PR review remediation

The mobile child-task E2E now selects an isolation-capable worktree executor for the separate-workspace choice; the Local executor is intentionally rejected before task creation. The test verifies the child-task form and preview fit the phone surface after their opening animations settle.

Validation passed: targeted mobile child-fork E2E with retries disabled. The shared-child and same-task agent flows remain available.
