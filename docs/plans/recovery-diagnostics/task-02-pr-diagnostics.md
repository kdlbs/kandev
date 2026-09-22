---
id: "02-pr-diagnostics"
title: "Distinguish PR lookup outcomes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.1
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.2
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.3
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.4
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-001.5
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 02: Distinguish PR lookup outcomes

## Summary

Separate provider errors from successful empty post-push searches. Apply the
same diagnostic contract to existing searching watches.

## In scope

- Update `detectPushAndAssociatePRWithIdentity` and `searchPRForExistingWatch`.
- Reuse GitHub discovery categories through a narrow exported classifier.
- Warn on failed attempts with resolved target identity and safe category.
- Keep empty and exhausted summaries at debug. Record counts and latest outcome.
- Preserve production delays, context cancellation, association, and admission.
- Add a private wait seam only as needed to exercise retries without real sleeps.

## Out of scope

Other work orders, terminal behavior, UI, schema, retry policy, and live-instance changes.

## Acceptance

1. Provider errors never become successful empty results or expose raw error text.
2. Empty results and cancellation produce no provider-failure or exhaustion warning.
3. Later success, watch reuse, repository identity, and association behavior remain correct.

## Regression evidence

Add `TestPushDiscoveryDiagnostics` and `TestExistingWatchDiscoveryDiagnostics`
in new `event_handlers_github_push_diagnostics_test.go`. Reuse service seeding
and mocks from `event_handlers_github_test.go` and multi-branch association tests.
First prove a provider error is currently absent from warnings and that all-empty
retries currently end with WARN. Use an observed logger through the real caller.

Cover all-empty, all-error, error then empty, empty then error, error then found,
and cancellation before lookup and during a delay. Include an existing searching
watch, a numbered watch, empty legacy repository name, and a secondary repository.
Assert exact provider calls, wait schedule, no duplicate associations, no health
mutation, and no raw credential-containing errors in logs. A successful lookup
must continue through existing association persistence. Extend classifier tests
in `service_pr_discovery_health_test.go` only if the exported seam needs coverage.
Do not duplicate the category rules in the orchestrator.

## Verification

Run the regression first and record its expected failure. After implementation,
run these commands from the repository root:

```bash
(cd apps/backend && go test ./internal/orchestrator -run 'Test(PushDiscoveryDiagnostics|ExistingWatchDiscoveryDiagnostics|DetectPushAndAssociatePR|GitHubPushAssociation|PushAssociation|ResolvePRWatchBranchForWatch)' -count=1)
(cd apps/backend && go test -race ./internal/orchestrator -run 'Test(PushDiscoveryDiagnostics|ExistingWatchDiscoveryDiagnostics)' -count=1)
(cd apps/backend && go test ./internal/github -run 'Test(ClassifyPRDiscoveryError|PRDiscovery)' -count=1)
git diff --check
```

## Files likely touched

- `apps/backend/internal/orchestrator/event_handlers_github.go`
- `apps/backend/internal/orchestrator/event_handlers_github_push_diagnostics_test.go (new)`
- `apps/backend/internal/orchestrator/event_handlers_github_test.go (mock seam if needed)`
- `apps/backend/internal/github/service_pr_discovery_health.go`
- `apps/backend/internal/github/service_pr_discovery_health_test.go`

## Dependencies

None. Execute in the plan order by default.

## Risks

Error and empty results can alternate. Historical errors must not override later successful outcomes.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/integrations/requirements/github-pr-discovery-health.md)
- [System design](../../specs/integrations/system-design/github-pr-discovery-health.md)
- [Plan evidence and exclusions](plan.md)
- Backend AGENTS.md and TDD backend test guidance.

## Results

Implemented a bounded exported GitHub discovery classifier and separated
successful empty lookups, provider failures, found PRs, and cancellation in
post-push and existing-watch paths. Failed provider attempts now log only the
workspace, task, repository identity, branch, attempt, and safe category.
Empty attempts and retry exhaustion are debug logs with outcome counts. The
existing association and watch reuse paths remain unchanged after a found PR.

The first regression run failed at compile time because the retry wait seam was
not yet present. After implementation, verification passed:

```text
go test ./internal/orchestrator -run 'Test(PushDiscoveryDiagnostics|ExistingWatchDiscoveryDiagnostics|DetectPushAndAssociatePR|GitHubPushAssociation|PushAssociation|ResolvePRWatchBranchForWatch)' -count=1
go test -race ./internal/orchestrator -run 'Test(PushDiscoveryDiagnostics|ExistingWatchDiscoveryDiagnostics)' -count=1
go test ./internal/github -run 'Test(ClassifyPRDiscoveryError|PRDiscovery)' -count=1
git diff --check
```

## Review remediation

PR discovery now uses the caller context state to decide whether to stop. A
live caller treats wrapped provider deadlines and cancellations as failed
attempts, so post-push retries and existing-watch diagnostics remain active.
Focused coverage covers wrapped provider deadlines in both paths and retains
actual caller-cancellation coverage. Builds and tests were not rerun for this
review remediation, so the verification above describes the pre-remediation
implementation state at that time.

## PR fixup verification

The focused orchestrator and GitHub regression commands passed after the review
fixes, and the base-relative backend lint completed with zero issues:

```text
go test ./internal/orchestrator -run 'Test(PushDiscoveryDiagnostics|ExistingWatchDiscoveryDiagnostics|DetectPushAndAssociatePR|GitHubPushAssociation|PushAssociation|ResolvePRWatchBranchForWatch)' -count=1
go test ./internal/github -run 'Test(ClassifyPRDiscoveryError|PRDiscovery)' -count=1
golangci-lint run ./... --new-from-rev='195530e5766abf7f44f08e2d5ad436b22c28d748' --timeout=5m
```

The full backend build and test suites remain unrun locally.
