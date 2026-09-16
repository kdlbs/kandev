---
id: "04-reachability-api-and-events"
title: "Reachability API, change event, and immediate probe"
status: done
wave: 3
depends_on: ["03-reachability-poller"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-REACHABILITY-001
  - REQ-EXECUTORS-SSH-REACHABILITY-002
acceptance_criteria:
  - AC-EXECUTORS-SSH-REACHABILITY-001.14
  - AC-EXECUTORS-SSH-REACHABILITY-001.18
  - AC-EXECUTORS-SSH-REACHABILITY-001.19
  - AC-EXECUTORS-SSH-REACHABILITY-001.26
  - AC-EXECUTORS-SSH-REACHABILITY-001.28
  - AC-EXECUTORS-SSH-REACHABILITY-002.2
  - AC-EXECUTORS-SSH-REACHABILITY-002.3
  - AC-EXECUTORS-SSH-REACHABILITY-002.5
  - AC-EXECUTORS-SSH-REACHABILITY-002.6
  - AC-EXECUTORS-SSH-REACHABILITY-002.7
system_design:
  - ../../specs/executors/system-design/ssh-reachability-surfaces.md
  - ../../specs/executors/system-design/ssh-reachability.md
---

# Task 04: Reachability API, Change Event, and Immediate Probe

## Summary

Project the reachability record over HTTP as its own resource, publish a
change event only when the state or reason actually changes, and add an
immediate-probe action that coalesces concurrent callers into one connection.
Reset an executor's record when its connection configuration is saved.

## In scope

- Three routes on the existing `/api/v1/ssh` group:
  `GET /reachability` for every SSH executor's record,
  `GET /executors/:id/reachability` for one, and
  `POST /executors/:id/reachability/probe` to probe now, persist, and return
  the record.
- **Do not reuse `resolveSSHTarget`'s mapping wholesale.** That helper answers
  `400` both for a wrong executor type *and* for a failure to project the
  executor's config into a target, and an unresolvable configuration (or one
  with no pinned fingerprint) is exactly the case `AC-…-002.5` requires to
  return a `200` outcome carrying reason `config` instead. On all three
  routes: `404` for a nonexistent id or one that is soft-deleted; `400` only
  when the id names an executor whose `type` is not `ssh`; an unresolvable
  `ssh` config is a `200` with reason `config`, never a `400`. The `POST`
  route additionally answers `409` when the executor is `ssh` but its `status`
  is not `active` — probing a deactivated executor on request would
  contradict the rule that its retained record stays unchanged.
- `GET /reachability` returns one entry per SSH executor that is not
  soft-deleted, **ordered by `executor_id` ascending** — the same total order
  the poller's pass uses — and includes an executor whose `status` is not
  `active`, carrying its retained record (`AC-…-001.17`).
- `GET` on an eligible executor with no record returns the `unknown` shape
  with every timestamp — `checked_at`, `last_success_at`, and `updated_at` —
  `null`, never `404`. A client treats a null `updated_at` as older than any
  real record, so this synthesized shape can never displace one that actually
  exists.
- The DTO adds `updated_at` alongside `checked_at` and `last_success_at`, and
  `probing_enabled`, `false` when the interval key is `0`, so a surface
  distinguishes "not probed yet" from "probing is off" without inferring it
  from an absent timestamp. `probe_interval_seconds` carries the **effective**
  (clamped) interval, which task 06's staleness and refetch logic depends on.
- Coalescing through a `golang.org/x/sync/singleflight` group keyed by executor
  id, so two overlapping `POST`s open one connection and share one result. Its
  write follows the same last-write-wins rule as a scheduled probe. **The
  coalesced probe does not run on any caller's request context** — a
  disconnecting caller must neither cancel it nor fail the other caller
  waiting on it — but it is also not `context.Background()`: it runs through
  task 03's off-cycle-probe primitive, registered on the reachability
  package's own `WaitGroup`/context, so `Stop` still cancels and awaits it. A
  probe that ran but was not stored (the write failed, or a guard refused it)
  still answers `200` with the outcome and `persisted: false`.
- The immediate probe runs and persists while the poller is disabled.
- `events.ExecutorReachabilityChanged = "executor.reachability.changed"`, a
  matching `ws.ActionExecutorReachabilityChanged`, and the bridge subscription
  in `task_notifications.go` beside `events.ExecutorUpdated`. Published only
  when `state` or `reason` differs from the stored record.
