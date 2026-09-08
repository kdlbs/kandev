---
spec: docs/specs/platform/requirements/git-subprocess-admission.md
created: 2026-08-02
status: implemented
---

# Implementation Plan: Git Subprocess Admission

## Overview

Replace the undifferentiated Git semaphore with one class-aware,
work-conserving admission controller, then migrate interactive, lifecycle, and
background call sites onto its typed API. Complete the repair by separating
workspace-poll admission from execution timeouts and exposing authenticated
contention diagnostics. Public Git response contracts and the existing process
cap remain unchanged.

Confirmed root cause: all Git work waits on one undifferentiated process-wide
pool, while multi-repository interactive fan-out can fill that pool. Workspace
polling creates its 10-second context before admission and treats the resulting
queue timeout like a Git failure, so repeated contention can stop an otherwise
healthy tracker. The merged fan-out limits reduce pressure but do not provide
cross-class progress or separate queue delay from execution failure.

---

## Backend

### Shared class-aware admission

- **Files:** `apps/backend/internal/common/subproc/throttle.go`,
  `apps/backend/internal/common/subproc/shared.go`,
  `apps/backend/internal/common/subproc/metrics.go`, and focused sibling tests.
- Add the following public contract (names may vary only to match established Go
  naming in the package):

```go
type GitWorkClass string

const (
    GitInteractive GitWorkClass = "interactive"
    GitLifecycle   GitWorkClass = "lifecycle"
    GitBackground  GitWorkClass = "background"
)

func AcquireGit(ctx context.Context, class GitWorkClass) (release func(), err error)
func GitCapacity() int
func AdmissionSnapshot() Snapshot
```

- Implement one mutex-protected FIFO per class and a round-robin cursor ordered
  `interactive`, `lifecycle`, `background`. Skip empty queues and grant
  immediately whenever unused capacity exists.
- Remove canceled waiters under the scheduler lock and make release idempotent.
  Aggregate and per-class counters must be updated from the same synchronized
  state.
- Require `GitWorkClass` on run/output/combined-output helpers and any
  after-admission command builder. Keep generic `Throttle` for `gh`; retain old
  Git entry points only during migration, then remove or privatize them after all
  call sites are classified.
- Preserve `KANDEV_GIT_MAX_CONCURRENT` resolution. Add only a test-scoped capacity
  seam; runtime resizing is not part of this repair.

### Interactive operations and repository fan-out

- **Files:** `apps/backend/internal/agentctl/server/api/git.go`,
  `apps/backend/internal/agentctl/server/api/git_fanout_test.go`, and agentctl
  process Git operators outside the polling path, including `git.go`,
  `git_pr_providers.go`, `workspace_files.go`, `workspace_git_cmd.go`,
  `workspace_git_commits.go`, and `workspace_git_diff.go`.
- Classify user-request status, log, diff, staging, branch, commit, fetch, pull,
  push, file-history, and PR-support commands as `GitInteractive`.
- Replace the unbounded status goroutine burst and `maxGitFanout = 8` with one
  helper that creates at most
  `min(repository count, subproc.GitCapacity())` workers.
- Preserve indexed result ordering, response shapes, and partial-error behavior.
  Admission remains authoritative for active subprocess count across overlapping
  requests.

### Backend lifecycle operations

- **Files:** `apps/backend/internal/worktree/git_throttle.go`,
  `apps/backend/internal/worktree/manager_git.go`,
  `apps/backend/internal/repoclone/clone.go`, and
  `apps/backend/internal/agent/runtime/lifecycle/env_preparer_local.go`.
- Classify cloning, fetching, branch preparation, worktree creation/removal,
  cleanup, rescan, and local runtime preparation as `GitLifecycle`.
- Preserve arguments, credentials, error wrapping, rollback, and cleanup. Use
  focused search in the owned backend packages as the migration completeness
  check.

### Workspace tracker admission and timeout ownership

- **Files:** `apps/backend/internal/agentctl/server/process/workspace_monitor.go`,
  `workspace_git_cmd.go`, `workspace_git_poll.go`, `workspace_tracker.go`, and
  focused sibling tests.
- Classify initial index resolution, tracker setup, and explicit rescan as
  `GitLifecycle`; classify repeating polls as `GitBackground`.
- Acquire with the tracker lifetime context. Only after admission succeeds,
  create the existing 10-second execution context and construct/start Git.
- Return an internal admission-canceled result that does not increment
  consecutive Git failures. Preserve the five-failure stop rule for genuine
  post-admission failures and the existing missing-workspace behavior.
- After interactive, lifecycle, and poller migrations are complete, remove or
  privatize the classless Git entry points and use a repository-wide production
  search plus compilation as the final completeness check.

### Authenticated diagnostics

- **Files:** `apps/backend/internal/agentctl/server/api/control_server.go`,
  `apps/backend/internal/agent/runtime/agentctl/control.go` with focused tests.
- Preserve aggregate expvar metrics and add class-keyed inflight, waiter,
  acquisition, and cumulative admission-wait values plus an immutable snapshot.
- Add authenticated `GET /api/v1/debug/subprocess-admission` to the agentctl
  control group and `ControlClient.SubprocessAdmission(ctx)` using its existing
  bearer transport.

---

## Tests

