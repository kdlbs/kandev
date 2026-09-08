---
id: "01-gate-repository-discovery"
title: "Gate repository discovery by connection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.9
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.10
  - AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.11
system_design:
  - ../../specs/integrations/system-design/azure-devops-integration-01.md
---

# Task 01: Gate Repository Discovery by Connection

## Summary

Make built-in repository discovery conditional on the selected workspace's
source-control connection availability. Preserve partial results and errors for
providers that were eligible, and prove identical behavior in the existing
desktop and phone task-create picker.

## In scope

- Add built-in provider eligibility to `useRemoteRepositories` and its loading,
  refresh, and workspace-change behavior.
- Prevent unconfigured built-in providers from issuing repository-list requests
  or contributing provider tabs and source errors.
- Retain current partial-success behavior for eligible provider failures.
- Add focused hook and task-create Playwright regressions.

## Out of scope

- Backend integration routes, credential persistence, and health polling.
- Browser-local integration toggle semantics.
- Plugin repository-provider discovery.
- Repository picker layout, translation copy, and branch-resolution behavior.

## Acceptance

- An unconfigured GitLab integration makes no GitLab project-list request and
  shows no GitLab repository error while connected GitHub repositories remain
  selectable.
- A GitLab integration that reports available and then fails its project-list
  request still contributes the existing bounded source error without removing
  successful provider results.
- Refresh, workspace changes, desktop, and phone all use current provider
  eligibility while manual URL entry remains available.

## Verification

```bash
cd apps && pnpm --filter @kandev/web exec vitest run hooks/domains/integrations/use-remote-repositories.test.tsx
cd apps/web && pnpm e2e:run tests/task/create-task-remote-repo.spec.ts -- --grep "unconfigured provider"
cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-create-task-remote-repo.spec.ts -- --grep "unconfigured provider"
cd apps/web && pnpm run typecheck
```

## Files likely touched

- `apps/web/hooks/domains/integrations/use-remote-repositories.ts`
- `apps/web/hooks/domains/integrations/use-remote-repositories.test.tsx`
- `apps/web/e2e/tests/task/create-task-remote-repo.spec.ts`
- `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`

## Dependencies

None.

## Risks

- Availability can change after mount; stale status must not suppress a newly
  connected provider or reintroduce an unconfigured provider after a workspace
  switch.
- The configured-provider path must keep reporting real outages rather than
  treating every failure as an unconfigured integration.

## Parallelism

`sequential`

## Inputs

- `docs/specs/integrations/requirements/azure-devops-integration.md`
- `docs/specs/integrations/system-design/azure-devops-integration-01.md`
- `docs/specs/integrations/requirements/enable-disable-toggle.md`
- `docs/decisions/2026-07-20-provider-neutral-remote-repositories.md`
- Existing hook tests and desktop/mobile task-create repository picker specs.

## Results

- Added workspace-scoped eligibility gating for GitHub, GitLab, and Azure
  DevOps repository discovery. Unconfigured providers no longer issue list
  requests or add provider tabs and source errors.
- Preserved partial-success behavior for eligible provider failures, including
  retry support through refreshed connection status and repository discovery.
- Keyed GitLab status by workspace and scoped Azure DevOps connection state to
  the requested workspace. Azure DevOps re-probes after availability
  invalidation and on the shared health cadence, with focused hook regressions
  for both behaviors.
- Updated repository-picker test harnesses to provide the state required by
  the shared connection probes.
- Added desktop and mobile task-create picker regressions proving the silent
  unconfigured-provider behavior and continued repository selection.
- Verification passed: 13 repository-hook tests, 6 GitLab tests, 6 Azure
  DevOps tests, 56 focused frontend tests, both focused Playwright tests,
  frontend typecheck, targeted ESLint, and specification linting.
