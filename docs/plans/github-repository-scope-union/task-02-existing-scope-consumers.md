---
id: "02-existing-scope-consumers"
title: "Union semantics at existing scope consumers"
status: done
wave: 2
depends_on:
  - "01-scope-model-and-migration"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.5
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.6
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.7
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.10
  - AC-INTEGRATIONS-GITHUB-REPOSITORY-SCOPE-001.11
system_design:
  - ../../specs/integrations/system-design/github-repository-scope.md
---

# Task 02: Union semantics at existing scope consumers

## Outcome and dependencies

Use the shared rule in existing consumers. Work order 01 must pass.
Read Existing Consumers, Copy/Reset, and Deferred Scope in the design.
Keep implementation local to mode handling and existing query/filter helpers.

## Owned scope

Paths below are under `apps/backend/internal/`.

- `github/workspace_settings_service.go`: qualifiers, scope filters, totals,
  cache keys, empty-scope checks, and result predicates.
- `github/service_reviews.go`, `service_issues.go`, and
  `service_accessible_repos.go`: audit current policy after provider/cache reads.
- New `github/workspace_settings_union_search_test.go`, plus affected
  `workspace_settings_test.go` and `watch_poll_lifecycle_test.go`.
- `automation/evaluator_pr_events_test.go`: real `GitHubEvaluator`/`ListAutomationPRs`
  evidence. Change production `evaluator.go` only for a proven union defect.
- `mentions/provider_github.go` and its tests: replace the old-mode switch
  after workspace and reference checks.
- `github/copy_test.go` and `workspace_clone_test.go`: verify canonical lists.
  `copyCloneSettings` uses raw `INSERT ... SELECT` after migration, so no new
  production clone path is needed. Settings copy retains credential exclusions.
- `github/store_enterprise_auth.go`: change the INSERT and ON CONFLICT
  `'repos'` literals to `selected`. Cover replacement and same-host reconnect
  in `enterprise_connections_test.go`.
- `github/service_app_auth_connection_store.go` and `webhook_service_test.go`:
  prune selected exact entries while preserving owner grants. Removing the last
  exact entry with no owners leaves empty selected scope, never `all`.
- Existing `github/app_credential_provider_test.go`: verify that named App minting
  stays restricted to the requested repository. No new broker reader or lifecycle.

## Acceptance

1. `TestWorkspaceScopeUnionSearch` and `TestWorkspaceScopeUnionWatch` prove owner
   and outside exact matches, overlaps, denied candidates, empty denial, custom
   query terms, totals, pagination, and fresh cache/cursor identity for either list.
   Existing explicit watch filters remain an intersection.
2. `TestGitHubPRScopeUnion` and `TestGitHubMentionScopeUnion` prove automation
   and mention outcomes through their existing services. Include allowed and
   denied candidates together, broad personal access, and wrong-workspace references.
3. Copy/clone preserve both arrays under their existing authentication contracts.
   Host replacement resets both lists, reconnect preserves them, and installation
   removal cannot widen access. Existing App mint behavior stays per-repository.

## Verification

Run the first new regression before implementation. Run from repo root:

```bash
(cd apps/backend && go test -race ./internal/github -run '^(TestWorkspaceScopeUnion(Search|Watch)|TestService_(SearchUserPRsPagedForWorkspace_.*|SearchUserIssuesPagedForWorkspace_.*|CheckReviewWatch_.*|CheckIssueWatch_.*)|TestCopyWorkspaceSettings.*|TestWorkspaceClone.*|TestEnterpriseHostReplacement.*|TestEnterprise.*Reconnect.*|TestWebhook.*|TestCachedInstallationCredentialProviderPreservesActorCapabilitiesAndExpiry)$' -count=1)
(cd apps/backend && go test -race ./internal/automation -run '^TestGitHubPR(ScopeUnion|EventSelection.*)$' -count=1)
(cd apps/backend && go test -race ./internal/mentions -run '^TestGitHub.*$' -count=1)
```

Name copy/clone/removal regressions with the corresponding command prefixes.
Do not add another PostgreSQL gate for enum-only compatibility changes.

## Exclusions, risks, and results

Preserve trigger selection, stored filters, provider bounds, identity ownership,
and token behavior. New scope enforcement in broker admission/redemption is
explicitly deferred. Never pair a repository branch as an org/user count pair.
Results:

- Focused `go test -race ./internal/github` passed for scope round-trip, patch
  compatibility, migration replay, old-schema startup, search/watch, consumer
  resets, and installation removal.
- `go test -race ./internal/automation -run '^TestGitHubPR(ScopeUnion|EventSelection.*)$' -count=1` passed.
- `go test -race ./internal/mentions -run '^TestGitHub.*$' -count=1` passed.
- `go test -race ./internal/backendapp -run '^TestGitHubSettingsOperationsScopeUnion$' -count=1` passed.
- The full backend test suite passed, including copy/clone and App credential tests.

The temporary overlay's union failures were recorded during investigation. The
permanent regressions now pass. A legacy migration fixture without scope columns
is handled as a no-op because it contains no saved scope values.
