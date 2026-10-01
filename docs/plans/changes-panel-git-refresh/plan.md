---
created: 2026-09-30
status: implemented
requirements:
  - REQ-PLATFORM-WORKSPACE-GIT-STATUS-001
system_design:
  - ../../specs/platform/system-design/workspace-git-status.md
legacy_specs: []
---

# Implementation Plan: Changes panel Git refresh

## Overview

Publish complete file membership before enrichment, and make accepted tracker work recoverable after caller timeout.
Implement six sequential work orders in the primary session. The user explicitly authorized implementation on 2026-09-30. No delegation is planned because the orders are sequential and share contracts.
Work Orders 01 through 06 are complete. The requirement and system design are current, and the publication ADR is accepted.

## Confirmed failure chain

Source checkout: `d05b0a91c80a44f385473fb31011946c61ef137c`.
This is the inspected source, not the original macOS running build SHA.

1. `computeGitStatus` collects file status but waits for sequential per-file enrichment and branch totals before returning.
2. Backend source probing uses one two-second deadline and `GetGitStatusMultiFresh`.
3. An eligible execution sets `live=true` before its query succeeds. Failure returns no messages and deliberately blocks database fallback.
4. `getGitStatusClass` owns detached singleflight work but does not cache or publish its success.
5. Only explicit update/attach paths write the cache. Monitor and Git poll updates depend on changed state.
6. `ChangesPanelTimeline` uses `hasAnything` without membership readiness and renders the normal empty message for missing status.

The portable diagnostic used Go overlays outside this checkout and a disposable linked Git worktree.
It created staged-only, unstaged-only, mixed, and untracked paths.
A channel gate stopped the real diff-enrichment entry after status parsing.
A controlled context then reported `DeadlineExceeded`, without any host-speed threshold.
After release, real computation returned four paths and both mixed facets.
The cached timestamp remained zero, cached files remained zero, and subscriber events remained zero.
The diagnostic passed because it asserted the current defect. Implementation must invert these assertions into regressions.
The gate only changed scheduling. It did not replace Git parsing or diff calculation with fixture output.

Stream attach can also synthesize a timestamp for a zero cache, and diff auto-close treats absent content as file removal.
These are source-confirmed hazards, not separately reproduced live-browser incidents.
The original macOS latency and contention samples remain supplied evidence, not measurements from this environment.
No developer instance, database, private repository, or original evidence file was used.

## Scope

### In scope

- Existing [requirement](../../specs/platform/requirements/workspace-git-status.md), updated criterion `.2`, and criteria `.19` through `.32`.
- Tracker-owned basic/enriched publication, state validation, bounded jobs, cancellation, cache replay, and stream attach.
- HTTP/runtime/WebSocket quality and ordering fields, multi-repository failures, and source replacement guards.
- Shared environment state, pending/error UI, pending diff membership, mobile parity, localization, and focused browser evidence.

### Out of scope

- Larger default request timeout, aggressive polling, unlimited diff concurrency, and full live-diff persistence.
- New Git library, hunk staging, unrelated conflict parsing, executor redesign, runtime toggles, commits, push, or PR creation.

## Technical approach

Use the [system design](../../specs/platform/system-design/workspace-git-status.md) as the technical authority.
The [accepted ADR](../../decisions/2026-09-30-progressive-workspace-git-refresh.md) records publication ownership and alternatives.
Task 01 establishes complete basic snapshots and compatible phase fields.
Task 02 adds ordered, fenced publication and bounded asynchronous enrichment.
Task 03 completes the environment-safe delivery and correlated recovery path.
Task 04 connects the state model to desktop/mobile surfaces and translations.
Task 05 proves the integrated behavior with isolated browser fixtures and documents delivered recovery.
Intermediate work orders are validation boundaries, not separately shippable releases.

Keep `fresh` admission interactive. Enrichment uses the existing bounded background pool.
Do not hold `updateMu` while waiting for enrichment.
The job worker retains one running job and one latest-only pending replacement.
Use exact index and comparison identity plus worktree evidence. Equal HEAD alone is insufficient.
Every accepted result goes through one publication function. Callers cannot independently overwrite cache state.
Detailed snapshot/Review/base-capture callers use the bounded `details=wait` mode. The normal Changes live probe remains two seconds. Foreground recovery is finite and uses a correlated response, not an interval loop.

### Compatibility matrix

