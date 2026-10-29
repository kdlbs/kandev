---
created: 2026-10-28
status: complete
requirements:
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-001
  - REQ-INTEGRATIONS-GITHUB-PR-POLLING-002
  - REQ-INTEGRATIONS-GITHUB-RATE-002
system_design:
  - ../../specs/integrations/system-design/github-pr-discovery-health.md
  - ../../specs/integrations/system-design/github-rate-limit-coordination.md
legacy_specs: []
---

# Implementation Plan: Automatic sidebar PR freshness

## Overview

Repair background PR monitoring so inactive task rows receive current status
without task selection. First separate completed status publication from deferred
discovery. Then resume pending work at its admission boundary. Finally prove
delivery through task summaries and the existing desktop and phone rows.

The design package is implemented. All three work orders are done and record
their own results.

## Evidence and assumption check

The user reported that sidebar PR icons update after selecting each task.
The investigation used read-only state from the running instance on 2026-10-28.
At 14:28:35 UTC, PR #4733 had no successful check for 31 minutes, 41 seconds.
PR #4818 was 17 minutes, 26 seconds old. Both watches remained active.

Logs contained repeated `background provider admission deferred: background_pacing`
errors in the workspace batch and per-target fallback paths.
Metrics also recorded GraphQL and Core pacing deferrals.
The monitor goroutine remained alive at its normal ticker select.
Stored task summaries agreed with the stale provider records.
This evidence locates the defect before sidebar rendering.

The temporary `TestReproSidebarPRBackgroundPacing` used real `RateAdmission`,
fake provider responses, SQLite, and `testing/synctest`.
Each of five virtual poll cycles added a searching watch.
Each cycle fetched fresh numbered status, then deferred branch discovery for
one second. The stored PR stayed pending through all five cycles.
An interactive sync then stored success immediately.
The temporary test and diagnostic archive were removed.

The investigation checkout was `4222c3654f9bb1efd1a7d27b5151c4b4c4f1ffd2`.
The running build was `fb4c2bc3ef98da2fc303fe05cb144bcd6c167d58`.
The relevant batching, pacing, and monitoring files matched between those commits.
These snapshots are evidence, not implementation dependencies.

Confirmed intent is automatic recovery under the existing provider admission rules.
Integrations owns synchronization and its visible status outcome.
Platform retains task-summary delivery ownership. UI retains shared icon presentation.
No material product question blocks the package.

## Requirement conformance

Existing POLLING-001.3 defines the one-minute cadence for known PRs.
Existing POLLING-002.1-.4 define bounded fallback and preservation during deferral.
RATE-002.2, .3, and .7 preserve admission, interactive priority, and eventual progress.

The discovery requirement now adds POLLING-001.6-.8 and POLLING-002.5-.7.
These criteria cover independent publication, search churn, inactive rows,
local deferral, timely continuation, and current authorization.
The paired requirement and design keep their existing draft status.
Do not promote the entire discovery capability because this repair finishes.

The accepted rate-coordination and watch-retirement ADRs remain authoritative.
No new ADR is required. The correction fits their existing boundaries.

## Scope

### In scope

- Publish complete known-PR status independently of deferred branch discovery.
- Preserve phase progress across unrelated searching-target changes.
- Retain and resume unfinished monitoring work without another fixed minute.
- Classify local admission separately from provider and transport failures.
- Preserve transaction, lifetime, credential, and explicit-refresh fences.
- Prove task-summary publication and inactive desktop/phone row updates.

### Out of scope

- New UI, copy, controls, settings, public APIs, schema, or feature flags.
- Removing pacing, quota reserve, interactive priority, or provider retry windows.
- Changing one-minute monitoring or 15/30-minute idle discovery intervals.
- Changing Actions cache expiry, enrichment budget, or merge readiness rules.
- Replacing the scheduler or adding per-row/per-watch polling goroutines.
- General REST pagination continuation, other providers, and orphan-sweep redesign.
- Live-instance repair, restarts, deployment, commit, push, or PR creation.

## Technical approach

Follow the discovery design's independent publication, admission continuation,
and sidebar delivery sections.

Task 01 changes `service_pr_watch_batched.go` and `batch_progress.go`.
Separate numbered and searching phase progress and expose completed results with
unfinished groups through one internal outcome. Apply complete numbered results
before the searching phase. Preserve per-status review-thread completeness.
Adapt all batch callers in `poller.go` and `service_pr_watch.go` so error handling
cannot discard successful writes or repeat completed targets through fallback.
Keep HTTP and WebSocket response envelopes unchanged.

