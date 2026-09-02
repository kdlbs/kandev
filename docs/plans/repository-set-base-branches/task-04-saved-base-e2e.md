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
