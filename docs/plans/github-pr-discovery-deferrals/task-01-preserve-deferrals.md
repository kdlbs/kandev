---
id: 01-preserve-deferrals
title: Preserve discovery deferrals and omit expired retry text
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
  - REQ-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001
acceptance_criteria:
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.5
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.6
  - AC-INTEGRATIONS-GITHUB-PR-POLLING-002.7
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.3
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.6
  - AC-INTEGRATIONS-GITHUB-PR-DISCOVERY-HEALTH-001.8
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
---

# Task 01: Preserve discovery deferrals and omit expired retry text

## Summary

Make two small changes: preserve "try later" through fork-parent lookup, and
show retry text only for strict RFC3339 deadlines in the future. Reuse the
existing scheduler, immutable batch target, strict timestamp parser, and shared
desktop and phone warning.

## In scope

- Apply the [plan's deferral propagation and visibility approach](plan.md#technical-approach).
- Preserve every consumer of a deferred target, including joined attempts.
- Fan out a discovered parent PR to duplicate fork-branch watches after the representative is promoted.
- Reject normalized-invalid, partial, and timezone-less retry timestamps.
- Add the backend, component, and browser regressions named in the plan.
- Reuse existing admission, continuation, lifecycle guards, copy, and UI surfaces.

Keep production edits in `service_pr_watch_batched.go` and
`github-pr-discovery-health.tsx`. Extend the existing test files and browser
scenarios. The current scheduler, rate coordinator, and health store remain the owners.

## Out of scope

New schedulers, public fields, status categories, timers, controls, localization
keys, schema changes, live-data resets, deployment, commit, push, and PR creation.

## Acceptance

1. A wrapped local deferral preserves prior health and `last_checked_at`.
   Every consumer remains pending. Completed siblings still publish.
   A deferral records neither success nor failure.
2. The existing continuation path performs the parent lookup at its admission
   boundary, even for an idle watch. Genuine errors retain their failure handling.
   Existing retirement and credential fences still reject obsolete work.
3. Desktop and phone omit only the retry line for expired, equal, invalid, or
   missing deadlines. Future deadlines show it. Real failure warnings persist until success.
   A deadline passing never clears a failure by itself.
4. When multiple tasks watch the same fork branch, one successful parent lookup
   promotes every watch and creates each task association.

## ASCII UI preview

UI-01, [full preview](plan.md#ascii-ui-preview). Both the inline warning and
the existing phone limits drawer apply HEALTH-001.3, .6, and .8:

```text
Expired deadline:              Future deadline:
PR discovery failed            PR discovery failed
Reason: Unavailable            Reason: Unavailable
Last failure: 18 minutes ago   Last failure: 18 minutes ago
Affected targets: 7            Affected targets: 7
                              [existing localized retry line]
```

The missing expired line is required. Example values and spacing are illustrative.
Keep desktop hover/focus details and the phone touch drawer from `GitHubAccessHelp`.
Use the same warning component and existing refresh-driven renders on both devices.

## Verification

Run commands from the repository root. Before production edits, add and run
`TestSyncWatchesBatched_ForkParentDeferralPreservesHealth` and the expired-deadline
component test. Record their behavioral failures. Use virtual time for continuation.

```bash
(cd apps/backend && go test -tags fts5 ./internal/github -run '^TestSyncWatchesBatched_ForkParentDeferralPreservesHealth$' -count=1 -v)
```

After the correction, run the following commands sequentially. In a fresh
worktree, install dependencies once with `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/backend && go test -tags fts5 -race ./internal/github -run 'Test(SyncWatchesBatched_|PRMonitorContinuation_|PRDiscovery|PRStatusProgress_)' -count=1 -timeout=8m)
(cd apps/web && pnpm exec vitest run components/github/github-rate-limit.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/github/github-pr-discovery-health.tsx)
(cd apps/web && pnpm e2e:run --project chromium tests/integrations/github-workspace-settings.spec.ts -- --grep 'PR discovery')
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts -- --grep 'PR discovery')
bash scripts/lint-go-changed apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_batched_budget_test.go apps/backend/internal/github/poller_pr_continuation_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/github-pr-discovery-deferrals
```

Use the managed E2E runner's fresh build. Assert each command discovers its
intended tests. Run the repository documentation-coverage preflight with the final
changed-file set. Record each result and the exact command if any path changes.

## Files likely touched

- `apps/backend/internal/github/service_pr_watch_batched.go`
- `apps/backend/internal/github/service_pr_watch_batched_budget_test.go`
- `apps/backend/internal/github/poller_pr_continuation_test.go`
- `apps/web/components/github/github-pr-discovery-health.tsx`
- `apps/web/components/github/github-rate-limit.test.tsx`
- `apps/web/e2e/tests/integrations/github-rate-limit-fixture.ts`
- `apps/web/e2e/tests/integrations/github-workspace-settings.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-github-workspace-settings.spec.ts`
- This work order and its sibling plan.
- `docs/public/integrations.md`, only if the final troubleshooting guidance needs correction.

## Dependencies

None. The existing discovery-health and PR-monitor continuation implementations
are present in this checkout. Re-read their current interfaces before implementation.

## Risks

Joined attempts must receive the typed error rather than a completed empty result.
A timestamp write before deferral can delay the parent lookup through idle admission.
The fixed September 2026 component fixture needs a deterministic clock.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/integrations/requirements/github-pr-discovery-health.md).
- [Failure, presentation, and continuation design](../../specs/integrations/system-design/github-pr-discovery-health.md).
- Existing fork-parent tests in `service_pr_watch_batched_budget_test.go`.
- Existing `TestPRMonitorContinuation_ResumesAtAdmissionBoundary` and lifecycle tests.
- Existing desktop disclosure and phone drawer PR-discovery scenarios.

## Results

Completed. The typed fork-parent deferral now reaches the existing pending-work
continuation with all duplicate consumers. The prior health and check timestamp
remain intact, successful siblings still publish, and genuine provider failures
keep their existing failed-health path. Successful fork-parent results use the
original batch target when shared with other consumers. The batched-branch key
folds owner and repository case to match GitHub identity while preserving branch
case. Sibling fan-out uses a one-entry PR status map; singleton groups skip that
map. The shared warning uses the strict RFC3339 parser and shows its retry line
only for valid future deadlines.

The required backend service, continuation, component, desktop, and phone
regressions failed before the production changes and passed afterward. Two
additional regressions reproduced the review findings before their fixes and
passed afterward. Verification:

The latest review correction also retains per-target retry boundaries when
several fork-parent lookups defer during one workspace batch. The added
continuation regression failed when the second target inherited the first
target's five-minute deadline, then passed after each group's deferral was
carried into pending work. Joined attempts retain their group's boundary too.
Desktop and phone assertions now require exactly one inline warning before
inspecting its contents.

- `(cd apps/backend && go test -tags fts5 ./internal/github -run '^TestPRMonitorContinuation_KeepsForkParentBoundariesPerTarget$' -count=1)`: failed before the correction because the second target inherited the first target's five-minute boundary; passed after, retaining its 20-minute boundary.
- `(cd apps/backend && go test -tags fts5 -race -trimpath ./internal/github -run 'Test(SyncWatchesBatched_|PRMonitorContinuation_|PRDiscovery|PRStatusProgress_)' -count=1 -timeout=8m)`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/github/poller.go apps/backend/internal/github/poller_pr_continuation.go apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_batched_budget_test.go apps/backend/internal/github/poller_pr_continuation_test.go`: passed with 0 issues.
- `(cd apps/web && pnpm run typecheck)`: passed; Oxlint passed for both changed E2E files.
- `(cd apps/web && pnpm e2e:run --no-build --project chromium tests/integrations/github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 desktop test passed against refreshed backend, fixture, plugin, and Vite assets.
- `(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 phone test passed against that build.
- `make -C apps/backend build build-e2e-delivery-fixture build-e2e-send-now-fixture` and `make -C apps/backend e2e-plugin-package`: passed.

- `cd apps/backend && go test -tags fts5 ./internal/github -run '^TestSyncWatchesBatched_ForkParentPRPromotesSharedConsumers$' -count=1 -v`: failed before the fix because the second watch remained searching and lacked a task association; passed after the fix.
- `cd apps/web && pnpm exec vitest run components/github/github-rate-limit.test.tsx`: failed before the parser change because a normalized-invalid future deadline displayed retry text; 3 tests passed after the correction.
- `(cd apps/backend && go test -tags fts5 -race ./internal/github -run 'Test(SyncWatchesBatched_|PRMonitorContinuation_|PRDiscovery|PRStatusProgress_)' -count=1 -timeout=8m)`: passed.
- `cd apps/backend && go test -tags fts5 -run '^TestSyncWatchesBatched_ForkParentPRPromotesSharedConsumers$' ./internal/github -count=1`: passed; both watches and task associations resolved after one fork-parent lookup.
- `cd apps/backend && go test -tags fts5 -run '^TestSyncWatchesBatched_ForkParentPRPromotesSharedConsumers$' ./internal/github`: the `Fork/r` case-variant sibling failed before key normalization and passed after the fix.
- `(cd apps/backend && go test -tags fts5 -race -trimpath ./internal/github -run 'Test(SyncWatchesBatched_|PRMonitorContinuation_|PRDiscovery|PRStatusProgress_)' -count=1 -timeout=8m)`: passed after the PR review follow-up.
- `bash scripts/lint-go-changed apps/backend/internal/github/graphql.go apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_batched_budget_test.go`: passed with 0 issues after the PR review follow-up.
- `(cd apps/web && pnpm run typecheck)`: passed after comparing the parser's nanoseconds to the current millisecond time without unsupported BigInt literal syntax.
- `(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/github/github-pr-discovery-health.tsx)`: passed.
- `(cd apps/web && pnpm e2e:run --project chromium tests/integrations/github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 desktop test passed; managed runner built backend and web app.
- `(cd apps/web && pnpm e2e:run --no-build --project mobile-chrome tests/integrations/mobile-github-workspace-settings.spec.ts -- --grep 'PR discovery')`: 1 phone test passed on the fresh build above.
- `python3 scripts/list-docs.py validate`: passed, 454 decisions and 2,021 specifications after the requirement and design update.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/github/service_pr_watch_batched.go apps/backend/internal/github/service_pr_watch_batched_budget_test.go`: passed with 0 issues after a concurrent shared pre-commit lint finished.
- `git diff --check`: passed after this results update.
- Documentation coverage preflight: passed for all changed files after this results update.

The regression command recorded in this work order was also run before the
implementation and reproduced the failure. This work order and implementation
are committed on the PR branch.