Task 02 extends `prMonitorLoop` with a bounded pending inventory and one timer.
Use the existing `AdmissionDeferredError` retry boundary and current admission.
Retain unfinished work across normal ticks. Rotate workspaces without waiting
inside the global loop. Local deferral bypasses transport fallback and circuits.
Fallback budgets remain tied to the normal poll cycle across continuation wakes.

Task 03 proves publication through the real summary projector and exercises the
existing sidebar consumers. No production frontend change is expected.
If delivery exposes a separate defect, record it before expanding repair scope.

### Compatibility matrix

| Path | Required behavior | Evidence |
| --- | --- | --- |
| PAT and GitHub App, REST/GraphQL | Retain principal admission and credential fences | Real admission with fake HTTP transport |
| Named or legacy CLI, GraphQL | Retain host/account selection and typed deferral | Fake executor plus existing CLI admission regressions |
| Shared principal across workspaces | Shared quota, separate authorization/results | Multi-workspace monitor regression |
| Independent principal | Progress while another principal remains blocked | Mixed-principal monitor regression |
| Client without GraphQL | Existing bounded, rotating REST fallback | Unsupported-client and fallback-count regressions |
| Desktop sidebar and phone task picker | Same summary-driven state without target interaction | Two focused Playwright specs |

Shared implementations do not add a new provider or authentication mode.
Unsupported GraphQL remains a capability fallback, not an authentication failure.

## Tests

New test names are planned additions. Full IDs appear in each work order.

| Acceptance | Regression evidence |
| --- | --- |
| POLLING-001.6, .7 | `TestPRStatusProgress_CompletedKnownPhaseSurvivesDiscoveryPacing` in `service_pr_watch_status_progress_test.go` |
| POLLING-001.6, POLLING-002.7 | `TestPRStatusProgress_PreservesEvidenceAndFences` in the same file |
| POLLING-002.5 | `TestPRMonitorContinuation_LocalDeferralSkipsFallback` in `poller_pr_continuation_test.go` |
| POLLING-002.6, RATE-002.2, .3, .7 | `TestPRMonitorContinuation_ResumesAtAdmissionBoundary` and `TestPRMonitorContinuation_PreservesFairnessAndBudgets` |
| POLLING-001.3, .5, POLLING-002.7 | `TestPRMonitorContinuation_PrunesObsoleteWorkAndStops` plus existing schedule/lifetime suites |
| POLLING-001.8 | `TestPRSidebarDelivery_InactiveTaskReceivesSummary` in `pr_sidebar_status_delivery_test.go` |

The first regression asserts corrected behavior and must fail on the current code.
It reverses the temporary reproduction's assertion of stale stored state.
Use at least five virtual cycles with a changing search set and request counts.
Keep an unchanged-search control and a manual-refresh interleaving case.

## E2E tests

- Desktop: `apps/web/e2e/tests/pr/pr-sidebar-background-refresh.spec.ts`, project `chromium`.
- Phone: `apps/web/e2e/tests/pr/mobile-pr-sidebar-background-refresh.spec.ts`, project `mobile-chrome`.
- Shared fixture: `apps/web/e2e/tests/pr/pr-sidebar-background-refresh-fixtures.ts`.

Both specs keep a separate navigation task active. Publish a newer PR status
for an unopened task and await its correlated summary event before assertions.
Assert the existing icon's `data-pr-state`, unchanged route, and absence of
`github.task_pr.sync` for the target after the stimulus.
Do not hover, tap, focus, or select the target, reload, or write the browser store.
Seed underlying feedback as well as aggregates through existing mock APIs.

The phone opens the existing Tasks picker and leaves it open during publication.
It uses the configured Pixel 5 project. No drawer or layout redesign is required.
Existing composition remains the visual baseline, so no new UI preview is needed.

Browser mock publication proves delivery, not provider admission.
The backend service test is the RED gate for the reproduced defect.
The backend integration test connects corrected monitoring to summary publication.
Together these tests cover the repair without a live GitHub quota experiment.

## Work orders

- [x] [Task 01: Preserve completed PR status progress](task-01-status-progress.md)
- [x] [Task 02: Resume deferred PR monitoring](task-02-admission-continuation.md)
- [x] [Task 03: Prove inactive sidebar delivery](task-03-sidebar-delivery.md)

