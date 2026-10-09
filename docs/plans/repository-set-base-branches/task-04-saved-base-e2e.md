---
id: "04-saved-base-e2e"
title: "Prove saved-base workflows"
status: done
wave: 4
depends_on:
  - "03-responsive-inline-editor"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-REPOSITORY-SETS-002
  - REQ-WORKSPACES-REPOSITORY-SETS-003
acceptance_criteria:
  - AC-WORKSPACES-REPOSITORY-SETS-002.2
  - AC-WORKSPACES-REPOSITORY-SETS-002.5
  - AC-WORKSPACES-REPOSITORY-SETS-002.7
  - AC-WORKSPACES-REPOSITORY-SETS-002.8
  - AC-WORKSPACES-REPOSITORY-SETS-002.9
  - AC-WORKSPACES-REPOSITORY-SETS-003.4
  - AC-WORKSPACES-REPOSITORY-SETS-003.6
  - AC-WORKSPACES-REPOSITORY-SETS-003.9
system_design:
  - ../../specs/workspaces/system-design/repository-sets.md
---

# Task 04: Prove Saved-Base Workflows

## Summary

Extend repository-set Playwright coverage for saved bases. Prove the same user
outcome on desktop and phone layouts.

## In scope

- Create and edit saved member bases in repository settings.
- Apply a configured set in New Task and create the task.
- Prove persisted task-repository bases.
- Prove phone drawer containment, scrolling, safe footer, and touch targets.
- Prove branch search, refresh, remote-qualified names, and origin badges.
- Prove that large-set editor open does not start branch requests for every row.

## Out of scope

- Full E2E suite execution.
- Remote URL and Quick Chat flows.
- Visual asset capture.

## Acceptance

- Desktop creates a task with the configured base for each set member.
- Settings uses the same searchable and origin-labeled branch rows as New Task.
- Phone settings and task creation produce the same saved-base result.
- Phone tests show no document-level horizontal overflow.

## Verification

Run `make build-web` from the repository root. Run the `pnpm` commands from
`apps/web`.

```bash
make build-web
```

```bash
pnpm e2e:raw --project=chromium e2e/tests/settings/workspace-repository-sets.spec.ts e2e/tests/task/create-task-repository-sets.spec.ts
pnpm e2e:raw --project=mobile-chrome e2e/tests/settings/mobile-workspace-repository-sets.spec.ts e2e/tests/task/mobile-create-task-repository-sets.spec.ts
```

## Files likely touched

- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/settings/workspace-repository-sets.spec.ts`
- `apps/web/e2e/tests/settings/mobile-workspace-repository-sets.spec.ts`
- `apps/web/e2e/tests/task/create-task-repository-sets.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-repository-sets.spec.ts`

## Dependencies

- Task 03 completes the responsive UI and task workflow.

## Risks

- E2E branch fixtures need distinct branches in every test repository.
- A stale web build can hide frontend changes.

## Parallelism

`sequential`

## Inputs

- Desktop and mobile repository-set E2E patterns
- Mobile layout assertions
- System-design verification strategy

## Results

- Desktop settings and task-create coverage verifies saved-base editing,
  application, task persistence, idempotence, and Save as set behavior.
- Desktop settings coverage opens the shared New Task branch picker and verifies
  grouped search, refresh, local and remote-qualified values, and origin badges.
- Phone settings coverage verifies the full-height drawer, internal scroll,
  safe-area action footer, touch targets, picker search/refresh, remote badges,
  viewport containment, and no document overflow.
- Phone task-create coverage verifies the same saved base reaches the created
  task repository.
- Verification: `make build-web`, desktop E2E (8 tests), and mobile E2E (4
  tests) pass.


### Hosted phone picker handoff follow-up

At PR head `11b985f7df`, E2E run `37540216925`, shard 1 job `112541511492`
failed the strict flake gate: 227 tests passed, two skipped, and this drawer
case passed on retry after its initial branch search input disappeared.
The original failed snapshot retains the editor but no branch dropdown.
The picker interaction that failed at the preceding head passed in this run.

Cold reproduction passed three times; keeping another repository available
also passed three times. A controlled slower repository-menu exit passed once.
These checks did not reproduce the exact dismissal, so the original cause is
unconfirmed. The fixture previously observed removal of the repository option,
which does not establish that its popover and focus cleanup have finished.
It now keeps an unused repository available, waits for the add-repository
popover to unmount and its enabled opener to regain native focus, then opens
the branch picker. The unused repository is removed during owned cleanup.
No production UI, timeout, retry or animation override changed; temporary
animation diagnostics were removed. All original search, refresh, badge,
touch-target, hit-target, viewport and overflow assertions remain.

`KANDEV_RUN_QUIET_DIR=/root/.cache/kandev-pr3598-quiet-owned E2E_PORT_OFFSET=0 GOMAXPROCS=4 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome -- tests/settings/mobile-workspace-repository-sets.spec.ts --repeat-each=3 --retries=0 --trace=retain-on-failure`
passed all nine cases in 32.3 seconds. Pushed-head hosted CI remains the
independent confirmation; no source startup or focus bug is claimed fixed.

Desktop parity:
`KANDEV_RUN_QUIET_DIR=/root/.cache/kandev-pr3598-quiet-owned E2E_PORT_OFFSET=0 GOMAXPROCS=4 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project chromium -- tests/settings/workspace-repository-sets.spec.ts --retries=0 --trace=retain-on-failure`
passed both cases in 8.6 seconds. Web typecheck, scoped ESLint, catalog/full spec
lint, whitespace, and the 58-work-order coverage preflight passed.
