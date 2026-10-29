---
id: "02-resolve-project-providers"
title: "Resolve project URLs through shared provider admission"
status: done
wave: 2
depends_on:
  - "01-discover-plugin-providers"
plan: "plan.md"
requirements:
  - REQ-TASKS-PROJECT-REPOSITORIES-001
  - REQ-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001
  - REQ-PLUGINS-REPOSITORY-URL-DISCOVERY-001
acceptance_criteria:
  - AC-TASKS-PROJECT-REPOSITORIES-001.1
  - AC-TASKS-PROJECT-REPOSITORIES-001.2
  - AC-TASKS-PROJECT-REPOSITORIES-001.3
  - AC-TASKS-PROJECT-REPOSITORIES-001.6
  - AC-TASKS-PROJECT-REPOSITORIES-001.10
  - AC-TASKS-PROJECT-REPOSITORIES-001.11
  - AC-TASKS-PROJECT-REPOSITORIES-001.12
  - AC-TASKS-PROJECT-REPOSITORIES-001.13
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.3
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.4
  - AC-WORKSPACES-REMOTE-REPOSITORY-REGISTRATION-001.6
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.3
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.4
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.5
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.8
system_design:
  - ../../specs/tasks/system-design/project-repository-context.md
  - ../../specs/workspaces/system-design/remote-repository-registration.md
  - ../../specs/plugins/system-design/repository-url-discovery.md
---

# Task 02: Resolve project URLs through shared provider admission

## Summary

Resolve unhinted remote project URLs from verified built-in connections and active repository plugins before any repository or task write.
Preserve explicit selections and reject unknown or ambiguous candidate sets.

## In scope

- Add `resolveWorkspaceRemoteProvider` and integrate it into `preflightRepositoryInputs`.
- Gather verified GitHub/GitLab candidates and Task 01's plugin discovery candidates for unhinted custom URLs.
- Accept exactly one positive candidate. Keep fatal discovery errors separate from ordinary candidate failures.
- Add a discovery-specific trust wrapper without weakening explicit provider equality or immutable pinning checks.
- Apply origin verification to explicit GitLab inputs and propagate the modified input through the slice.
- Remove registration's duplicate verifier call after common preflight owns verification.
- Add project success/refusal, arbitration, mixed-source, and real adapter tests named in the plan.
- Preserve saved IDs, local inputs, Host descriptors, explicit plugins, external-ID retries, and lower resolver guards.

## Out of scope

- Broad parser fallback, new transports, credentials, schema, external plugin implementations, UI, and executor changes.
- Weakening direct `ResolveRepositoryRef` refusal for untrusted GitLab inputs.

## Acceptance

1. RED project cases for self-managed GitLab and bare plugin URLs pass after the correction, including subgroup/context paths and mixed project sources.
2. Unknown, ambiguous, canceled, stale, and invalid selections leave repository/task/attachment counts unchanged and publish no creation event in either source order.
3. Public built-ins, uniquely configured Enterprise, explicit providers, registration reuse, saved/local inputs, and idempotent retries retain their current outcomes.

## Verification

