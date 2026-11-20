---
created: 2026-11-16
status: done
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
legacy_specs: []
---

# Implementation plan: GitHub PR discovery deferrals

## Overview

Keep the fix to two small changes in one work order:

1. Carry "try later" from fork-parent lookup to the existing retry scheduler.
2. Show the retry line only for a valid future deadline in the shared warning.

Expected production edits are limited to `service_pr_watch_batched.go` and
`github-pr-discovery-health.tsx`, plus tests beside their existing coverage.

## Agreed mental model

A lookup can succeed, fail, or wait for local admission. A local wait preserves
the previous health state and the last completed-check time. The existing
scheduler retries the unfinished lookup. Its actual result then updates health.

A retry deadline determines when another attempt is eligible. The deadline
passing does not establish recovery. An expired deadline hides only the retry
line. An unresolved failure stays visible until successful discovery clears it.

The user confirmed this scope and requested a simple fix. Reuse the current
error type, scheduler, health store, and warning component.

## Evidence and requirement conformance

Read-only diagnostics from the running instance showed six fork-parent lookup
deferrals at 2026-11-16 19:29:05 UTC. Each error reported
`background provider admission deferred: background_pacing`.
The status endpoint reported seven failed targets and category `unavailable`.
Its latest failure matched those log entries. Its earliest retry deadline was
2026-11-14 15:19:02 UTC. Authentication remained active with available quota.
The captured log window does not establish the oldest target's original error.

`applyBatchedSearchingWatch` converts the wrapped deferral into `SyncFailed`
and advances `last_checked_at`. `finishSearchingBatchGroup` then records
`unavailable`. The typed error never reaches pending-work continuation.
The shared warning formats every supplied deadline, including expired values.

POLLING-002.5-.7 already require pending work, timely continuation, and current
authorization. This correction reuses those criteria. New HEALTH-001.8 defines
the small deadline-display improvement. HEALTH-001.3 and .6 preserve genuine
failure visibility and parity between desktop and phone.
No material product question remains.

## Scope

### In scope

- Preserve typed local deferrals from fork-parent lookup through batch outcomes.
- Preserve prior health and watch timestamps until discovery completes.
- Reuse the monitor's existing pending inventory and continuation timer.
- Omit expired, missing, and invalid retry text in the shared warning.
- Add focused backend, component, desktop, and phone regressions.

### Out of scope

- New retry infrastructure, status categories, controls, copy, dependencies, or flags.
- Changes to provider pacing, retry deadlines, idle intervals, or fallback budgets.
- Clearing old failures by age, quota refresh, or local deferral.
- Public wire changes, SQL migrations, credential changes, or live-instance repair.

## Technical approach

### Deferral propagation

In `apps/backend/internal/github/service_pr_watch_batched.go`, carry the typed
deferral in `PRWatchSyncResult` as an optional internal field. Use
`admissionDeferralOf` before the generic lookup-error branch. Return without
`applyPRWatchDiscoveryResult`, `SyncFailed`, or a failure log.

Have `applySearchingBatchGroups` return pending work and the typed deferral
through `prWatchSyncOutcome` to `runBatchedLeaderPhases`. Release each deferred
attempt and complete its shared result with the error. Include every consumer
of the deferred target. Joined callers must receive the same pending outcome.
Keep completed sibling results available for publication.

When a shared fork-parent lookup succeeds, key its status under the batch
group's original target. Applying the result promotes and mutates the
representative watch to the parent PR identity; other consumers still use the
original fork target while receiving the shared status.

Reuse `Poller.recordPRMonitorPending` and its existing retry-boundary rules.
A retry must perform the previously unfinished parent lookup. It must not
appear successful because a premature timestamp invokes the freshness shortcut.
Keep actual provider errors on their existing failure path.

Keep this correction in the existing batch service. The scheduler, rate
coordinator, and health store already provide the required behavior.

| Provider path | Intended behavior | Evidence |
| --- | --- | --- |
| GitHub CLI or direct HTTP with GraphQL | Preserve wrapped local deferrals from parent lookup | Service test with a successful branch batch and wrapped `AdmissionDeferredError` |
| Duplicate watches and joined attempts | All consumers remain pending after the shared attempt defers | Multi-consumer service regression |
| Unsupported GraphQL | Existing bounded fallback remains authoritative | Existing batch-budget and monitor tests |

Existing admission and credential ownership ADRs remain authoritative.

### Deadline visibility

