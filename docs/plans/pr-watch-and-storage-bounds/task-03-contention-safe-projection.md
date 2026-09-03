---
id: "03-contention-safe-projection"
title: "Coalesce task-status projection contention"
status: done
wave: 1
depends_on: ["02-idempotent-polling-events"]
plan: "plan.md"
spec: "../../specs/platform/pr-watch-and-storage-bounds.md"
---

# Task 03: Coalesce task-status projection contention

## Intent

Ensure duplicate/equivalent PR signals and genuine CAS races converge without handler errors or lost final summaries.

## Acceptance

- Equivalent pending refreshes for one task are single-flight/coalesced and result in at most one effective summary persistence/publication.
- Genuine CAS loss reloads/rebases authoritative state and uses bounded exponential backoff with jitter; it does not merely raise the retry count.
- Concurrent PR-event tests preserve the correct final summary and report zero exhausted-CAS handler errors.

## Files likely touched

- `apps/backend/internal/task/statussummary/projector.go`
- `apps/backend/internal/task/statussummary/projector_events.go`
- `apps/backend/internal/task/statussummary/projector_helpers.go`
- `apps/backend/internal/task/statussummary/projector_test.go`
- `apps/backend/internal/task/statussummary/projector_pending_authority_test.go`
- New concurrent projector regression test

## Dependencies

Task 02.

## Parallelism

Sequential. It depends on the canonical PR-event semantics from task 02.

## Verification

```bash
cd apps/backend && go test -race ./internal/task/statussummary -run 'Test.*(Projector|CAS|Concurrent|PullRequest).*' -count=1 -v
cd apps/backend && go test ./internal/task/statussummary -count=1
git diff --check
```

## Output contract

Report expected red failure, contention test worker/count setup, final revision/summary assertions, and no-exhaustion result. Update task and plan status.

## Results

Implemented and verified.

**What was already satisfied (no code change needed):**
- Per-task in-process serialization already existed (`Projector.lockTask`), so
  concurrent events for one task are already sequenced, never interleaved.
- `persistAndPublishLocked` already short-circuits a semantically-equal derived
  summary (`TaskStatusSummary.SemanticEqual`) before touching the store or
  publishing, so N concurrent equivalent refreshes for one task already
  converge to at most one real persist/publish (`TestProjectorCoalescesConcurrentEquivalentPendingRefreshes`
  proves this holds under real goroutine concurrency with `-race`).
- Reload/rebase-on-CAS-rejection (`rebaseProjectionStateFromCurrent`) already
  existed and is exercised by the pre-existing `projector_pending_authority_test.go`
  suite.

**Gap fixed:** `persistPendingRefreshLocked`'s and `applyQueueStatusEvent`'s
fixed-attempt CAS retry loops previously retried immediately with zero delay,
so a repeatedly-losing writer (e.g. a live projector racing an unsynchronized
boot reconciliation or HTTP rebuild pass, which write to the same
`CompareAndUpdateTaskStatusSummary` row without going through the projector's
lock) had no way to let the other writer settle before colliding again -
"exhausted CAS retries" only raised the attempt count, never changed the
collision dynamics. Added a small (4ms-64ms, capped) bounded exponential
backoff with up to +25% jitter (`defaultCASRetryBackoff`/`casRetryJitter`),
injected between each rejected attempt and the next retry in both loops. The
attempt bound (3) is unchanged; only the pacing between attempts changed.
`ProjectorConfig.RetryBackoff` allows tests to inject a fast/no-op backoff so
unit tests stay instantaneous while still exercising the real retry path.

**New tests** (`projector_contention_test.go`):
- `TestProjectorConcurrentSessionEventsConvergeWithoutExhaustedCAS`: 24
  goroutines apply distinct session observations for one task concurrently
  while a deterministic "alternating reject" store simulates an external
  writer (boot reconciliation/HTTP rebuild analog) racing the live projector
  on the same row. Asserts zero handler errors, that the backoff hook fires
  under contention, and that the final summary reflects every concurrently
  applied session (`ActiveSubagentCount == 24`, correct `ForegroundActivity`).
- `TestProjectorCoalescesConcurrentEquivalentPendingRefreshes`: 20 goroutines
  fire the exact same session-state event concurrently; asserts exactly one
  `SummaryUpdated` publish and one persisted revision.
- `TestDefaultCASRetryBackoffRespectsContextCancellation`,
  `TestCASRetryJitterStaysWithinBounds`, `TestDefaultCASRetryBackoffDelayGrowsAndCaps`:
  unit coverage of the backoff/jitter helper itself (bounds, cancellation,
  cap).

**Verification:**
```
go build ./...                                                     -> ok
go vet ./internal/task/...                                          -> ok
go test -race ./internal/task/statussummary -count=1 -v             -> ok, all PASS (incl. 5x repeat of new
                                                                        concurrent/backoff tests under -race)
go test ./internal/orchestrator/... -count=1                        -> ok
go test ./internal/backendapp -run "StatusSummary|PRWatch|Projector" -count=1 -> ok
gofmt -l internal/task/statussummary/projector.go internal/task/statussummary/projector_contention_test.go -> no output
golangci-lint run ./internal/task/statussummary/...                 -> 0 issues
git diff --check                                                    -> exit 0
```
Pre-existing, unrelated failures (`TestServiceInitializeLocalRepository*`,
`TestCreateDirectoryRejectsInvalidOrExistingChild`,
`TestHTTPInitializeLocalRepositoryCreatesRepository`,
`TestHTTPInitializeLocalRepositoryMapsClientErrors`,
`TestHTTPCreateDirectoryCreatesFolder`,
`TestRepositoryMutationsRejectedInImproveKandevWorkspace`) were confirmed via
`git stash` to fail identically without this change - a sandbox filesystem
permission limitation ("parent directory cannot be accessed"), not a
regression from this task.

**Deferred/out of scope:** the boot-reconciliation/HTTP-rebuild retry loop in
`internal/task/service/service_status_summary_rebuild.go` already has bounded
exponential backoff (`waitForSummaryReconcileRetry`) but no jitter; this task's
file list scoped the fix to `internal/task/statussummary`, so that loop's
jitter was left untouched to avoid broadening scope beyond this wave.