- **Save detection lives in `internal/task/service`, not `internal/ssh`.**
  `Service.CreateExecutor` and `Service.UpdateExecutor`
  (`service_resources.go`) are the only two call sites that can form a
  before/after connection-configuration comparison — `internal/ssh` never
  sees an executor write. Each notifies an `ExecutorSaveObserver` interface
  the service package declares (this task adds the interface and the two call
  sites); `internal/executors/reachability` implements it and is wired at
  construction (also this task). `applyExecutorUpdates` mutates the loaded
  executor in place, so `UpdateExecutor` must copy the before-values out
  ahead of that call, or a same-row comparison never detects a difference and
  the reset never fires. On any difference in the connection-configuration
  fields (a create counts as a difference by definition), the observer calls
  `ResetExecutorReachability` with the newly saved host and dispatches an
  out-of-band probe through task 03's off-cycle primitive, if the executor is
  still eligible.

## Out of scope

- Any frontend file. Task 06 owns the client types, API calls, and store.
- Changing `POST /api/v1/ssh/test`, `probe-agents`, or `probe-shells`. The
  immediate-probe action is a separate action against a saved executor and its
  pinned fingerprint; the test endpoint keeps dialing unpinned against a
  possibly-unsaved form configuration and keeps writing no record.
- Reachability fields on the executor DTO. A record that changes every interval
  must not invalidate the executor list payload.
- Any launch-path behavior (task 05).
- The `ResetExecutorReachability` SQL statement itself and the off-cycle-probe
  dispatch primitive. Both are implemented in tasks 02 and 03 respectively;
  this task only calls them from the new observer and the `POST` route.

## Acceptance

- Two overlapping `POST` probes for one executor produce exactly one dial and
  return one identical record to both callers.
- A probe whose result matches the stored state and reason publishes nothing;
  a probe that changes either publishes exactly one event carrying the record.
- `GET /reachability` returns entries ordered by `executor_id` ascending,
  includes a deactivated `ssh` executor's retained record, and omits a
  soft-deleted executor entirely.
- A `GET` for a wrong-type executor id returns `400`; a `GET` for an `ssh`
  executor whose config cannot resolve a target returns `200` with reason
  `config`, not `400`. A `POST` against a deactivated `ssh` executor returns
  `409` without probing.
- Saving a changed host on an existing SSH executor leaves its record
  `unknown` with a zero counter and both timestamps `null`, and dispatches a
  probe without waiting for the next scheduled pass, rather than carrying the
  previous target's failure streak. Saving a change that does not touch the
  connection-configuration fields (for example only a name/label change)
  leaves the record untouched.
- Cancelling the client that issued a `POST` mid-probe neither cancels the
  probe nor prevents its result from being persisted.

## Verification

Start with the coalescing test as a failing test — hold a probe open, issue a
second request, and assert one dial and two identical responses — and confirm
it fails against a handler that probes per request. Then run:

```bash
# From apps/backend:
go test -tags fts5 -race ./internal/ssh/...
go test -tags fts5 -race ./internal/executors/reachability/... -run 'Publish|Change|SaveObserver'
go test -tags fts5 -race ./internal/task/service/ -run 'Executor.*Reachability|SaveObserver'
go test -tags fts5 -race ./internal/gateway/websocket/ -run 'Reachability|Executor'
go test -tags fts5 -race ./internal/events/...
make lint
```

## Files likely touched

- `apps/backend/internal/ssh/reachability_handlers.go`
- `apps/backend/internal/ssh/reachability_handlers_test.go`
- `apps/backend/internal/ssh/handlers.go`
- `apps/backend/internal/events/types.go`
- `apps/backend/pkg/websocket/actions.go`
- `apps/backend/internal/gateway/websocket/task_notifications.go`
- `apps/backend/internal/executors/reachability/publish.go`
- `apps/backend/internal/executors/reachability/publish_test.go`
- `apps/backend/internal/executors/reachability/save_observer.go`
- `apps/backend/internal/executors/reachability/save_observer_test.go`
- `apps/backend/internal/task/service/service_resources.go`
- `apps/backend/internal/task/service/service_resources_executors_reachability_test.go`

## Dependencies

