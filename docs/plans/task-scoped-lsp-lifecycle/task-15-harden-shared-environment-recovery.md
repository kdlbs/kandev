---
id: "15-harden-shared-environment-recovery"
title: "Harden shared-environment recovery"
status: completed
wave: 7
depends_on: ["14-verify-status-customization"]
plan: "plan.md"
spec: "../../specs/lsp-file-intelligence/spec.md"
---

# Task 15: Harden Shared-Environment Recovery

## Acceptance

- Task LSP environment lookup ignores deleted historical environment references while rejecting
  multiple surviving physical environments.
- Reset cannot cross a borrower LSP admission, destroy a warm borrower's environment, or invalidate
  an active workspace-group environment pointer.
- Docker borrowers project repository roots from the physical task-host order, not their own task
  ordering; fresh single-repository launches derive the same durable branch identity, and
  incomplete legacy mappings fail closed.
- Task deletion can remove internal runtime credentials after the environment row is cascade
  deleted; durable cleanup snapshots retain the secret references required for retries.
- Task environment endpoints authorize the task owner before runtime access and map denial to a
  non-leaking not-found response. Cached workspace executions cannot bypass the session access
  check.
- Monaco's toolbar controls the task derived from its editor session, never the globally active
  task. A missing session mapping exposes no control.
- Every WebSocket subscription establishment refreshes stable task LSP state, including retrying a
  failed initial load. Revision ordering rejects a delayed REST response after newer live evidence.
- Capacity accepts a new backend epoch through an authoritative snapshot even after wall-clock
  rollback, retires prior epochs against delayed authoritative responses, and does not promote a
  queue entry while recovered processes still fill the configured limit.
- Direct and cascade Stop/Archive/Delete serialize on the shared physical environment, preserve a
  warm non-archived borrower even when its latest session is terminal, and roll back ownership
  transfers when the durable mutation fails.
- Per-language cleanup failures are suppressed only after the runtime proves the physical task-host
  process tree is gone; preserving a borrower's host is an explicit no-op, not cleanup success.
- Optional task-environment repository wiring cannot panic terminal task mutations. Stopping a
  borrower never changes another task's environment owner or readiness.
- Physical-environment admission stays blocked through asynchronous direct and cascade teardown,
  not only through the preceding task-row mutation.
- Partial cascade failure keeps ownership transfers for tasks already archived/deleted and rolls
  back transfers for the failed and not-yet-mutated tasks.
- Concurrent authoritative HTTP refreshes settle in request order, so an older response carrying a
  previously unseen capacity epoch cannot replace a newer response.
- Public docs, the feature spec, and the ADR define shared-owner transfer consistently: a live
  borrower preserves the physical host while the departing task's independent slots stop.
- Direct Archive/Delete and cascades reserve durable cleanup admission before any fallible inventory
  read; failed preparation cancels promptly, and old prepared rows recover even when marker writes
  or cancellation exceeded the immediate retry window.
- Task-host purge retires an acquired slot before map removal, and delayed watch evidence cannot
  resurrect stopped state or capacity. Explicit Start/Restart resets an exhausted recovery epoch.
- Workspace-source refresh holds task/environment admission through runtime mutation. Task hosts
  stay private until readiness plus credential persistence commit, and incomplete hosts are reaped
  before replacement.
- Docker container-auth tokens and serialization state survive warm stop and uncertain cleanup but
  are evicted after confirmed physical removal and executor shutdown.
- Workspace deletion freezes task membership, reserves every task cleanup barrier before inventory,
  holds task and physical-environment mutation admission, and stops task-owned language servers
  before the database cascade.
- Prepared cleanup reservations use the same PostgreSQL task-row serialization as resource
  creators, reject Office-session admission, and renew an explicit owner lease while a legitimate
  long-running mutation remains active.
- Cascade and workspace mutations reconcile durable state after ambiguous database errors instead
  of cancelling cleanup that may already own deleted resources. A delayed GET cannot clear a newer
  task-LSP control failure.

## Verification

Completed 2026-08-12.

- Independent review: GPT-5.6 Sol reviewed the immutable pre-fix head in a separate Kandev task and
  workspace, producing eight actionable findings covered by this task. Its next exact-head audit
  found seven additional recovery gaps: single-repository Docker identity, live terminal borrowers,
  borrower-host cleanup proof, physical-environment mutation guards, failed-load reconnect retry,
  retired capacity epochs, and recovery overflow promotion. All seven have focused regressions.
- Backend focused packages: `go test ./internal/task/service ./internal/task/handlers
./internal/task/repository/sqlite ./internal/orchestrator/executor
./internal/agent/runtime/lifecycle ./internal/backendapp` passed.
- Race-focused lifecycle cases: `go test -race ./internal/task/service ./internal/lsp
./internal/agent/runtime/lifecycle ./internal/orchestrator/executor -count=1` passed, including
  physical-environment admission/reset ordering, ownership rollback, process-tree cleanup evidence,
  and concurrent task-language controls.