In `apps/web/components/github/github-pr-discovery-health.tsx`, parse the wire
deadline with `parseStrictRfc3339Timestamp` and compare its nanoseconds with the
current render time before showing the retry line. Reuse existing localized
copy. Preserve all other warning content until newer successful discovery
clears the affected target. The backend health projection and its revision
semantics stay unchanged.

## ASCII UI preview

UI-01: Workspace settings > Integrations > GitHub. An unresolved failure has
an expired retry deadline. The inline warning has the same content on both devices.

```text
Before                         After
PR discovery failed            PR discovery failed
Reason: Unavailable            Reason: Unavailable
Last failure: 18 minutes ago   Last failure: 18 minutes ago
Affected targets: 7            Affected targets: 7
Retry after 2 days ago
```

A future valid deadline retains the existing retry line. A local deferral alone
does not create a warning. Existing genuine failures remain visible during deferral.
These states cover HEALTH-001.3, .6, .8 and POLLING-002.5.

The existing `GitHubAccessHelp` is the mobile exemplar: desktop hover/focus
opens details, while a phone tap opens the limits drawer. Both reuse the warning.
Keep the drawer's scroll owner, focus return, safe-area handling, and touch targets.
Spacing and example times are illustrative. This package changes one text condition.

## Tests

- `service_pr_watch_batched_budget_test.go`:
  `TestSyncWatchesBatched_ForkParentDeferralPreservesHealth` must fail before the fix.
  Cover healthy and degraded prior health, wrapped pacing and interactive-priority
  deferrals, two consumers, a joined attempt, and a genuine provider error control.
  Assert pending targets, unchanged failure evidence and timestamps, and completed siblings.
  `TestSyncWatchesBatched_ForkParentPRPromotesSharedConsumers` covers two tasks
  watching one fork branch. Assert the discovered parent PR is attached to both
  watches and both tasks after one shared lookup. This covers POLLING-002.5 and HEALTH-001.3.
- `poller_pr_continuation_test.go`:
  `TestPRMonitorContinuation_ForkParentDeferralResumes` uses virtual time and an idle watch.
  Assert the parent lookup succeeds at its admission boundary, before the next tick.
  `TestPRMonitorContinuation_KeepsForkParentBoundariesPerTarget` verifies two
  fork-parent deferrals on different resources retain their own retry deadlines.
  Reuse existing retirement and credential-change tests for POLLING-002.6-.7.
- `components/github/github-rate-limit.test.tsx`:
  freeze time and cover future, equal, expired, invalid, and missing deadlines.
  Include future-looking normalized-invalid, partial, and timezone-less strings.
  Invalid deadlines must hide only the retry line. This covers HEALTH-001.3 and .8.

## E2E tests

Extend the existing PR discovery scenarios in these files:

- `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`, project `chromium`.
- `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`, project `mobile-chrome`.

Use `github-rate-limit-fixture.ts` to supply an expired deadline with an unresolved
failure. Assert the inline warning and actual desktop disclosure or phone drawer
retain the failure without the retry line. Preserve the current recovery assertion.
Freeze browser time for future-deadline coverage. These scenarios cover HEALTH-001.3, .6, and .8.
Backend tests prove admission and continuation. Browser fixtures prove presentation only.
Extend these scenarios and their fixture instead of creating a new browser suite.

## Work orders

- [x] [Task 01: Preserve discovery deferrals and omit expired retry text](task-01-preserve-deferrals.md)

## Companion records and documentation impact

The completed `github-pr-discovery-health`, `watch-task-cleanup`, and
`github-sidebar-pr-freshness` packages retain their historical results.
This package closes a remaining fork-parent path in the existing design.
Internal requirements and design are updated. The implementation also clarified
`docs/public/integrations.md`: a retry deadline controls eligibility, while an
expired deadline does not clear an unresolved warning.

## Verification results

Implementation, review corrections, and verification are complete. Follow-up
regressions cover duplicate fork-parent promotion and strict retry timestamp parsing.

PR review follow-up aligned branch-result keys with GitHub's case-insensitive
owner and repository identity while preserving case-sensitive branch names.
Sibling fan-out now carries at most one shared PR status, and singleton groups
skip the shared status map. A later review correction carries each searching
and joined group's typed deferral through poller continuation so different
fork-parent targets keep their own retry deadlines.

