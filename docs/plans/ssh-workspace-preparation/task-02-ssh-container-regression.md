---
id: "02-ssh-container-regression"
title: "SSH container regression"
status: done
wave: 2
depends_on: ["01-ssh-workspace-lifecycle"]
plan: "plan.md"
spec: "../../specs/executors/requirements/ssh-executor.md"
---

# Task 02: SSH Container Regression

## Acceptance

- A real container-backed SSH task with a disposable provider repository proves that the remote task
  directory is the primary Git checkout and contains the expected repository content.
- A custom profile prepare script is observable before the agent turn completes; a terminal cleanup
  script is observable after archive/delete and is absent after a non-terminal stop.
- The test owns and tears down every repository server, Git rewrite, profile, and SSH container state
  it creates, including failure paths.

## Verification

```bash
cd apps
rtk pnpm install --frozen-lockfile
cd web
rtk env KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/ssh/launch-task.spec.ts
```

If the scenario lives in a focused sibling spec, substitute that exact file in the final command and
record Playwright's discovered/passed test count.

## Files likely touched

- `apps/web/e2e/tests/ssh/launch-task.spec.ts` or
  `apps/web/e2e/tests/ssh/workspace-preparation.spec.ts` (new)
- `apps/web/e2e/helpers/http-git-server.ts` only if a provider-URL option is required
- `apps/web/e2e/helpers/api-client.ts` only for a reusable default-script helper

## Dependencies

Task 01.

## Parallelism

Sequential. It validates Task 01's runtime behavior against the real SSH transport.

## Inputs

- The SSH spec's default preparation, custom hook, failure, reuse, and cleanup scenarios.
- `apps/web/e2e/fixtures/ssh-test-base.ts`, `apps/web/e2e/helpers/ssh.ts`, and the existing SSH
  workspace-source test's disposable HTTP Git pattern.

## Risks

- Do not expose host credentials or a developer checkout to the SSH container.
- Restore any system Git URL rewrite and shared profile/repository state in `finally`/fixture cleanup.
- Confirm the `containers` project discovers the intended test count; a skipped Docker gate is not
  passing evidence.

## Output contract

Report files changed, RED/GREEN run commands and counts, artifact paths on failure, owned-resource
teardown evidence, blockers/risks, and synchronized task/plan status.

## Results

- Updated the SSH fixture to serve a disposable provider-backed HTTP Git repository reachable from the SSH container via a scoped Git URL rewrite.
- Added assertions for the remote primary checkout, task branch, repository content, custom prepare marker, archive cleanup marker, preserved task directory, and session-directory behavior.
- Rebuilt the Linux mock-agent fixture and ran the exact container project command: 9 passed.