| Boundary                                             | Identity                                                           | Behavior and proof                                                                                                    |
| ---------------------------------------------------- | ------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------- |
| Single repository / linked worktree                  | Environment, canonical workdir, Git directory/index, tracker epoch | Complete root membership, index lifetime cleanup. Process tests.                                                      |
| Multi-repository / initialized submodule             | Same environment plus repository scope                             | Healthy and failed entries coexist. No synthetic root status. API/backend/browser tests.                              |
| Sibling sessions / sibling tasks sharing environment | Canonical environment binding, requested session as route          | One accepted repository state. Backend/store tests.                                                                   |
| Replaced execution or workspace                      | Captured execution/stream generation and validated source epoch    | Reject delayed predecessor callbacks and responses. Lifecycle/WS tests.                                               |
| Local, container, SSH, Kubernetes, remote agentctl   | Existing shared workspace HTTP/stream contract                     | Same producer/consumer fields. No executor-specific transport change. Local transport tests exercise shared contract. |
| Legacy detailed / compact persisted payload          | Existing environment/repository selector, explicit data quality    | Detailed data stays readable. Compact rows remain summary-only. Materialized-status/store tests.                      |
| Missing identity / invalid repository                | No authoritative candidate                                         | Unavailable state, no outer-repository traversal, no persisted live-failure fallback.                                 |

No real remote/container executor run is claimed by the local test plan.

## ASCII UI preview

### UI-01: Changes status (desktop)

Entry: task Changes panel. The existing toolbar remains above the single scroll body.

```text
Changes                                      [Refresh]
Loading:       Checking changed files...
Unavailable:   Git status unavailable.        [Retry]
Prior data:    Refresh failed. Showing last observed changes. [Retry]
Dirty:         Changed files are ready. Diffs are loading.
  Unstaged
    src/a.ts   Modified                       Diff pending
  Staged
    src/b.ts   Modified                       Diff pending
Clean:         Your changed files will appear here
```

### UI-02: Changes status (phone)

Entry: task bottom navigation > Changes. Reuse the shipped mobile Changes body and full-height diff drawer.

```text
< Task                 Changes
Git status unavailable.
[ Retry (touch target) ]
Unstaged
  src/a.ts  Modified
  Diff pending
-----------------------------
[Sessions] [Files] [Terminal] [Changes]

< Back        src/a.ts
Diff is loading...
```

The status row scrolls with the existing body. Phone navigation and drawer controls retain their safe-area behavior.
Retry is 44px on touch surfaces and 28px on desktop. Each surface has one vertical scroll owner.
The clean line appears only after complete successful empty membership, with no independent PR or commit content.
Row selection opens a pending placeholder and remains open until authoritative membership removes that repository/path/layer.
These structures satisfy criteria `.23` through `.30`. Copy and spacing are illustrative and must use localization and existing primitives.

## Tests

Use `/tdd` for implementation. Run each new regression red before changing its production path.
Use barriers, controlled contexts, or existing test seams, not sleeps or wall-clock latency assertions.
Keep repository mutation in disposable fixtures, disable hooks/signing, and clean up all resources.

| Criteria                          | Evidence and named regression                                                                                                                                                                                                                                             |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `.1` through `.5`, `.19`, `.20`   | `workspace_git_status_progressive_test.go`: `TestWorkspaceTrackerDirtyFilesVisibleBeforeEnrichment`, `TestWorkspaceTrackerExpiredCallerStillPublishes`, `TestWorkspaceTrackerUnchangedDirtyReplay`. Existing concurrency tests remain.                                    |
| `.6` through `.18`, `.31`         | Existing diff-budget, mixed-facet, untracked-exclusion, comparison, command/admission, and stream suites. Add staged/unstaged/mixed/untracked matrix and symlink/submodule/rename fixtures.                                                                               |
| `.21`, `.22`, `.27`, `.31`        | `workspace_git_status_publication_test.go`: `TestWorkspaceTrackerOlderObservationCannotReplaceNewer`, `TestWorkspaceTrackerRejectsChangedEnrichment`, `TestWorkspaceTrackerEnrichmentDeduplicatesAndDrains`. Gate HEAD, index, content, retarget, and Stop independently. |
| `.20`, `.26`, `.28`, `.32`        | API fresh-concurrency, backend helper/materialized tests, gateway/lifecycle delivery tests: `TestGitRefreshSnapshotResponseRecoversDroppedNotification`, `TestGitRefreshPreservesHealthyRepositories`, `TestGitRefreshRejectsReplacedSource`.                             |
| `.21`, `.23` through `.28`, `.32` | `git-status-state.test.ts`, existing normalizer/multi-repo/WS tests, derived-status and hook tests. Deferred responses prove strict epoch/revision rejection, summary-only membership, prior-data preservation, and one finite recovery attempt.                          |
| `.24`, `.29`, `.30`               | `task-changes-panel.test.ts`, Review projection and editor consumers, mobile domain tests. Pending/failed diffs survive selection. Localization checks and rendered desktop/mobile E2E prove UI.                                                                          |

