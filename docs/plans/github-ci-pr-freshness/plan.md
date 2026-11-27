---
created: 2026-10-13
status: implemented
requirements:
  - REQ-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001
system_design:
  - ../../specs/integrations/system-design/github-ci-pr-freshness.md
legacy_specs: []
---

# Implementation Plan: GitHub CI PR freshness

## Overview

Bound the automatic forced-refresh loop reported in
[issue #4452](https://github.com/kdlbs/kandev/issues/4452) with the existing
30-second freshness predicate. One sequential work order supplies the regression
coverage and minimum correction.

## Evidence and assumptions

Source confirms that `handleTaskPRCIAutomationWithRefresh` computes freshness
but unconditionally refreshes when either CI switch is enabled. A fresh
`github.task_pr.updated` event therefore enters `TriggerPRSyncAll`, whose explicit
refresh bypasses freshness admission and invalidates feedback/workflow caches.
Auto-fix fetches feedback; its write-through can publish another semantic update.
The coordinator coalesces overlapping work but imposes no interval between runs.
REST review derivation counts COMMENTED reviews as pending; GraphQL derives
pending count from review requests. Alternating writes can sustain the cycle.

Scope assumption: implement the issue's freshness guard first. Review-policy
normalization remains a separate product decision. The reporter's measured rates
are external evidence, not a local performance result.

Temporary reproduction: `TestIssue4452TemporaryFreshStatusRepro` invoked the
real handler with fresh PR input and the existing mock service. Both `auto_fix`
and `auto_merge` failed with `fresh PR forced 1 sync calls; want 0` under
`go test -trimpath ./internal/orchestrator -run
'^TestIssue4452TemporaryFreshStatusRepro$' -count=1` from `apps/backend`. The
probe was removed after execution. No live GitHub traffic was generated.

## Scope

### In scope

- Freshness admission at the orchestrator handler boundary.
- Provider call-count and automation-effect regression tests.
- Preserve stale refresh, PR-scoped options, merge gates, partial-sync matching,
  option-save handling, and lifecycle errors.

### Out of scope

- Review-state and commit-status normalization, GraphQL `reviewDecision` changes.
- Rate-limit coordination, poll cadence, cache TTLs, explicit user refresh changes.
- UI, schema, configuration, and public API changes.

## Technical approach

Change the refresh conditional in
`apps/backend/internal/orchestrator/event_handlers_github_ci_automation.go`
to require `!freshlySynced`. Keep `freshlySynced` available for the existing
merge gate. Add regressions in a separate file to avoid growing the oversized
`event_handlers_github_ci_automation_test.go`. Adjust existing fixtures only
where they must enter the refresh branch to prove their intended behavior.

The guard applies to the shared GitHub service seam for PAT, App, and CLI-backed
clients. Handler mocks prove refresh admission independent of transport; they do
not establish authentication or live-provider performance. Noop/disabled clients
retain their existing behavior. Other providers are outside this handler.

## Tests

All new cases live in
`apps/backend/internal/orchestrator/event_handlers_github_ci_freshness_test.go`:

- `TestCIAutomationFreshStatusSkipsRefresh`: fresh auto-fix, auto-merge, and both
  switches, zero forced sync calls (AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.1).
- `TestCIAutomationFreshStatusContinuesAutomation`: actionable feedback queues
  one prompt; ready auto-merge uses the supplied expected head (AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.3-.4).
- `TestCIAutomationStaleStatusRefreshes`: old and missing timestamps, one sync,
  and evaluation of the returned matching state (AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.2-.4).
- `TestCIAutomationRefreshAdmissionPreservesScope`: no-refresh, disabled
  evaluated PR with enabled sibling, and enabled evaluated PR with disabled
  sibling (AC-INTEGRATIONS-GITHUB-CI-PR-FRESHNESS-001.2, .5).

Retain `TestCIAutomationHasFreshPRStatusAt`, stale auto-merge rejection, refresh
failure/lifecycle tests, and partial-sync matching. Set partial-sync input stale
while keeping its matching returned PR fresh, and assert one sync call so the
fixture cannot bypass its own subject.

## Work orders

- [x] [Task 01: Guard fresh CI PR refreshes](task-01-guard-fresh-refresh.md)

## Verification results

Design validation on 2026-10-13:

- `python3 scripts/list-docs.py validate`: passed (381 decisions, 1587 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check` and plan-scoped diff/status inspection: passed; all four
  artifacts remain untracked, unstaged, and uncommitted.
- Catalog lookup discovers both new integration specifications.
- Direct cross-reference inspection: requirement, all five acceptance IDs,
  paired design, and work-order-to-plan references match.
- Node-based `.github/scripts/pr-docs.cjs` coverage preflight could not run:
  this environment has no `node` on PATH. Rerun during implementation with a
  Node-equipped environment; specification lint is not a substitute.

Implementation validation on 2026-10-13:

- RED: the focused regression command failed five fresh-input cases with one
  forced sync instead of zero, before the production guard was added.
- `(cd apps/backend && go test -trimpath ./internal/orchestrator -run
  '^TestCIAutomation(FreshStatus|StaleStatus|RefreshAdmission)' -count=1)`:
  passed (four groups, 15 cases; package time 0.542s).
- `(cd apps/backend && go test -trimpath -race ./internal/orchestrator -count=1)`:
  passed (package time 206.892s), including refresh failure, freshness boundary,
  stale-merge rejection, options updates, and the corrected partial-sync fixture.
- `python3 scripts/list-docs.py validate`: passed (381 decisions, 1587 specs).
- `python3 scripts/lint-spec-files.test.py`: passed (36 tests).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `git diff --check`: passed; all changed Go files are gofmt-clean.
- The work-order Node preflight using
  `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`: passed,
  status `covered`, no errors. Node was installed but absent from PATH.

Implementation matches the paired requirements and design. The requirement is
active, the design current, and this work order complete. No live-provider
rate measurement was performed. Publication is tracked through the pull request.


## Risks

- The residual REST/GraphQL semantic disagreement remains; the fix bounds the
  forced-refresh loop without promising convergence of review state.
- Existing fresh fixtures may stop exercising refresh-specific error paths.
- Keep the existing merge expected-head guard when reusing a recent observation.

## Documentation impact

Internal specs and this plan define the correction. No public documentation
change is needed for this design package or the proposed internal admission
repair; commands, controls, polling promises, and operator configuration retain
their existing contracts.

## PR remediation scope

PR #4462 reviews identified partial feedback renewing status freshness and
provider feedback events reading an older cached snapshot. Preserve the previous
status-sync timestamp on feedback-only writes, invalidate it on head/base
changes, and invalidate the scoped feedback cache on provider feedback events.
Internal write-through events keep reusing cached feedback and fresh status.
This extends the same freshness contract (AC-001.6 and AC-001.7), with no schema
or public API change and no review-policy normalization.

New evidence belongs to `TestPRFeedbackPreservesStatusFreshness` (five cases),
`TestPRFeedbackEventReadsNewFeedbackWithoutStatusRefresh`, and
`TestAutomationFeedbackInvalidationPreservesSiblingCache`. The existing
cache-hit persistence test also gains an independent stored-title sentinel.
Review remediation validation:

- RED: `TestPRFeedbackPreservesStatusFreshness` failed all five cases because
  partial feedback renewed the stored and published timestamp.
- RED: `TestPRFeedbackEventReadsNewFeedbackWithoutStatusRefresh` failed with
  no queued prompt when the provider event reused an older cached response.
- Focused GitHub producer/cache tests and orchestrator freshness/event tests
  passed. Coverage now includes 23 new regression cases plus the strengthened
  existing cache-hit persistence check.
- `(cd apps/backend && go test -trimpath -race ./internal/github ./internal/orchestrator -count=1)`:
  passed. GitHub package: 98.231s; orchestrator package: 210.856s.
- `(cd apps/backend && golangci-lint run ./... --new-from-rev=7c340deb50847c9015e3be73998e78f921e4aec4 --timeout=5m)`:
  passed, zero issues.
- Documentation catalog, 36 specification-linter tests, full specification
  lint, documentation coverage over the full changed-file set, and diff
  checks passed.

The targeted cache invalidation preserves sibling PR cache entries. Credential
failure prevents dispatch from cached feedback. Partial feedback retains the
existing status-sync timestamp or clears it when the head/base changes; normal
status-sync writes continue advancing it. Both review findings are addressed
locally. Publication and exact-head CI/review evidence are tracked in PR #4462.