The controller fairness test, mixed interactive/background tracker test, and
authenticated diagnostic integration test are written first and must fail on
current behavior for the missing class policy, timeout boundary, and endpoint
respectively before implementation turns them green.

- **What:** hard cap, same-class FIFO, deterministic class rotation,
  work-conserving admission, cancellation, idempotent release, env capacity, and
  metric/snapshot accounting.
  **File:** `apps/backend/internal/common/subproc/*_test.go`.
  **How:** table-driven tests with capacity 1–3 and channels controlling grants,
  releases, and cancel-versus-release races; repeat concurrency cases under
  `go test -race`.
- **What:** capacity-derived fan-out and unchanged status/log/diff order and
  partial results.
  **File:** `apps/backend/internal/agentctl/server/api/git_fanout_test.go` and
  focused handler tests.
  **How:** overlapping handler requests against temporary repositories through
  the real Git operator; assert peak inflight and `interactive` attribution.
- **What:** backend lifecycle classification and cancellation cleanup.
  **File:** focused tests in `internal/worktree`, `internal/repoclone`, and
  `internal/agent/runtime/lifecycle`.
  **How:** saturate admission, cancel preparation, and assert no command starts and
  existing rollback remains intact.
- **What:** interactive saturation cannot turn a queued poll into a Git failure.
  **File:** `apps/backend/internal/agentctl/server/process/workspace_*_test.go`.
  **How:** use a cross-platform helper process, hold interactive work, enqueue a
  poll, release slots, and assert bounded execution, unchanged failure count, and
  zero leaked slots/waiters.
- **What:** authenticated diagnostic path.
  **File:** control-server and `ControlClient` tests.
  **How:** exercise route → bearer-authenticated client and assert authorized,
  unauthorized, and snapshot response behavior.
- **What:** before/after Windows evidence required to close issue #2150.
  **File:** attach results to the issue or implementation PR; no permanent host
  fixture.
  **How:** with cap 12, run the same multi-repository workload on current `main`
  and the completed branch; record p50/p95/max latency, admission/execution
  timeouts, tracker stops, and peak Git process count.

---

## Verification Results

Implementation checks are recorded in the task files. The focused admission,
diagnostics, poller race tests, production classless-call audit, and
`make -C apps/backend build` all pass. The Windows before/after workload
comparison required to close issue #2150 remains an external evidence step and
is not available in this Linux worktree.

The affected backend package sweep also passes: `go test ./internal/common/subproc
./internal/agentctl/server/process ./internal/agentctl/server/api
./internal/worktree ./internal/agent/runtime/lifecycle
./internal/agent/runtime/agentctl ./internal/repoclone ./internal/debug
./internal/backendapp -count=1` — 2,340 tests across 9 packages.

Review-blocker validation also passes: the raw-Git repository guard is green,
the deterministic cancel/release boundary test is green under `go test -race
./internal/common/subproc`, and the fresh-status regression records only
`GitInteractive` admissions for every subprocess in that observation. A
compile-only sweep (`go test -run '^$' ./...`) is green after the final audit.

The repository-wide `go test -tags fts5 ./...` reached 10,254 passing tests and
29 skips but stopped on the pre-existing websocket assertion
`TestTaskEventBroadcaster_NoDuplicateSubscriptions` (62 subscriptions versus
its expected 61); the same isolated test fails without any files in this
change's scope.

---

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [task-01-class-aware-admission](task-01-class-aware-admission.md) — done

Wave 2 (parallel candidates — user authorization required):

- [x] [task-02-interactive-git-fanout](task-02-interactive-git-fanout.md) — done
- [x] [task-03-lifecycle-git-classification](task-03-lifecycle-git-classification.md) — done
- [x] [task-05-admission-diagnostics](task-05-admission-diagnostics.md) — done

Wave 3:

- [x] [task-04-poller-admission-liveness](task-04-poller-admission-liveness.md) — done

Wave 4:

- [x] [task-06-review-blockers](task-06-review-blockers.md) — done

The default is sequential execution in the primary conversation. Wave 2 is
parallel-safe only because the tasks own disjoint call-site, backend lifecycle,
and diagnostics files; waves do not authorize subagents.

---

## Risks and mitigations

| Risk | Mitigation |
| --- | --- |
| Cancellation races grant work after its caller exits | Queue removal, grant, and counters share one lock; targeted race tests cover cancel-versus-release |
| A forgotten call site remains classless | Remove or privatize classless Git access, audit raw command construction/lookup/exec paths, and enforce the repository guard |
| Round robin reduces single-class throughput | Scheduler skips empty queues and lets the only active class use full capacity |
| Fan-out refactor changes result order or partial errors | Preserve indexed result slots and add handler-level regression tests for all three operations |
| Diagnostics expose process-local state across runtimes | Keep the authenticated agentctl snapshot endpoint process-local and do not aggregate or persist it |
| Fair admission cannot help a permanently hung command | Keep post-admission execution timeouts and release slots on every terminal path |

## Completion criteria

- All task files have recorded passing validation commands.
- Production Git call sites compile only through a declared work class.
- Race tests cover the scheduler and tracker contention paths.
- Documentation links and indexes resolve.
- The Windows before/after evidence required by the spec is attached before
  closing issue #2150.
