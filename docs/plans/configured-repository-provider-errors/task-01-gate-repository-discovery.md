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


### PR #3598 remote picker fixture remediation, 2026-10-07

The shared desktop dialog opener could click an inert navigation item after a
prior test persisted a zero-height sidebar. Added a collapsed-navigation setup
to the repository-loading case. Before the repair, its first attempt and the
existing suite retry both failed with the same pointer obstruction reported by
CI. The opener now expands navigation through its visible control before
clicking New task. The suite retry override is zero; production UI is unchanged.

Reproduction command: `pnpm --dir apps/web e2e:run --host --no-build --shards 1
--project chromium tests/task/create-task-remote-repo.spec.ts -- --grep
'keeps the unified input fixed' --retries=0`. Repeat validation adds
`--repeat-each=3`; results are recorded below.

Collapsed-navigation regression: all three repeat runs passed with retries
disabled. Focused ESLint passed with zero warnings.

### PR #3598 optional disclosure remediation (2026-10-07)

Hosted shard 9 timed out waiting for `sidebar-navigation-expand` in picker
scenario 1. The sidebar only renders that disclosure when navigation is clipped
or a saved size is expandable. The failure artifact shows New Task already
present, so the earlier opener's unconditional attribute read was invalid.
The shared opener now uses the always-present divider's `End` keyboard action
before clicking New Task. Provider, repository, and dialog assertions remain.
Production UI and timeout/retry settings are unchanged. Rebuilt browser checks
and fresh hosted validation are pending.

After merging main's profile-enabled omission fix, the rebuilt combined desktop
command passed all 17 cases on their first attempts:
`E2E_PORT_OFFSET=0 pnpm e2e:run tests/task/create-task-remote-repo.spec.ts
 tests/workflow/workflow-move-preview.spec.ts
 tests/chat/queue-admission-reliability.spec.ts --project=chromium --retries=0`
from `apps/web`. Profile-enabled race checks passed through backendapp,
settings controller/store/handlers, MCP, and lifecycle. Focused ESLint, web
typecheck, catalog/spec lint, and 74-work-order coverage passed.

The matching mobile command then passed all 10 cases on their first attempts:
`E2E_PORT_OFFSET=0 pnpm e2e:run --no-build
 tests/task/mobile-create-task-remote-repo.spec.ts
 tests/workflow/mobile-workflow-move-preview.spec.ts
 tests/chat/mobile-queue-admission-reliability.spec.ts
 --project=mobile-chrome --retries=0` from `apps/web`.
Fresh hosted CI remains pending; these local passes do not establish its result.
