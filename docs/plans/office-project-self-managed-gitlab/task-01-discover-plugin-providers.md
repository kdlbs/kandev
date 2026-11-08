---
id: "01-discover-plugin-providers"
title: "Discover active plugin repository providers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-REPOSITORY-URL-DISCOVERY-001
  - REQ-PLUGINS-REPOSITORY-TASK-CREATION-001
acceptance_criteria:
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.1
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.2
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.3
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.4
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.5
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.6
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.7
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.8
  - AC-PLUGINS-REPOSITORY-URL-DISCOVERY-001.9
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.2
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.3
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.4
  - AC-PLUGINS-REPOSITORY-TASK-CREATION-001.7
system_design:
  - ../../specs/plugins/system-design/repository-url-discovery.md
  - ../../specs/plugins/system-design/repository-provider-task-creation.md
---

# Task 01: Discover active plugin repository providers

## Summary

Add a reusable server discovery operation over active manifest-owned repository inspectors.
Return validated candidates and ordinary provider failure separately from operation errors, then expose the result through the existing task adapter.

## In scope

- Add `DiscoverRepositoryProviders`, its result type, and deterministic eligible-provider snapshots.
- Retain internal match/no-match/failure outcomes without changing the existing explicit inspector's error projection.
- Apply the design's four-call concurrency bound, shared 15-second deadline, cancellation, and candidate-snapshot recheck.
- Reuse manifest ownership, verified workspace dispatch, generation leases, and descriptor validation.
- Extend `RepositorySelectionResolver` with `DiscoverRepositorySelections` and its neutral result type.
- Extend `pluginRepositorySelectionResolver`, its inspector seam, and existing test stubs so all explicit callers still compile.
- Add `TestDiscoverRepositoryProviders` and `TestPluginRepositorySelectionDiscoveryUsesAuthoritativeDescriptors` before production implementation.

## Out of scope

- Task routing changes, new plugin actions, SDK changes, external plugin edits, and provider hostname lists.
- Direct access to plugin secrets or connection configuration.

## Acceptance

1. Table tests cover Bitbucket, Forgejo, and an arbitrary future provider through the same manifest/action path, including simultaneous successful matches.
2. Tests distinguish no-match, invalid descriptor, provider failure, operation deadline, cancellation, and replacement. Only validated current candidates leave discovery.
3. The real backend adapter preserves exact clone/scope/immutable identity and workspace context. Existing explicit selection tests retain their behavior.

## Verification

