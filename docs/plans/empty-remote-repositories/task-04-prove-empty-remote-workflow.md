---
id: "04-prove-empty-remote-workflow"
title: "Prove the Empty Remote Workflow"
status: done
wave: 4
depends_on:
  - "03-surface-publication-recovery"
plan: "plan.md"
requirements:
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001
  - REQ-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002
acceptance_criteria:
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.1
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.4
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-001.5
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.1
  - AC-WORKSPACES-EMPTY-REMOTE-REPOSITORIES-002.8
system_design:
  - ../../specs/workspaces/system-design/empty-remote-repositories.md
---

# Task 04: Prove the Empty Remote Workflow

## Summary

Add desktop and phone Playwright scenarios for empty-remote task launch and Push. Inspect the disposable remote to prove base-first publication.

## In scope

- Add a disposable empty Git remote fixture and cleanup.
- Start a task from that remote through the normal task workflow.
- Create and commit one project change.
- Push through the desktop Changes action and inspect both remote refs.
- Push through the phone Changes menu and prove the same outcome.

## Out of scope

- Call only APIs without a user-facing assertion.
- Add a new Changes control.
- Use a live provider account or developer repository.
- Test provider API change-request creation through Playwright.

## Acceptance

- The task reaches an agent-ready state instead of launch failure.
- Desktop Push publishes the empty base before the task branch.
- The phone Changes path completes the same publication with existing touch controls.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run tests/git/empty-remote-repository.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/git/mobile-empty-remote-repository.spec.ts)
```

## Files likely touched

- `apps/web/e2e/helpers/empty-remote-repository.ts`
- `apps/web/e2e/tests/git/empty-remote-repository.spec.ts`
- `apps/web/e2e/tests/git/mobile-empty-remote-repository.spec.ts`

## Dependencies

- Task 01 enables launch from the empty remote.
- Task 02 enables base-first publication.
- Task 03 provides user-visible recovery messages.

## Risks

- The worker-scoped E2E backend requires disposable remote and repository cleanup after failure.
- The mobile project discovers only files with the `mobile-` prefix.
- The test must wait for session state and Git events instead of fixed delays.

## Parallelism

`sequential`

## Inputs

- Desktop and mobile behavior in the empty-remote design.
- The E2E fixture state and cleanup rules.
- Existing Git Changes Playwright patterns.

## Results

Completed. Added a disposable bare-remote fixture and desktop/mobile scenarios that prove launch without remote mutation, local commit, base-first Push, and both published refs. The desktop scenario and `mobile-chrome` scenario each passed.