Task 03. The poller's write path is the single place that decides whether a
result is a change, and the immediate probe must share it rather than
reimplementing hysteresis at the handler. Task 02's `ResetExecutorReachability`
statement and task 03's off-cycle-probe primitive, both consumed by the new
`ExecutorSaveObserver`.

## Risks

- A `singleflight` group keyed by executor id shares the *result* as well as
  the work. A caller that mutates the returned record would corrupt the other
  caller's copy; return a value, not a pointer into shared state.
- The change-only publish rule is what keeps a steady host silent, and it is
  also what makes `checked_at` age in an open client. Task 06 must refetch on
  the interval; if that is dropped, this task's correctness becomes a
  user-visible staleness bug in a different work order.
- Registering a new WS action without the bridge subscription produces no
  compile error and no test failure unless the bridge is asserted directly.
- The save-reset mechanism spans two packages: the interface and its call
  sites live in `internal/task/service`, the implementation in
  `internal/executors/reachability`. A change to `applyExecutorUpdates`'s
  field set in a later, unrelated PR can silently stop detecting a
  connection-configuration change if the observer's field list is not kept in
  sync; there is no compiler error for a comparison that reads a field that no
  longer changes. Name the exact compared fields in the observer's own test,
  not just in a comment.
- `Service.UpdateExecutor` mutates the loaded executor in place via
  `applyExecutorUpdates`; capturing before-values after that call instead of
  before it compares the row against itself and the reset silently never
  fires. Assert this with a test that changes the host and asserts the
  observer actually saw old-host/new-host, not just that it was called.

## Parallelism

`parallel-safe` with task 05 — disjoint files, both depend only on task 03.

## Inputs

- System design, sections *API and event contracts* and *Save detection*.
- `internal/ssh/handlers.go` for the route group and the WS dispatcher
  registration shape (not its 404/400 mapping — see the *Do not reuse*
  bullet above).
- `internal/gateway/websocket/task_notifications.go` around the
  `events.Executor*` subscriptions.
- `internal/task/service/service_resources.go` around `CreateExecutor`
  (line ~1581), `UpdateExecutor` (line ~1611), and `applyExecutorUpdates`
  (line ~1683) for the exact before/after capture points.

## Results

Implemented as specified.

- `internal/executors/reachability/publish.go` adds `RecordDTO`, `BuildRecordDTO`
  (the nil-record → `unknown` placeholder, `updated_at`/`probing_enabled`/
  `probe_interval_seconds`/`persisted` projection), and `Publisher.PublishChanged`,
  wired via `Poller.SetPublisher`. `Poller.probeAndPersist` and the new
  `ProbeAndWait` (`probe_and_wait.go`) both route through a shared
  `observeAndPublish` so every probe primitive (scheduled pass, `ProbeNow`,
  `ProbeAndWait`) publishes through the one path, only on an actual state/reason
  change.
- `ProbeAndWait` coalesces concurrent callers for the same executor id through a
  `golang.org/x/sync/singleflight` group and copies the shared record out before
  returning, so no caller can mutate another's copy. It runs on the poller's own
  context/`WaitGroup` (via `acquire()`), never the caller's request context, so
  `Stop` still drains it and a disconnecting HTTP client neither cancels it nor
  affects a sibling still waiting on the same coalesced result.