- Frontend regressions: nine focused hook, state, status, settings, and control files passed 55
  tests. Web typecheck and lint passed with zero diagnostics; `i18n:check` and `i18n:ratchet`
  passed.
- `make test` exercised the repository broadly but the local workspace filesystem made the
  orchestrator and task-service packages exceed Go's 10-minute package timeout while closing
  SQLite fixtures. No assertion failed; both affected packages passed in the focused command.
- Canonical exact-head CI and a second independent Sol review run after the remediation push; their
  results remain PR delivery evidence rather than changing this completed implementation record.
- The next exact-head GPT-5.6 Sol audit found four blockers and two majors. Nil-safe optional
  environment wiring, owner-only Stop mutation, worker-held physical teardown locks, selective
  partial-cascade rollback, authoritative request ordering, and the shared-owner contract now have
  focused regressions or reconciled docs.
- Post-remediation verification: `go test ./internal/task/service ./internal/mcp/handlers -count=1`
  passed; the exact CI panic regression passed 10 repetitions; the new concurrency cases passed 10
  race-enabled repetitions; full `go test -race ./internal/task/service -count=1` passed; changed
  task-service lint reported zero issues. The two focused frontend files passed 20 tests, ESLint,
  and typecheck. Public-doc validation passed 60 tests and all 41 published pages.
- Main advanced after the remediation push, so the branch was rebased cleanly onto `origin/main` at
  `9c3e7a2d3`. Exact rebased-head verification repeated the full task-service race suite, the exact
  MCP CI regression 10 times, changed-code Go lint, 20 focused frontend tests plus ESLint/typecheck,
  and the 60-test/41-page public-doc validators; all passed.
- Subsequent main updates rebased cleanly through release commit `2089e7c92`, Message Queue UI commit
  `db4fc039a`, and session auto-start commit `1f1710e54`. The last update exposed one required-state
  mismatch in the status-surface test fixture; adding the upstream `resumeSkippedSessionIds` field
  restored type safety. Fresh final-base verification passed the full task-service race suite
  (113.217s), the exact MCP regression 10 times, 9 focused frontend files / 104 tests, the repaired
  status-surface file / 10 tests, web typecheck, changed-code Go lint (zero issues), focused
  ESLint/Prettier, all 60 public-doc validator tests, and all 41 published pages.
- The independent exact-head Sol review verified all six preceding findings closed, then found one
  cascade-delete blocker: `PrepareTaskResourceCleanup` lost legacy runtime-secret references because
  `TaskEnvironment` intentionally marks them `json:"-"` and the task FK removed the source row before
  the worker ran. Direct and prepared cleanup now share one snapshot builder that copies both IDs.
  The real cascade-delete regression failed before implementation and passed afterward; its focused
  race set passed 10 repetitions, the full task-service suite passed (59.186s), the full race suite
  passed (91.743s), the exact MCP regression passed 10 repetitions, and changed-code Go lint reported
  zero issues.
- Two subsequent unrelated utility/dialog commits moved `origin/main` to `3928c58ec`; the branch
  rebased cleanly. On that exact base, the full task-service suite passed (66.790s), the focused
  credential-cleanup race set passed 10 repetitions, 3 overlapping frontend files / 35 tests passed,
  web typecheck passed, changed-code Go lint reported zero issues, all 60 public-doc validator tests
  passed, and all 41 published pages validated.
- A subsequent independent audit found a teardown race in capacity promotion: after `Release`
  promoted a queued entry, `promoteCapacityEntry` could reach runtime launch without reacquiring the
  task/environment admission barrier used by ordinary Start. A regression first demonstrated the
  queued server becoming ready despite an active teardown barrier. Promotion now reacquires task
  admission before task/environment/runtime access; a blocked promotion releases its reservation,
  returns to `waiting_for_task`, and schedules desired-state recovery. The focused regression passed,
  `go test -race ./internal/lsp -count=10` passed, the three task-service admission regressions passed
  10 race-enabled repetitions, and changed-code LSP lint reported zero issues.
- The same audit found that preserving a shared physical host also retained the departing borrower's
  task-keyed slots, snapshots, subscriptions, diagnostic/capability caches, and workspace projection.
  Cleanup now invokes one trusted task-scoped agentctl purge after language stop attempts. Purge
  refuses live or pending runtime ownership, deletes only the departing namespace, and remains an
  error when the shared host survives; a proven full process-tree reap safely supersedes it. The
  controller borrower cases, manager isolation/live-runtime cases, authenticated internal route,
  and client transport tests passed 10 race-enabled repetitions.
- Sol Max found a second cleanup blocker: `PrepareTaskResourceCleanup` reserved a durable admission
  barrier before inventory capture, but a capture failure returned without cancelling the current
  reservation because the cascade map recorded it only after success. Preparation now cancels its
  own failed reservation, the coordinator records and cancels the current operation too, and the
  terminal transition retries on a bounded detached context. Regressions prove the real job is
  `cancelled`, session creation resumes, the failed coordinator reservation is included, and a
  cancelled request survives one transient cancellation failure.
