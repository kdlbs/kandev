---
id: "04-prove-phase-recovery"
title: "Prove end-to-end phase recovery"
status: completed
wave: 4
depends_on:
  - "03-expose-recovery-warnings"
plan: "plan.md"
requirements:
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-001
  - REQ-TASKS-MISSING-WORKSPACE-REHOME-002
  - REQ-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001
acceptance_criteria:
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.1
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.2
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.3
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.4
  - AC-TASKS-MISSING-WORKSPACE-REHOME-001.5
  - AC-TASKS-MISSING-WORKSPACE-REHOME-002.5
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.2
  - AC-EXECUTORS-CODER-TASK-ROOT-DURABILITY-001.4
system_design:
  - ../../specs/tasks/system-design/missing-workspace-rehome.md
  - ../../specs/executors/system-design/coder-task-root-durability.md
---

# Task 04: Prove end-to-end phase recovery

## Summary

Exercise the real workflow and SSH boundaries: complete phase A, reset context,
remove or rebuild the remote task directory, and prove phase B starts once on a
fresh binding while preserving task and step identity. Cover desktop/mobile
warnings and the terminal replacement-failure path.

## In scope

- Container-gated SSH workflow transition regression.
- Nested-parent-checkout SSH materialization at
  `/opt/jumprope-fullstack/.kandev` that reaches `RUNNING`.
- Concurrent transition/retry and bounded-failure assertions through real
  orchestration and persistence.
- Desktop and mobile recovery-action Playwright flows.
- Desktop and mobile Coder profile warning flows.

## Out of scope

- Tests against Jesse's production Coder workspace.
- Broad unrelated E2E or backend suites.

## Acceptance

- The deleted-directory phase transition keeps the original task and target
  step and produces one replacement workspace/session.
- A second replacement failure remains visible after reload with both causes.
- Desktop and phone users can understand and authorize the loss warning; the
  phone document has no horizontal overflow.

## Verification

```bash
cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/ssh/workflow-missing-workspace-rehome.spec.ts
cd apps/web && pnpm e2e:run tests/task/launch-failure-recovery.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-launch-failure-recovery.spec.ts
cd apps/web && pnpm e2e:run tests/settings/executor-profile-coder-root-warning.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-executor-profile-coder-root-warning.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/ssh/workflow-missing-workspace-rehome.spec.ts`
- `apps/web/e2e/tests/task/launch-failure-recovery.spec.ts`
- `apps/web/e2e/tests/task/mobile-launch-failure-recovery.spec.ts`
- `apps/web/e2e/tests/settings/executor-profile-coder-root-warning.spec.ts`
- `apps/web/e2e/tests/settings/mobile-executor-profile-coder-root-warning.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`

## Dependencies

- Tasks 01 through 03.

## Risks

- The container project must delete the task directory without also destroying
  the SSH host fixture or hiding the typed probe path.
- Race coverage must use barriers or controllable fakes rather than sleeps.

## Parallelism

`sequential`

## Inputs

- Completed unit/integration work orders.
- Existing SSH container project and launch-failure desktop/mobile specs.

## Results

Pending.
