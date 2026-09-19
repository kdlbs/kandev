---
id: "04-end-to-end-inheritance"
title: "End-to-end inheritance"
status: done
wave: 4
depends_on: ["02-creator-session-resolution", "03-explain-session-default"]
plan: "plan.md"
spec: "../../specs/tasks/requirements/mcp-task-agent-profile-default.md"
---

# Task 04: End-to-end inheritance

## Acceptance

- Desktop E2E proves that a non-primary creating session transfers its profile
  and changed runtime values to subtask and top-level initial sessions.
- Workspace-default E2E proves that the saved policy prevents creator runtime
  inheritance.
- Mobile E2E proves that the revised setting is touch-usable, readable, and free
  of horizontal overflow.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/task/subtask.spec.ts)
(cd apps/web && pnpm e2e:run tests/task/mcp-task-agent-profile-default.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-mcp-task-agent-profile-default.spec.ts)
```

## Files likely touched

- `apps/web/e2e/tests/task/subtask.spec.ts`
- `apps/web/e2e/tests/task/mcp-task-agent-profile-default.spec.ts`
- `apps/web/e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts`
- `apps/web/e2e/helpers/api-client.ts` only if no existing session-config helper
  can drive the scenario

## Dependencies

- Task 02 supplies creator-session inheritance.
- Task 03 supplies the final setting label and explanation.

## Parallelism

Sequential. These scenarios verify the integrated behavior and production web
build after Tasks 02 and 03.

## Inputs

- Spec: **Scenarios**
- Plan: **E2E Tests** and **Risks**
- E2E references: `fixture-state.md` and `ui-state-and-cleanup.md`

## Output contract

Report discovered test counts, files changed, exact managed-runner commands,
results, failure artifact paths, cleanup evidence, blockers, risks, and
synchronized task/plan status.

## Results

Added the desktop creator-session scenario, workspace-default runtime
suppression assertion, and mobile touch/viewport coverage. The desktop scenario
uses a changed second session to create both a subtask and a top-level task,
then verifies the creator profile, model, mode, and dynamic option values while
checking that the subtask keeps its executor profile.

Verification used the managed Docker runner after a final backend and web
build:

```text
pnpm e2e:run --no-build tests/task/subtask.spec.ts  PASS (15 tests)
pnpm e2e:run --no-build tests/task/mcp-task-agent-profile-default.spec.ts  PASS (1 test)
pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-mcp-task-agent-profile-default.spec.ts  PASS (1 test)
```

The disposable PR capture spec was removed after producing and validating the
compressed desktop and mobile assets. No temporary test file remains.
