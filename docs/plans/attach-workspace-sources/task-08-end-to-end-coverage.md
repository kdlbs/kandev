---
id: "08-end-to-end-coverage"
title: "End-to-end coverage"
status: completed
wave: 5
depends_on: ["05-remote-materialization", "07-files-panel-surface"]
plan: "plan.md"
spec: "../../specs/tasks/attach-workspace-sources.md"
---

# Task 08: End-to-End Coverage

## Acceptance

- Desktop and `mobile-chrome` tests prove mixed local repository/folder attachment, live Files
  refresh, Git-only scoping, reload persistence, mobile geometry, and active-turn gating.
- Container-gated Docker and SSH tests prove repository materialization, folder capability gating,
  rollback, relaunch/reconnect, and owned cleanup.
- Tests use isolated disposable repositories/folders and leave developer data untouched.

## Verification

```bash
rtk make build-web
rtk make build-backend
cd apps/web
rtk pnpm e2e -- e2e/tests/task/add-workspace-sources.spec.ts --project=chromium
rtk pnpm e2e -- e2e/tests/task/mobile-add-workspace-sources.spec.ts --project=mobile-chrome
rtk env KANDEV_E2E_CONTAINERS=1 pnpm e2e -- e2e/tests/docker/add-workspace-sources.spec.ts e2e/tests/ssh/add-workspace-sources.spec.ts --project=containers
```

## Files likely touched

- `apps/web/e2e/tests/task/add-workspace-sources.spec.ts` (new)
- `apps/web/e2e/tests/task/mobile-add-workspace-sources.spec.ts` (new)
- `apps/web/e2e/tests/docker/add-workspace-sources.spec.ts` (new)
- `apps/web/e2e/tests/ssh/add-workspace-sources.spec.ts` (new)
- focused E2E fixtures/helpers only when necessary

## Dependencies

Tasks 05 and 07.

## Inputs

- Spec Scenarios.
- `apps/web/e2e/README.md` and existing task-create, mobile file viewer, Docker launch, and SSH launch
  specs.

## Output contract

Summary, files changed, exact projects/tests run, artifacts/screenshots, blockers, risks, divergence,
and task/plan status updates.

## Result

- Managed desktop Chromium coverage passed: 2/2 tests.
- Managed Pixel 5/mobile Chrome coverage passed: 1/1 test, including enabled-state and focus return.
- Docker and SSH specs were added, collected, typechecked, and linted. Live execution was
  environment-blocked before source attachment: the managed runner did not expose its Docker
  socket, while the direct host Docker task did not reach an idle state within 120 seconds.
