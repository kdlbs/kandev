---
id: "03-e2e-evidence"
title: "End-to-end evidence on desktop and mobile"
status: done
wave: 3
depends_on: ["02-agent-column-presentation"]
plan: "plan.md"
requirements:
  - REQ-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001
acceptance_criteria:
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.2
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.4
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.5
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.7
  - AC-TASKS-TASK-CREATE-AGENT-COMPATIBILITY-001.9
system_design:
  - ../../specs/tasks/system-design/task-create-agent-executor-compatibility.md
---

# Task 03: End-to-end evidence on desktop and mobile

## Summary

Prove the rendered states against the production build on desktop and on a
phone viewport. The E2E backend runs the mock agent as its only agent type, so
each case first restarts it with `KANDEV_MOCK_PROVIDERS: "codex-acp"` to get a
second, compatible agent type, then restarts it back to the baseline.

## In scope

- Add `apps/web/e2e/tests/task/agent-compatibility-helpers.ts`: create a
  Codex-alias profile, a Docker executor profile without credentials, the
  `/api/v1/remote-credentials` route mock (env secret required for the seeded
  agent, no methods for the alias), and a workflow that pins a profile and is
  remembered as the dialog's last-used workflow.
- Extend `apps/web/e2e/tests/task/create-task.spec.ts` with two cases: pick
  the seeded agent on the default executor, switch to the Docker profile, and
  assert the selector now shows the Codex profile with no empty state or note
  and an enabled start action; and a workflow that pins the seeded profile,
  asserting the `agent-profile-incompatible-note` names the workflow, the agent
  profile, and the executor profile, that the credentials link targets the
  executor profile, that `agent-profile-empty-state` is absent, and that the
  start action is disabled.
- Keep the existing empty-state case green.
- Add `apps/web/e2e/tests/task/mobile-create-task-agent-compatibility.spec.ts`
  driving the same workflow-locked flow through `MobileKanbanPage` at the
  configured phone device. Assert the note is visible, the link is tappable,
  the start action is disabled, and `document.documentElement.scrollWidth`
  does not exceed the viewport width.

## Out of scope

- Container-backed executor projects. The Docker profile is never launched;
  only the dialog's gate is exercised.

## Acceptance

- The desktop workflow-locked case passes and the existing Docker empty-state
  case still passes.
- The mobile spec passes under the `mobile-chrome` project with no document
  horizontal overflow.

## Verification

```bash
# From the repository root, after Task 02. The harness runs the built backend
# binary, the E2E web bundle, and the E2E plugin package, so all three must be
# current:
make build-backend build-web-e2e build-e2e-plugin-package
# From apps/web:
pnpm e2e:raw tests/task/create-task.spec.ts
pnpm e2e:raw --project=mobile-chrome tests/task/mobile-create-task-agent-compatibility.spec.ts
```

## Files likely touched

- `apps/web/e2e/tests/task/agent-compatibility-helpers.ts`
- `apps/web/e2e/tests/task/create-task.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-agent-compatibility.spec.ts`

## Dependencies

Task 02.

## Risks

- Workflow creation with a pinned agent profile needs the API client helper
  used by `workflow-agent-profile.spec.ts`; reuse it rather than posting raw
  requests.
- The note copy is asserted through `toContainText` on the workflow, agent,
  and executor names, not on the full sentence, so translations and wording
  edits do not break the spec.

## Parallelism

`sequential`

## Inputs

- Plan section "E2E tests".
- Existing "explains when a Docker executor has no compatible agent
  credentials" case in `create-task.spec.ts`.
- `apps/web/e2e/tests/task/mobile-create-task-branch-policy.spec.ts` as the
  mobile exemplar.

## Results

- Added `agent-compatibility-helpers.ts` (second agent type through
  `backend.restart({ KANDEV_MOCK_PROVIDERS: "codex-acp" })`, Codex-alias
  profile, Docker profile without credentials, catalog route mock, remembered
  locked workflow) and two desktop cases plus the mobile spec.
- Two false starts recorded for the next reader: the first scenario locked the
  only agent type and therefore produced the empty state, not the locked note;
  and clicking the already-selected seed option toggled it off, so the
  replacement test now selects the seed only when something else is selected
  and asserts the replacement is a Codex profile (the alias also ships a
  default profile) rather than one specific name.
- Prerequisites in a fresh worktree: `make build-backend build-web-e2e
  build-e2e-plugin-package` and `pnpm exec playwright install chromium`.
- `pnpm e2e:raw tests/task/create-task.spec.ts`: 17 passed (2.8 m).
  `pnpm e2e:raw --project=mobile-chrome
  tests/task/mobile-create-task-agent-compatibility.spec.ts`: 1 passed.
