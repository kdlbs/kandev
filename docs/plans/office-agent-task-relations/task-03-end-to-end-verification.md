---
id: "03-end-to-end-verification"
title: "Verify agent task relations end to end"
status: done
wave: 3
depends_on:
  - "01-runtime-create-relations"
  - "02-cli-and-skills"
plan: "plan.md"
requirements:
  - REQ-OFFICE-AGENT-TASK-RELATIONS-001
  - REQ-OFFICE-AGENT-TASK-RELATIONS-003
  - REQ-OFFICE-AGENT-TASK-RELATIONS-004
acceptance_criteria:
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.1
  - AC-OFFICE-AGENT-TASK-RELATIONS-001.3
  - AC-OFFICE-AGENT-TASK-RELATIONS-003.1
  - AC-OFFICE-AGENT-TASK-RELATIONS-003.5
  - AC-OFFICE-AGENT-TASK-RELATIONS-004.1
system_design:
  - ../../specs/office/system-design/agent-task-relations.md
---

# Task 03: Verify Agent Task Relations End to End

## Summary

Drive the real `agentctl` binary against a running backend in the existing
Playwright `agentctl-cli.spec.ts` suite. Prove that an agent-created join task
gets both edges, that an out-of-scope blocker is refused, and that an explicit
`new_workspace` child records its own workspace mode while an omitted mode still
inherits.

## In scope

- New tests in `apps/web/e2e/tests/office/agentctl-cli.spec.ts`, or a new
  sibling spec if that file nears the lint limit:
  1. **Fan-in:** seed a run on a parent task, create children A and B through
     the CLI, then create a join task with `--blocked-by A,B --assignee
     <worker>`. Assert the join task reports both A and B in `depends_on`.
  2. **Scope denial:** an unrelated root task in the same workspace passed to
     `--blocked-by` exits 1, and no task is created.
  3. **Workspace:** a child created with `--workspace-mode new_workspace`
     reports `metadata.workspace.mode == "new_workspace"`; a child created
     without the flag reports `inherit_parent`.
- Any `officeApi` / `apiClient` fixture helper needed to read a task's
  dependencies.

## Out of scope

- Launching real agent sessions or asserting worktree paths on disk. Backend
  tests in 01 cover environment binding.
- Mobile UI work. This is a CLI and backend flow with no rendered UI.

## Acceptance

- All new E2E tests pass on the guarded runner, with one worker.
- The targeted backend suites from 01 and 02 still pass.
- `make -C apps/backend fmt`, then `make -C apps/backend typecheck test lint`,
  pass.

## Verification

From `apps/backend`:

```bash
rtk make build
```

From `apps/web`:

```bash
pnpm e2e:run -- tests/office/agentctl-cli.spec.ts
```

From the repository root:

```bash
make -C apps/backend fmt
make -C apps/backend typecheck test lint
```

## Files likely touched

- `apps/web/e2e/tests/office/agentctl-cli.spec.ts`
- `apps/web/e2e/fixtures/` dependency helper (if missing)

## Dependencies

Work orders 01 and 02.

## Risks

- Wake timing is covered by `REQ-OFFICE-SCHEDULER-002` tests. Do not assert on
  runs here.
- The spec file is already long. Use a new sibling spec if the lint limit is
  near.

## Parallelism

Sequential, last.

## Inputs

- `apps/web/e2e/README.md` and the `/e2e` skill.
- The existing `agentctl-cli.spec.ts` token-minting pattern (`seedRun`,
  `mintRuntimeToken`).

## Results

- Added `apps/web/e2e/tests/office/agentctl-task-relations.spec.ts`: fan-in join
  edges, out-of-scope blocker refusal (no ID leaked), explicit `new_workspace` vs
  omitted mode, and removed flags failing as unknown. No fixture changes were
  needed (`getTaskDependencies`, `getTask`, `seedRun`, `mintRuntimeToken` exist).
- `pnpm --dir apps/web e2e:run --host --shards 1 --project chromium
  tests/office/agentctl-task-relations.spec.ts`: 4 passed.
- ESLint on the spec and `apps/web` typecheck passed.