Dependency order: 01, then 02, then 03. No delegation is authorized.
Each work order contains exact RED and GREEN verification commands.

## Companion records

The discovery-health, watch-task-cleanup, watch-reconciliation, retirement, and
rate-limit integration repair packages retain their recorded scopes and results.
This package owns the new freshness correction. Do not reopen completed work
orders or replace their historical verification with this package's results.

## Verification results

Design validation passed on 2026-10-28:

- `python3 scripts/list-docs.py validate`: 413 decisions and 1795 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- PR-documentation preflight through `.github/scripts/pr-docs.cjs:validateCoverage`:
  all three work orders covered, with no errors. Inputs included the local
  package, both requirement/design pairs, and representative batch/monitor source changes.
- Work-order IDs and relative document links: all references resolved.
- `git diff --check -- docs/specs docs/plans/github-sidebar-pr-freshness`: passed.
- `git status --short -- docs/specs docs/plans/github-sidebar-pr-freshness`:
  two specification edits and the new package were unstaged and uncommitted at
  design handoff.

Implementation verification is recorded under "Implementation results" below.

## Implementation results

Implemented on 2026-10-28 against `4222c3654f9bb1efd1a7d27b5151c4b4c4f1ffd2`.
All three work orders are done; each records its own evidence.

RED evidence on the base commit (`/tmp/opencode/pr-red-baseline`, temporary
worktree, removed after the check):

- `TestPRStatusProgress_CompletedKnownPhaseSurvivesDiscoveryPacing` failed with
  `known PR must be persisted while unrelated discovery is deferred: "[]" should
  have 1 item(s), but has 0`.
- `TestPRMonitorContinuation_ResumesAtAdmissionBoundary` failed with
  `the deferred phase resumes at its admission boundary: expected 1, actual 0`.

GREEN evidence on the implementation checkout (after remediation):

- `(cd apps/backend && go test -tags fts5 -trimpath ./internal/github -count=1
  -timeout=25m)`: ok, 105.7s.
- `(cd apps/backend && go test -tags fts5 -trimpath -race ./internal/github
  -run 'TestPRStatusProgress_|TestPRMonitorContinuation_|TestPRSidebarDelivery_|TestTriggerPRSyncAll'
  -count=1 -timeout=10m)`: ok, 6.7s.
- `(cd apps/backend && go test -tags fts5 -trimpath -race ./internal/github
  -run 'TestPRStatusProgress_|TestPRMonitorContinuation_|Test.*(Poller|PRWatch|PRMonitoring|RateCoordinator|Batched|PRDiscovery)'
  -count=1 -timeout=15m)`: ok, 11.9s.
- `(cd apps/backend && go build -trimpath ./...)`: passed.
- `bash scripts/lint-go-changed apps/backend/internal/github/service_pr_watch_batched.go
  apps/backend/internal/github/poller.go
  apps/backend/internal/github/poller_pr_continuation.go
  apps/backend/internal/github/service_pr_watch.go
  apps/backend/internal/github/graphql_test.go
  apps/backend/internal/github/poller_test.go
  apps/backend/internal/github/service_pr_watch_status_progress_test.go
  apps/backend/internal/github/poller_pr_continuation_test.go
  apps/backend/internal/github/pr_sidebar_status_delivery_test.go`: 0 issues.
- Desktop: `(cd apps/web && pnpm e2e:run --host --project chromium
  tests/pr/pr-sidebar-background-refresh.spec.ts -- --retries=0)`: 1 passed (11.5s).
- Phone: `(cd apps/web && pnpm e2e:run --host --project mobile-chrome
  tests/pr/mobile-pr-sidebar-background-refresh.spec.ts -- --retries=0)`: 1 passed (10.4s).
- `python3 scripts/list-docs.py validate`: 413 decisions and 1795 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- PR-documentation coverage preflight: `covered`, all three work orders, no errors.
- `git diff --check`: passed.

Production changes stay inside `apps/backend/internal/github`; both requirements
and the system design keep their existing `draft` lifecycle.

### Review remediation

A code-only peer review found four defects in the first implementation. All are
fixed and covered by regressions:

- P1: a repeated admission deferral kept its expired deadline, so the monitor
  timer could retry with a zero delay and spin. Pending work now keeps one
  deadline per provider target, and a retried target always replaces its own
  deadline while untouched targets keep theirs.
  `TestPRMonitorContinuation_RepeatedDeferralRearmsForward` proves the second
  deferral re-arms forward, does not retry before the new boundary, and never
  leaves an expired deadline behind.
  `TestPRMonitorContinuation_HonorsExtendedProviderDeadline` proves a long
  provider boundary is not shortened by inventory ticks or the retry floor.