- `internal/task/service`: added the `ExecutorSaveObserver` interface and
  `SetExecutorSaveObserver`; `CreateExecutor`/`UpdateExecutor` call
  `notifyExecutorSaved` unconditionally on every save (mirroring
  `publishExecutorEvent`'s unconditional-fire shape) — all SSH-type/active-status/
  connection-config-diff gating lives in the observer implementation, not the
  service. `UpdateExecutor` captures `before := *executor` strictly before
  `applyExecutorUpdates` mutates the loaded executor in place.
- `internal/executors/reachability/save_observer.go` implements
  `ExecutorSaveObserver`: eligible only for an active `ssh` executor, resets via
  `ResetExecutorReachability` only when one of the eight connection-config keys
  actually changed (a nil `before` — create — always counts as changed), publishes
  the reset when the previous record was worth announcing, and dispatches an
  off-cycle probe via `Poller.ProbeNow` without waiting for the next scheduled pass.
- `events.ExecutorReachabilityChanged`, `ws.ActionExecutorReachabilityChanged`, and
  the `task_notifications.go` bridge subscription (bumping `wantSubscriptions` to
  72) route the change event to WS clients on the broadcaster's default (global)
  path, matching `ExecutorUpdated`.
- `internal/ssh/reachability_handlers.go` adds the three routes. `loadSSHExecutor`
  enforces 404 (nonexistent/soft-deleted) vs. 400 (wrong type) without attempting
  config resolution — deliberately not reusing `resolveSSHTarget`, whose 400 also
  covers an unresolvable config. `buildReachabilityDTOFromRecord` synthesizes the
  `unknown` placeholder for an eligible executor with no record, or a `reason:
  config` record (never a 400) when the executor's own config can't resolve a
  target via `lifecycle.SSHTargetFromExecutorConfig`. `listReachability` lists all
  executors and all records in one query each (no N+1), filters to type `ssh`,
  sorts by `ID` ascending, and includes a deactivated executor's retained record.
  `probeReachability` returns 409 for a non-active `ssh` executor without probing,
  otherwise delegates to the shared `*reachability.Poller.ProbeAndWait`.
- `internal/backendapp`: `startAgentInfrastructure` now captures
  `startSSHReachabilityPoller`'s return value, wires its publisher
  (`reachabilitypkg.NewPublisher(eventBus)`) and the task service's
  `ExecutorSaveObserver` (`reachabilitypkg.NewSaveObserver`), and threads the one
  running `*reachability.Poller` instance through `startGatewayAndServe` →
  `buildHTTPServer` → `routeParams.sshReachabilityPoller` → `sshhandlers.RegisterRoutes`.
  A nil poller is passed as a nil `ReachabilityProber` interface (not a typed-nil
  `*Poller`), avoiding the classic Go nil-interface trap in `helpers.go`.
- Process note: production code for `reachability_handlers.go` was written once,
  in full, before any test existed for it — a Iron Law violation caught before
  it compiled cleanly. The file was deleted and rebuilt from the work order's
  mandated RED test first (`TestProbeReachability_CoalescesConcurrentProbes`,
  confirmed to fail on a missing `probeReachability`/extended `NewHandler`
  signature), then GREEN, then the remaining acceptance criteria were each
  covered by a dedicated test (404/400/409/reason-config/ordering/filtering).
  Three of those — the config-resolution branch, the ascending sort, and the 409
  short-circuit — were confirmed non-vacuous by temporarily breaking the
  corresponding production logic and observing the test fail, then restoring it.

Verification (all green):

```bash
go test -tags fts5 -race ./internal/ssh/...
go test -tags fts5 -race ./internal/executors/reachability/... -run 'Publish|Change|SaveObserver'
go test -tags fts5 -race ./internal/task/service/ -run 'Executor.*Reachability|SaveObserver'
go test -tags fts5 -race ./internal/gateway/websocket/ -run 'Reachability|Executor'
go test -tags fts5 -race ./internal/events/...
go test -tags fts5 -race ./internal/backendapp/...
make lint
```

Running the full, unfiltered `internal/task/service` suite (not part of this
task's verification list, which only requires the `Executor.*Reachability|
SaveObserver` filter) surfaces a pre-existing, unrelated cluster of failures —
`TestDesktopDiscoveryRootPersistsAcrossServiceRestart`,
`TestReconnectDesktopDiscoveryRootNormalizesOldPath`,
`TestDesktopDiscoveryFailurePreservesCachedRepositories`,
`TestArchiveTaskCleanupPreservesTaskEnvironmentIdentity`,
`TestArchiveUnarchiveResumeReactivatesLocalOnlyBranch`,
`TestDeleteTaskCleanupFindsWorktreeAfterLastSessionDeletedAndRestart`,
`TestDeleteTaskCleanupRemovesEveryWorktreeAfterLastSessionDeletedAndRestart`,
`TestDeleteTaskWithDiscardConsentPersistsAndCleansDirtyWorktree`,
`TestDirtyWorktreeCleanupBatchOrderRemainsRetryable`, and
`TestRepositoryBranchPolicyServiceGitflowStarterIsAtomicAndOneTime`. All ten
are the same class of macOS `/var` vs `/private/var` `TMPDIR` symlink
resolution issue ("unsafe worktree path ... not a directory", "saved
repository path resolves to a different location"), confined to worktree
cleanup, repository discovery, and branch-policy files this task never
touches — none reference executors, reachability, or SSH.