- The capacity follow-up found one remaining stranded-generation path: after admission rejected a
  queued promotion, failure to persist `waiting_for_task` also suppressed recovery because no
  snapshot existed. Promotion now schedules recovery from its already-validated keep-warm intent;
  the focused admission and persistence-error tests passed 20 race-enabled repetitions. A bounded
  two-second process-stop deadline also stabilized the real process-tree proof under full-package
  race-detector load without changing the process-reap assertion.
- The final separate-workspace Sol Max review produced eight actionable findings. Synchronized
  regressions cover barrier-first direct mutation, durable abandoned preparation, purge/start
  exclusion, delayed-watch Stop ordering, fresh explicit-control recovery epochs, refresh/teardown
  admission, task-host readiness publication, and Docker auth-state eviction. Broad verification
  passed `go test -race` for `internal/lsp`, `internal/agentctl/server/lsp`,
  `internal/agent/runtime/lifecycle`, `internal/task/service`, and
  `internal/task/repository/sqlite`. A broader run exposed and fixed command coalescing into an
  exclusive watch callback before all five packages passed again.
- The branch rebased without conflict onto `origin/main` at `2c6a9c287`. On that exact
  base, the five-package race suite passed (task service 138.920s), changed-code Go lint reported
  zero issues, all 60 public-doc validator tests passed, all 41 published pages validated, and
  `git diff --check origin/main...HEAD` passed.
- A subsequent exact-head GPT-5.6 Sol review found seven cleanup/control defects not covered by CI:
  non-atomic PostgreSQL reservation, workspace inventory and LSP ordering, age-only preparation
  abandonment, Office-session admission, stale-GET error clearing, and ambiguous workspace commit
  handling. Barrier-first workspace teardown, task-row reservation locking, renewable preparation
  ownership, shared Office admission, durable outcome reconciliation, and frontend error epochs now
  have focused regressions. The full task-service and SQLite suites passed; their race suites passed
  in 112.526s and 24.024s; 22 focused frontend tests, typecheck, strict targeted ESLint/Prettier,
  i18n ratchet, and changed-code Go lint passed. The one unrelated agentctl timing failure from the
  combined race run passed five immediate race-enabled repetitions.
- The remediated branch rebased without conflict onto `origin/main` at `7855a4e08`. Exact-base
  verification passed the task-service and SQLite race suites (117.110s and 45.179s), changed-code
  Go lint with zero issues, 22 focused frontend tests, strict targeted ESLint, web typecheck, the
  i18n ratchet, all 62 public-doc validator tests, validation of all 41 published pages, and the
  three-dot diff whitespace check.
- After `origin/main` advanced again, the branch rebased without conflict onto `be966f410`. The
  intervening product source was unchanged. Final-base verification passed the complete task-service
  and SQLite suites (66.548s and 25.059s), changed-code Go lint with zero issues, 22 focused frontend
  tests, web typecheck, all 62 public-doc validator tests, validation of all 41 published pages, and
  the three-dot diff whitespace check.
- A fresh separate-workspace GPT-5.6 Sol audit found four attachment and rollback blockers despite
  green CI. Red-first tests reproduced loss after 255 cached diagnostic URIs, an overflowed but open
  attachment transport, permanent same-generation browser retry exhaustion, and unauthenticated
  Docker task-host rollback when the container token had not rotated. Replay now bypasses the
  bounded live queue, overflow detaches and closes the message stream, browser retries continue at
  the capped five-second backoff, and Docker retains the effective control credential separately
  from the optional durable-token update. The two attachment regressions passed 10 race-enabled
  repetitions; full agentctl LSP/API and lifecycle race suites passed; 29 focused frontend tests,
  strict targeted ESLint, and web typecheck passed.
- The remediated branch rebased without conflict onto `origin/main` at `d616a1fcd`. Exact-base
  verification passed the agentctl LSP/API, lifecycle, task-service, and SQLite packages under the
  race detector; changed-code Go lint with zero issues; 29 focused frontend tests; strict targeted
  ESLint; web typecheck; the i18n ratchet; all 62 public-doc validator tests; validation of all 41
  published pages; and the three-dot diff whitespace check.
- Exact-head review found one remaining Docker rollback gap: reconnect retained the effective
  credential but dropped the task-host ownership marker from the runtime instance. Failed readiness
  could therefore force-remove the shared task container instead of deleting only the task-host
  agentctl instance. Reconnect now derives the marker from the typed task-host request before
  rollback. The regression uses real reconnect plus authenticated deletion, rejects any Docker
  container mutation, and passed 20 race-enabled repetitions.

## Files

- `apps/backend/internal/task/service/**`
- `apps/backend/internal/task/repository/sqlite/session.go`
- `apps/backend/internal/orchestrator/executor/**`
- `apps/backend/internal/agent/runtime/lifecycle/**`
- `apps/backend/internal/task/handlers/**`
- `apps/web/hooks/domains/lsp/**`
- `apps/web/hooks/use-lsp.ts`
- `apps/web/components/editors/**`
- `apps/web/lib/state/slices/lsp/**`
