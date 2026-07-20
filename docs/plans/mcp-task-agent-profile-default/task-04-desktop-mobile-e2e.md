---
id: "04-desktop-mobile-e2e"
title: "Desktop and mobile E2E"
status: done
wave: 3
depends_on: ["02-mcp-profile-resolution", "03-responsive-task-actions-setting"]
plan: "plan.md"
spec: "../../specs/tasks/mcp-task-agent-profile-default/spec.md"
---

# Task 04: Desktop and Mobile E2E

## Acceptance

- Desktop Playwright proves that opting into workspace-default mode persists and causes an omitted-profile MCP-created task/subtask with no workflow profile to use the target workspace default rather than the caller profile.
- Existing or extended E2E coverage proves `current_task` remains the compatibility default.
- Mobile Playwright proves the Task Actions choice is reachable, saves across reload, remains touch-usable, and introduces no horizontal overflow; fixture reset restores `current_task` for every test.

## Verification

```bash
cd apps/web && pnpm e2e -- --project=chromium e2e/tests/task/mcp-task-agent-profile-default.spec.ts
cd apps/web && pnpm e2e -- --project=mobile-chrome e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts
```

If focused E2E does not rebuild production assets first:

```bash
make test-e2e
```

## Files Likely Touched

- `apps/web/e2e/tests/task/mcp-task-agent-profile-default.spec.ts`
- `apps/web/e2e/tests/task/mobile-mcp-task-agent-profile-default.spec.ts`
- `apps/web/e2e/tests/task/subtask.spec.ts`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/fixtures/test-base.ts`

## Dependencies

- `02-mcp-profile-resolution`
- `03-responsive-task-actions-setting`

## Inputs

- Spec: Scenarios for compatibility, workflow-before-workspace precedence, workspace default, subtask routing, and mobile persistence. Workflow precedence remains covered by focused Go integration tests if adding a second expensive browser execution would not improve viewport coverage.
- Plan: E2E Tests.
- Patterns: mock-agent `e2e:mcp:kandev:create_task_kandev(...)` directives and the existing `Subtask inheritance` tests in `apps/web/e2e/tests/task/subtask.spec.ts`; mobile settings navigation in `apps/web/e2e/tests/settings/mobile-general-settings.spec.ts`.
- Follow `/e2e` and `/mobile-parity`; run against freshly built production assets.

## Output Contract

Report seeded profile/workspace arrangement, desktop and mobile outcomes, exact commands and results, files touched, blockers, screenshots/overflow checks where applicable, and remaining flake risk. Set this task to `done` and update its plan checkbox only after both focused projects pass.