- P2: pending work stored one watch per provider target, so sibling tasks that
  shared a PR lost their resumption opportunity. Each target now retains every
  live watch consumer keyed by lifetime ID; the existing service grouping still
  issues one provider read and fans the result out.
  `TestPRMonitorContinuation_ResumesEverySharedTargetConsumer` proves both tasks
  update from one resumed read.
- P2: pending boundaries survived credential replacement and a new watch
  lifetime. Pruning now compares the workspace credential fingerprint and the
  live watch IDs, so a new principal or a resumed lifetime is admitted at the
  next normal opportunity instead of waiting for the old boundary.
  `TestPRMonitorContinuation_CredentialReplacementDropsStaleBoundary` and
  `TestPRMonitorContinuation_ResumeLifetimeDropsStaleSuppression` cover both
  cases.
- P2: passive per-task sync still entered REST fallback on a typed local
  deferral. `runBatchedOrPerWatchSync` now checks
  `prWatchBatchFallbackAllowed`, reloads completed rows, and returns the
  deferral without another provider attempt.
  `TestTriggerPRSyncAll_DeferredAdmissionSkipsFallback` covers plain, wrapped,
  and joined deferrals with zero REST calls and preserved rows.

Mutation checks confirmed each regression fails against the previous behavior:
removing the passive guard made the wrapped/joined cases return the REST error;
keeping expired deadlines, one consumer per target, and target-key-only pruning
made the continuation regressions fail (the expired-deadline mutation instead
spun the virtual-time loop until the test timeout, which is the reported P1
symptom).

### Review round 2

A second code review of the pushed head found four more defects and two stale
prose statements. All are fixed and covered:

- P1: a successful workspace pass did not clear the poller's prior failure
  streak, so a later failure continued an old backoff instead of starting fresh.
  `checkWorkspacePRWatches` now records the outcome on every completed pass
  (success clears, local deferral is ignored, errors keep their class).
  `TestCheckPRWatches_SuccessClearsPriorCircuitFailure` covers it.
- P1: the numbered phase was applied only after the searching **fetch** finished,
  so a slow search delayed a finished known-PR update and an expired context
  could lose it. `runBatchedLeaderPhases` now fetches, applies, and publishes the
  numbered phase before the searching read starts. A numbered failure also
  completes every searching attempt admitted for that pass, so a joined consumer
  cannot wait on an abandoned attempt.
  `TestPRStatusProgress_PublishesNumberedPhaseWhileSearchingIsHeld` holds the
  searching read and asserts the known-PR row is already persisted.
- P2: a new consumer joining a shared provider target could evict the consumers
  already waiting on it when its own attempt deferred. Pending merges now remove
  only the attempted consumer and keep sibling consumers plus the earliest
  boundary. `TestPRMonitorContinuation_PreservesConsumersOnSharedTargetWake`
  covers the three-consumer case.
- P2: a resume could query a searching watch's old branch after
  `UpdatePRWatchBranchIfSearching` changed it. `resumePRMonitorPending` now
  refreshes retained targets from the active inventory (falling back to the
  credential check alone if the list fails).
  `TestPRMonitorContinuation_ResumeSkipsRebranchedWatch` proves the old branch is
  not queried and the next tick polls the new one.
- Prose: the completed plan no longer claims the tree is uncommitted or that
  verification was not run, and the durable discovery design now describes the
  pre-repair behavior in the past tense.
- The pruning and budget regressions were tightened: retirement uses
  `RetirePRWatch` for both lifetime kinds, and the fairness test now counts a
  transport failure's bounded REST fallback across a continuation wake.



## Documentation impact

The existing public guide already documents one-minute known-PR monitoring.
This repair restores that behavior without a new operation or configuration.
No public page, terminology, or screenshot changes are planned.
New internal contracts remain in the existing discovery requirement/design pair.

## Risks

- A phase result must not publish incomplete review-thread evidence.
- Retained work must not cross credential changes, retirement, or explicit refresh.
- Timer wakes must not create duplicate attempts or reset fallback budgets.
- One blocked principal must not delay another principal's work.
- Actions evidence retains its existing cache bounds, independently of PR cadence.
- Browser publication fixtures alone cannot prove the pacing correction.