- `python3 scripts/list-docs.py validate`: passed, 454 decisions and 2,021 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `(cd apps/backend && go test -tags fts5 -race ./internal/github -run 'Test(SyncWatchesBatched_|PRMonitorContinuation_|PRDiscovery|PRStatusProgress_)' -count=1 -timeout=8m)`: passed.
- `(cd apps/web && pnpm exec vitest run components/github/github-rate-limit.test.tsx)`: 3 tests passed.
- `(cd apps/web && pnpm run typecheck)`: passed.
- `(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/github/github-pr-discovery-health.tsx)`: passed.
- `(cd apps/web && pnpm e2e:run --project chromium tests/integrations/github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 desktop test passed; the managed runner built the backend and web app.
- `(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 phone test passed using that fresh build.
- `bash scripts/lint-go-changed apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_batched_budget_test.go apps/backend/internal/github/poller_pr_continuation_test.go`: passed with 0 issues. The first run found two QF1008 suggestions in the continuation test; the selectors were simplified and lint passed on rerun.
- `(cd apps/backend && go test -tags fts5 -run '^TestSyncWatchesBatched_ForkParentPRPromotesSharedConsumers$' ./internal/github)`: the case-variant `Fork/r` sibling failed before key normalization and passed after the fix.
- `(cd apps/backend && go test -tags fts5 -race -trimpath ./internal/github -run 'Test(SyncWatchesBatched_|PRMonitorContinuation_|PRDiscovery|PRStatusProgress_)' -count=1 -timeout=8m)`: passed after the PR review follow-up.
- `bash scripts/lint-go-changed apps/backend/internal/github/graphql.go apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_batched_budget_test.go`: passed with 0 issues after the PR review follow-up.
- `(cd apps/backend && go test -tags fts5 ./internal/github -run '^TestSyncWatchesBatched_ForkParentPRPromotesSharedConsumers$' -count=1 -v)`: failed before the shared-target-key correction because the second watch stayed searching without a task association; passed after correction.
- `(cd apps/backend && go test -tags fts5 -race ./internal/github -run 'Test(SyncWatchesBatched_|PRMonitorContinuation_|PRDiscovery|PRStatusProgress_)' -count=1 -timeout=8m)`: passed after the backend correction.
- `(cd apps/web && pnpm exec vitest run components/github/github-rate-limit.test.tsx)`: failed before strict parsing because a normalized-invalid future deadline displayed retry text; 3 tests passed after correction, including partial and timezone-less timestamps.
- `(cd apps/web && pnpm run typecheck)`: passed after using the strict parser's nanoseconds and a target-compatible `BigInt(...)` millisecond conversion.
- `(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/github/github-pr-discovery-health.tsx)`: passed.
- `(cd apps/web && pnpm e2e:run --project chromium tests/integrations/github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 desktop test passed after a fresh backend and web build.
- `(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 phone test passed against that build.
- `bash scripts/lint-go-changed apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_batched_budget_test.go`: passed with 0 issues after a prior concurrent pre-commit lint finished.
- `git diff --check`: passed after final status and results updates.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: passed for the implementation, specs, public docs, plan, and work order. This local preflight did not contact GitHub.
- `(cd apps/backend && go test -tags fts5 ./internal/github -run '^TestPRMonitorContinuation_KeepsForkParentBoundariesPerTarget$' -count=1)`: reproduced the second target inheriting the first five-minute deadline before the correction; passed afterward with its 20-minute boundary preserved.
- `(cd apps/backend && go test -tags fts5 -race -trimpath ./internal/github -run 'Test(SyncWatchesBatched_|PRMonitorContinuation_|PRDiscovery|PRStatusProgress_)' -count=1 -timeout=8m)`: passed with per-target deferrals included.
- `bash scripts/lint-go-changed apps/backend/internal/github/poller.go apps/backend/internal/github/poller_pr_continuation.go apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_batched_budget_test.go apps/backend/internal/github/poller_pr_continuation_test.go`: passed with 0 issues.
- `(cd apps/web && pnpm run typecheck)`: passed; focused Oxlint checks on both changed E2E files also passed.
- `(cd apps/web && pnpm e2e:run --no-build --project chromium tests/integrations/github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 desktop test passed against refreshed backend, fixture, plugin, and Vite assets.
- `(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 phone test passed against that build.
- `make -C apps/backend build build-e2e-delivery-fixture build-e2e-send-now-fixture` and `make -C apps/backend e2e-plugin-package`: passed.

The required regression tests were first run before the fix and failed on the
lost deferral, missing continuation, and expired retry line. They passed after
the implementation. The implementation and plan package are committed on the
PR branch.

No instance settings or live data changed.

## Risks

- Discarding the typed error again can suppress continuation for joined consumers.
- Advancing `last_checked_at` can make an idle target wait another 30 minutes.
- Clearing prior health on deferral can conceal an actual provider failure.
- Deadline visibility updates on existing renders, without a dedicated countdown timer.
