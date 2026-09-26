---
created: 2026-09-07
status: done
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001
system_design:
  - ../../specs/integrations/system-design/azure-devops-integration-01.md
legacy_specs: []
---

# Implementation Plan: Configured Repository Provider Errors

## Overview

Gate built-in remote repository discovery on each workspace's existing
connection-availability signal. Unconfigured GitHub, GitLab, or Azure DevOps
integrations will be omitted silently, while a repository-list failure from an
eligible provider remains visible and does not hide successful provider
results.

## Scope

### In scope

- Derive the eligible built-in source-control providers for the task dialog's
  workspace before repository-list requests start.
- Preserve provider-specific errors for eligible providers and successful
  results from sibling providers.
- Cover the unconfigured GitLab regression and the configured-provider failure
  path with focused hook tests.
- Prove the user-visible result in the existing desktop and phone task-create
  repository picker flows.

### Out of scope

- Changing GitLab, GitHub, or Azure DevOps credential storage or status APIs.
- Making the browser-local enable/disable toggle a functional feature gate.
- Changing plugin repository-provider registration or error behavior.
- Changing repository picker layout, copy, manual URL entry, or branch loading.

## Technical approach

### Provider eligibility

Update `useRemoteRepositories` in
`apps/web/hooks/domains/integrations/use-remote-repositories.ts` to read the
existing workspace connection signals for the three built-in providers and
pass an explicit eligible-provider set into `loadBuiltInRepositories`.
Construct `RepositoryRequest` entries only for eligible providers. Re-evaluate
the set on workspace changes, integration-availability invalidation, and manual
refresh.

Keep the local `useGitHubEnabled`, `useGitLabEnabled`, and
`useAzureDevOpsEnabled` preferences out of this path. Their active requirement
defines them as presentation and navigation preferences, not feature gates.

### Error isolation

Keep `settleRepositoryRequests` as the partial-success boundary. Because
unconfigured providers never enter its request list, they cannot create
`sourceErrors`; because eligible providers do enter it, a later list failure
continues to produce the existing bounded inline error while sibling results
remain selectable.

### Responsive behavior

The shared hook feeds the existing desktop popover and phone task-create
picker. No composition or geometry changes are required. The nearest mobile
exemplar remains
`apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`: it already
proves the touch-sized input, provider tabs, manual URL fallback, and horizontal
containment. Extend it only with the connection-gating outcome.

## Tests

- `apps/web/hooks/domains/integrations/use-remote-repositories.test.tsx`
  - `AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.9`: GitHub connected and
    GitLab/Azure DevOps unconfigured calls only GitHub, returns its repositories,
    and exposes no source error.
  - `AC-INTEGRATIONS-AZURE-DEVOPS-INTEGRATION-001.10`: an eligible GitLab
    repository request that fails still reports its error while GitHub results
    remain available.
  - Refresh and workspace changes use the current eligibility set rather than a
    stale provider selection.

## E2E tests

- `apps/web/e2e/tests/task/create-task-remote-repo.spec.ts`: with GitHub
  connected and GitLab unconfigured, open the Remote picker, select the GitHub
  result, and assert that no GitLab repository request, GitLab provider tab, or
  repository-load error appears.
- `apps/web/e2e/tests/task/mobile-create-task-remote-repo.spec.ts`: prove the
  same unconfigured-provider behavior on the phone picker while manual URL
  entry and viewport containment remain available.

## Work orders

- [x] [Task 01: Gate repository discovery by connection](task-01-gate-repository-discovery.md) (`done`)

## Verification results

- Provider eligibility is derived from workspace-scoped GitHub, GitLab, and
  Azure DevOps connection status. Unconfigured providers do not enter the
  repository request set; eligible-provider failures remain bounded source
  errors beside successful results.
- `pnpm --filter @kandev/web exec vitest run` passed for the repository hook
  (13 tests), GitLab status (6 tests), and Azure DevOps connection (6 tests).
- Desktop and phone task-create regressions each passed with one Playwright
  test. The mobile run was repeated with build temporary files redirected to
  an agent-owned root-filesystem directory after the default `/tmp` mount
  filled during an earlier build.
- Frontend typecheck, targeted ESLint, and `python3 scripts/lint-spec-files.py
  --all` passed.

## Risks

- Connection probes settle asynchronously. The implementation must not show a
  false no-provider state or drop a provider that becomes available after the
  picker mounts.
- Explicit GitLab status consumers and Azure DevOps connection probes must stay
  keyed to their requested workspace.
- Existing tests assume all built-in list endpoints run unconditionally and
  must be updated without weakening configured-provider failure coverage.