Run from the repository root. Capture behavioral RED before production changes.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^Test(CreateTaskProjectRemoteProviders|RepositoryPreflightRemoteProviderDiscovery)$' -count=1 -v)
```

After the correction, run:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/task/service -run 'Test(CreateTask.*Project|ProjectRemoteProvider|RepositoryPreflightRemoteProviderDiscovery|RegisterRemoteRepository|.*Enterprise.*Repository|ParseRemoteRepositoryURL|.*PluginRepository|.*RepositorySelection|ResolveRepositoryRef_RejectsUntrustedGitLabOrigins)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run '^Test(CreateTaskProjectRemoteProviderAdapters|GitLabRepositoryOriginVerifier|GitHubRepositoryOriginVerifier|OfficeProjectRepositoryReaderWiring|PluginRepositorySelection)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/task/service/workspace_remote_provider.go apps/backend/internal/task/service/workspace_remote_provider_test.go apps/backend/internal/task/service/project_remote_provider_test.go apps/backend/internal/task/service/repository_selection.go apps/backend/internal/task/service/service_remote_repositories.go apps/backend/internal/backendapp/repository_origin_task_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

The commands cover every new success, refusal, arbitration, and adapter suite.
Include any additional changed Go file in the final lint command and record it in Results.

## Files likely touched

- `apps/backend/internal/task/service/workspace_remote_provider.go` and `_test.go` (new).
- `apps/backend/internal/task/service/project_remote_provider_test.go` (new).
- `apps/backend/internal/task/service/repository_selection.go`.
- `apps/backend/internal/task/service/service_remote_repositories.go`.
- `apps/backend/internal/backendapp/repository_origin_task_test.go` (new).
- This work order and `plan.md`, for delivery status and actual results.

## Dependencies

Task 01 supplies the discovery result and production adapter.

## Risks

- GitHub-first precedence must not silently choose an ambiguous custom host.
- Empty provider IDs cannot bypass the existing explicit provider equality check.
- Loop-local input changes must reach the actual slice before resolution.
- Source deduplication must not hide a rejected source.
- A partial discovery result is not safe after cancellation or lifecycle change.

## Parallelism

`sequential`

## Inputs

- [Project requirement](../../specs/tasks/requirements/project-repository-context.md), criteria .1-.3, .6, and .10-.13.
- [Shared admission design](../../specs/tasks/system-design/project-repository-context.md#workspace-verified-remote-admission).
- [Registration requirement](../../specs/workspaces/requirements/remote-repository-registration.md), criteria .3, .4, and .6.
- [Registration design](../../specs/workspaces/system-design/remote-repository-registration.md#failure-and-recovery).
- [Plugin discovery design](../../specs/plugins/system-design/repository-url-discovery.md).
- Task 01 results, the plan's compatibility matrix, and existing project/Enterprise/plugin test fixtures.
- `/tdd`, backend guidance, and the unchanged strict parser and origin verifier implementations.

## Results

Completed. `preflightRepositoryInput` now routes unhinted, non-public remote URLs through `resolveWorkspaceRemoteProvider`. That helper gathers verified GitHub Enterprise and GitLab candidates from workspace connections and every validated plugin candidate from Task 01, then accepts exactly one. Two or more candidates are invalid as ambiguous, zero candidates prefer unavailable over invalid over not-found, and a unique match survives an unrelated candidate's ordinary inspection error.

Explicit provider selections stay pinned to their provider and never fall back. Explicit GitLab hints now pass through the shared origin verifier, and the verified input is written back into the actual preflight slice so `TrustedRemote` reaches resolution. Registration no longer repeats the origin check after preflight. Saved IDs, local paths, Host descriptors, explicit plugin selections, external-ID retries, and the lower `untrusted GitLab origin` refusal keep their existing paths.

The discovery-specific trust wrapper supplies the discovered provider from server authority before applying the existing scope and immutable-ID pinning checks, so the explicit-provider equality check is unchanged. Plugin descriptors persist exactly as returned, including context paths and clone URLs.

Implemented files:

- `apps/backend/internal/task/service/workspace_remote_provider.go` and `_test.go` (new).
- `apps/backend/internal/task/service/project_remote_provider_test.go` (new).
- `apps/backend/internal/task/service/repository_selection.go`, `repository_origin.go`, `service_remote_repositories.go`.
- `apps/backend/internal/backendapp/repository_origin_task_test.go` (new).

`TestCreateTaskProjectRemoteProviders` covers a self-managed GitLab project source, a bare plugin project source, mixed built-in and plugin sources, and reuse of a registered GitLab identity. `TestProjectRemoteProviderFailuresBeforeWrites` covers built-in and plugin ambiguity, unknown hosts, foreign connections, interrupted discovery, an invalid discovered clone origin, and both source orders with no repository, task, attachment, or `task.created` writes. `TestRepositoryPreflightRemoteProviderDiscovery` and `TestRepositoryPreflightRegistrationSharesRemoteProviderAdmission` cover explicit-provider isolation, GitLab trust propagation, failure arbitration, saved/local bypass, and exactly one verification for bare and explicitly hinted registration. `TestCreateTaskProjectRemoteProviderAdapters` proves the selected workspace reaches the real GitLab connection reader and that the real plugin adapter persists the exact descriptor.

Validation passed:

- `go test -trimpath -tags fts5 ./internal/task/service -count=1` (368.895s)
- `go test -trimpath -tags fts5 -race ./internal/backendapp -run '^Test(CreateTaskProjectRemoteProviderAdapters|GitLabRepositoryOriginVerifier|GitHubRepositoryOriginVerifier|PluginRepositorySelection)' -count=1`
- `go test -trimpath -tags fts5 ./internal/task/service -run 'Test(CreateTask.*Project|ProjectRemoteProvider|RepositoryPreflight|RegisterRemoteRepository|.*Enterprise.*Repository|ParseRemoteRepositoryURL|.*PluginRepository|.*RepositorySelection|ResolveRepositoryRef_RejectsUntrustedGitLabOrigins)' -count=1`
- `bash scripts/lint-go-changed` over the listed files: 0 issues.
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`, `git diff --check`.

Review remediation:

- `verifyRemoteRepositoryOrigin` now short-circuits saved repository IDs, so a saved plugin or self-managed GitLab repository resolves even when its provider or connection is unavailable. `resolveRepoInputID` still refuses a foreign workspace ID.
- `resolveWorkspaceRemoteProvider` rechecks caller cancellation before accepting a candidate, so an interrupted pass cannot authorize a built-in candidate.
- `TestCreateTaskSavedRepositoryIDSkipsRemoteProviderAdmission` and `TestRepositoryPreflightRejectsCanceledCandidateSet` cover both contracts.