A slow basic observer tests timeout recovery separately from a slow enrichment gate.
Channel ownership assertions cover shutdown and deduplication. Subprocess tests prove admission and cleanup instead of only inspecting counters.
Test same-size edits with restored mtime, index replacement, HEAD moves, comparison-only changes, and stale stream callbacks.
No mutation is needed after an intentionally missed initial snapshot.

## E2E tests

Create `apps/web/e2e/tests/git/changes-panel-refresh-recovery.spec.ts` for `chromium`.
Create `apps/web/e2e/tests/git/mobile-changes-panel-refresh-recovery.spec.ts` for `mobile-chrome`.
Use the isolated backend fixture and disposable repository helpers.
Use existing WS routing and the fixture Git shim as controlled delivery/command boundaries.
A new barrier helper can gate matching diff commands without adding a production delay flag.
Do not copy the legacy helper that deletes active index locks.

Prove dirty rows before gate release, pending selected diff, completion without further Git edits, missed initial notification recovery, and reload/reconnect.
Prove loading and unavailable versus successful clean, existing good data after failure, and healthy data beside a failed repository.
Prove sibling environment scoping and workspace replacement.
Use causal waits armed before actions. Assert bounded request counts and terminal UI, not elapsed completion times.
Inspect desktop and phone screenshots. Assert phone Retry hitbox, Back/dismiss, one scroller, safe-area containment, and no document horizontal overflow.

## Work orders

- [x] [Task 01: Complete basic status before enrichment](task-01-basic-status.md)
- [x] [Task 02: Fence and bound tracker publication](task-02-ordered-enrichment.md)
- [x] [Task 03: Deliver and recover scoped snapshots](task-03-refresh-delivery.md)
- [x] [Task 04: Present truthful desktop and mobile states](task-04-status-ui.md)
- [x] [Task 05: Prove integrated recovery and document behavior](task-05-browser-proof.md) (done)
- [x] [Task 06: Fix review findings in snapshot safety and recovery](task-06-review-remediation.md) (done)

## Investigation verification results

Planning investigation only:

- Disposable diagnostic: `go test -overlay=<temporary-overlay> -run '^TestDiagnosticDirtyStatusDeliveryGap$' -count=1 -v ./internal/agentctl/server/process`. Passed.
- `go test -tags fts5 ./internal/backendapp -run 'Test(AppendLiveGitStatusMessage|TryGetLiveGitStatus)' -count=1`. Passed.
- `go test -race ./internal/agentctl/server/process -run 'TestWorkspaceTracker(ConcurrentFreshStatus|StatusWaiterCancellation|FreshStatusDoesNotJoinBackground|Stop.*SharedObservation|SharedObservationDeadline)' -count=1`. Passed.
- `python3 scripts/list-docs.py validate`: passed (334 decisions, 1262 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- Local `validateCoverage` from `.github/scripts/pr-docs.cjs`: covered, five work orders, no errors. Input included the proposed source change to exercise coverage rather than documentation-only exemption.
- `git diff --check -- docs/specs docs/decisions docs/plans/changes-panel-git-refresh`: passed.
- Diagnostic repeated with `-race`: passed. Temporary overlays, diagnostic repositories, and authoring scripts were removed.
- Source-path audit: all listed production files exist. Only explicitly planned regression test files are new.

No UI, i18n, or browser implementation validation is claimed yet.

## Task 01 execution results

The new `TestWorkspaceTrackerDirtyFilesVisibleBeforeEnrichment` regression was first run against the baseline and failed because the status payload lacked phase quality fields while already containing enriched diffs. It now passes with complete staged-only, unstaged-only, mixed, and untracked membership, `detail_state=pending`, and no diff commands in the basic observation. API, lifecycle event, backend client, frontend store-adapter, and type-boundary tests confirm that quality survives each implemented projection.

- `go test ./internal/agentctl/server/process ./internal/agentctl/server/api -run 'Test(WorkspaceTracker|GitStatus|ApplyPorcelain|UnquoteGitPath|.*Diff.*|.*Untracked.*|.*Symlink.*)' -count=1`: passed.
- `go test ./internal/agentctl/server/api -run '^TestHandleGitStatusMulti_SingleRepoUsesEmptyRepositoryName$' -count=1`: passed.
- `go test ./internal/agent/runtime/agentctl ./internal/agentctl/types/... -count=1`: passed.
- `go test ./internal/agent/runtime/lifecycle -run '^TestPublishGitStatus_PropagatesRepositoryName$' -count=1`: passed.
- `go test ./internal/backendapp -run '^TestBuildGitStatusNotificationIncludesAncestryEvidence$' -count=1`: passed.
- `pnpm exec vitest run lib/ws/handlers/git-status.test.ts`: passed (14 tests).
- `pnpm run typecheck`: passed.
- `git diff --check`: passed.

## Task 02 execution results

The ordered enrichment worker now publishes only validated snapshots, retains one running and one latest-only pending job, and drains pinned index snapshots on Stop. Basic and enriched status share the tracker publication sequence, while unchanged observations deduplicate enrichment. A cache-miss API response returns complete file membership with pending detail; the detail worker's identity validation is not counted as a second primary membership observation.

- `(cd apps/backend && go test -race ./internal/agentctl/server/process ./internal/agentctl/server/api -count=1)`: passed; process 130.276s, API 61.569s.
- `(cd apps/backend && go test -race ./internal/common/subproc -count=1)`: passed; 8.109s.
- `(cd apps/backend && golangci-lint run ./internal/agentctl/server/process ./internal/agentctl/server/api)`: passed; 0 issues.
- Focused progressive publication, changed-evidence fencing, shutdown, and stream regressions: passed.
- `git diff --check`: passed.

## Task 03 execution results

Fresh snapshot delivery, response recovery, source replacement fencing, and detailed snapshot consumers are implemented across the runtime and gateway path. Full checks exposed and fixed two API errors: cancellation now retains a distinct `status_canceled` code, and failed repository lookup is surfaced as a scoped `repository_unavailable` result without returning filesystem details. Healthy repositories remain available beside failed entries.

- `(cd apps/backend && go test -race -tags fts5 ./internal/backendapp ./internal/agent/runtime/lifecycle ./internal/gateway/websocket ./internal/orchestrator -count=1)`: passed.
- `(cd apps/backend && go test ./internal/agent/runtime/agentctl ./internal/agentctl/server/api ./internal/agent/handlers -count=1)`: passed.
- `(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator/executor -count=1)`: passed.
- Focused API cancellation and invalid-repository regressions: passed.
- `git diff --check`: included in final integrated verification.

## Risks

- The API now returns basic membership before full details. Every copied DTO and consumer must carry quality metadata.
- Pinning a linked-worktree index through enrichment changes cleanup lifetime. Cancellation and supersession must remove each owned temporary index.
- Filesystem observation is not an atomic transaction with external Git/editing processes. Reject changed evidence and show pending/unavailable details under continued churn.
- Snapshot epochs are source lifetimes, not sortable IDs across sibling executions. Canonical source validation must fence replacement.
- Non-blocking stream queues can drop frames. Correlated response/replay is required, not merely another broadcast.
- Background contention can delay enrichment. This investigation did not measure the original agentctl pool, authentication-protected diagnostics, or host contention.
- Compact snapshot persistence must preserve unknown detail state instead of publishing provisional zero totals as fresh.

## Existing companion packages

The scalability, dependency-tree exclusion, mixed staged/unstaged, fork comparison, and non-interactive Git packages are completed historical records.
This repair replaces scalability's fresh-read non-publication rule explicitly, without reopening those packages or changing their recorded results.
During implementation, Tasks source-precedence/quality prose was reconciled with the accepted Platform contract. Public recovery guidance is recorded in `docs/public/sessions-and-review.md`.

## Final integrated results

- Desktop recovery E2E passed (1 test): complete dirty membership appeared while enrichment was gated, and a dropped status notification recovered through the correlated refresh response without a Git mutation.
- Mobile recovery E2E passed (1 test): unavailable state, touch-sized Retry, response recovery, selected pending diff, full-height drawer bounds, and horizontal overflow were verified. The first full-height assertion exposed the shared drawer's 80vh cap (581.6 CSS px at a 727 px viewport); the drawer now uses dynamic viewport height and the final assertion passes.
- Existing mobile staged-file diff-sheet regression passed (1 test). Desktop and mobile capture-mode screenshots were inspected; generated `apps/web/.pr-assets` output was removed.
- Frontend unit suite passed (19 files, 188 tests), typecheck and lint passed, and i18n generation/check/ratchet passed across all six locales.
- Backend race tests, focused package tests, and `golangci-lint` passed as recorded in Work Orders 01 through 03. The E2E runner built the backend, Vite assets, and fixture plugin.
- `python3 scripts/list-docs.py validate`: passed (334 decisions, 1262 specifications); `python3 scripts/lint-spec-files.py --all`: passed; public-doc tests passed (62), and public-doc validation passed (47 pages).
- ADR discovery and final `git diff --check`: passed. The implementation and documentation are tracked in PR #4087, with current CI and review state linked there.

## Task 06 review remediation results

All eight core review findings were addressed with deterministic regressions and contract updates. Both backend race suites passed, as did the affected-package Go lint and backend build. The documented frontend suite passed (13 files, 156 tests), followed by 15 affected files and 164 tests; typecheck, ESLint, production Vite build, i18n checks/ratchet, desktop/mobile recovery E2E, and the multi-repository source-attachment E2E passed. Documentation validation and final diff checks passed. See [Task 06](task-06-review-remediation.md) for exact command results.