Run from the repository root. Capture RED for the new discovery contract before implementation.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/plugins -run '^TestDiscoverRepositoryProviders$' -count=1 -v)
```

After the implementation, run:

```bash
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/plugins -run '^Test(DiscoverRepositoryProviders|InspectRepositoryProvider)' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/backendapp -run '^TestPluginRepositorySelection' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/service -run '^Test(CreateTask.*Plugin|UpdateTaskPreflightsPlugin)' -count=1)
bash scripts/lint-go-changed apps/backend/internal/plugins/repository_provider_discovery.go apps/backend/internal/plugins/repository_provider_discovery_test.go apps/backend/internal/plugins/repository_provider_inspect.go apps/backend/internal/task/service/repository_selection.go apps/backend/internal/task/service/repository_selection_test.go apps/backend/internal/backendapp/repository_selection.go apps/backend/internal/backendapp/repository_selection_test.go
git diff --check
```

Include any additional changed Go file in the final lint command and record it in Results.
Use deterministic barriers for concurrency and lifecycle tests, with bounded cleanup for every started call.

## Files likely touched

- `apps/backend/internal/plugins/repository_provider_discovery.go` and `_test.go` (new).
- `apps/backend/internal/plugins/repository_provider_inspect.go`.
- `apps/backend/internal/task/service/repository_selection.go` (interface and result contract only).
- `apps/backend/internal/task/service/repository_selection_test.go` (compatible stub discovery behavior).
- `apps/backend/internal/backendapp/repository_selection.go` and `_test.go`.
- This work order and `plan.md`, for status and actual results.

## Dependencies

None. This operation and adapter are independently verifiable before task admission consumes them.

## Risks

- Collapsing `matched:false` and provider `404` loses the distinction between unrelated URL and failed repository lookup.
- A result from a replaced installation cannot retain selection authority.
- Returning the first success hides multiple valid claims.
- Treating a deadline as ordinary candidate failure exposes an incomplete result set.

## Parallelism

`sequential`

## Inputs

- [Discovery requirement](../../specs/plugins/requirements/repository-url-discovery.md).
- [Discovery design](../../specs/plugins/system-design/repository-url-discovery.md).
- [Explicit inspection design](../../specs/plugins/system-design/repository-provider-task-creation.md#inspection-action).
- [Discovery ADR](../../decisions/2026-10-29-provider-owned-repository-url-discovery.md).
- `repository_provider_inspect.go`, `repository_provider.go`, `registry.go`, `invoke.go`, and their current tests.
- `/tdd` and backend guidance. Do not create subagents.

## Results

Completed. `Service.DiscoverRepositoryProviders` snapshots every active manifest-owned provider that declares a workspace-scoped `repositories.inspect` action, orders them by plugin and provider ID for stable diagnostics, and inspects the bare URL with at most four simultaneous calls under one 15-second deadline. The internal inspection implementation now reports validated match, ordinary no-match, and typed failure separately; the explicit `InspectRepositoryProvider` keeps its previous not-found projection.

Discovery returns every validated descriptor plus the most severe ordinary candidate failure (unavailable, then invalid, then not-found). Caller cancellation returns the cancellation error, an expired discovery deadline returns an unavailable operation error, and a changed eligible provider, owner, or installation generation refuses the whole pass. Per-inspection sanitized logs are retained and one discovery log records candidate count, match count, duration, and failure category.

`RepositorySelectionResolver` gained `DiscoverRepositorySelections` and `RepositorySelectionDiscoveryResult`. `pluginRepositorySelectionResolver` maps each validated descriptor without rebuilding its clone URL and normalizes the candidate failure. Existing explicit-selection behavior is unchanged.

Implemented files:

- `apps/backend/internal/plugins/repository_provider_discovery.go` and `_test.go` (new).
- `apps/backend/internal/plugins/repository_provider_inspect.go`.
- `apps/backend/internal/task/service/repository_selection.go` and `_test.go`.
- `apps/backend/internal/backendapp/repository_selection.go` and `_test.go`.

`TestDiscoverRepositoryProviders` covers Bitbucket, Forgejo, and an arbitrary future provider, simultaneous matches, no-match, invalid descriptor, provider failure, failure severity, caller cancellation, the discovery deadline, a replaced installation, the four-call concurrency bound, sanitized logging, and an ineligible provider. `TestPluginRepositorySelectionDiscoveryUsesAuthoritativeDescriptors` and `TestPluginRepositorySelectionDiscoveryMapsOperationErrors` cover the real adapter.

Validation passed:

- `go test -trimpath -tags fts5 -race ./internal/plugins -run '^Test(DiscoverRepositoryProviders|InspectRepositoryProvider)' -count=1`
- `go test -trimpath -tags fts5 -race ./internal/backendapp -run '^TestPluginRepositorySelection' -count=1`
- `go test -trimpath -tags fts5 ./internal/task/service -run '^Test(CreateTask.*Plugin|UpdateTaskPreflightsPlugin)' -count=1`
- `bash scripts/lint-go-changed` over the listed files and the discovery files: 0 issues.

Review remediation:

- Dispatch read leases moved to `internal/plugins/dispatch_lock.go` (`contextRWMutex`). `InvokeAction`, `SearchEntityReferences`, `AuthorizeEntityReference`, and the plugin webhook handler use `beginPluginDispatchContext`, so a discovery pass waiting behind a config commit or restart write lease returns on cancellation or deadline instead of blocking past it. The write side keeps its exclusion, every started inspection is joined, and `runRepositoryProviderDiscovery` checks the caller context before its empty-snapshot return.
- Discovery emits exactly one sanitized completion event for every exit, including invalid request, deadline, cancellation, and changed eligibility, while per-candidate inspections keep their own sanitized entries. The explicit inspector logs invalid-request and missing-owner failures again.
- `TestDiscoverRepositoryProvidersLogsEveryExit`, `TestDiscoverRepositoryProvidersHonorsCancellationDuringDispatchLease`, `TestDiscoverRepositoryProvidersRejectsCanceledEmptySnapshot`, and `TestInspectRepositoryProviderLogsRejectedRequests` cover the new contracts.
