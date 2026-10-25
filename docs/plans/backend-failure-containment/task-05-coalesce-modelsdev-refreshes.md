---
id: "05-coalesce-modelsdev-refreshes"
title: "Coalesce models.dev refreshes"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/office/requirements/costs.md"
---

# Task 05: Coalesce models.dev refreshes

## Intent

Preserve stale-while-revalidate pricing while ensuring that one client performs
only one physical refresh and never collides on a shared temporary cache file.

## Acceptance

- Concurrent stale `LookupForModel` calls start one network request.
- Mixed concurrent `LookupForModel` and `LookupModelInfo` calls start one network
  request.
- Concurrent direct `Refresh` calls wait for and receive one shared result.
- Background and direct refresh paths use the same per-client guard.
- Failed and canceled refreshes release the guard so a later refresh can retry.
- Existing valid disk and memory cache data remains readable after failure.
- Atomic replacement uses a unique same-directory temporary file and leaves no
  temporary file after success, failure, or cancellation.
- `loadedAt` and the active in-memory indexes change only after the new cache
  file is committed and the complete in-memory replacement succeeds.

## TDD sequence

1. Add channel-controlled HTTP server tests for concurrent homogeneous lookups,
   mixed lookup methods, and direct `Refresh` calls. Assert one request and
   observe RED on the current unguarded paths.
2. Add failed-then-successful and canceled-then-successful tests. Assert stale
   data remains available and the second request runs.
3. Add filesystem assertions that no cache temporary files remain and that the
   prior valid cache survives a failed write/fetch.
4. Add a per-client `singleflight.Group`, route every refresh through it, and
   extract one physical refresh helper.
5. Replace the fixed `.tmp` path with a unique same-directory temporary file,
   deferred cleanup, and atomic rename. Update freshness only after commit and
   memory swap.
6. Run the package with the race detector, then refactor only after GREEN.

## Files likely touched

- `apps/backend/internal/office/costs/modelsdev/client.go`
- `apps/backend/internal/office/costs/modelsdev/client_test.go`

## Dependencies

None. `golang.org/x/sync/singleflight` is already a repository dependency.

## Parallelism

`parallel-safe` with Tasks 01, 02, and 06. This task owns only the models.dev
cost client package.

## Verification

- `cd apps/backend && go test -race ./internal/office/costs/modelsdev`
- `cd apps/backend && golangci-lint run ./internal/office/costs/modelsdev --timeout=5m`

## Inputs

- Observed cache rename `ENOENT` at backend log lines 37734 and 22060.
- Current `warmFromDisk`, `maybeRefresh`, `refreshSafe`, `Refresh`, and
  `writeCacheAtomic` paths.
- Existing repository uses of `singleflight.Group`.

## Output contract

Record the initial duplicate request or race evidence, final request counts,
retry/cancellation results, temporary-file assertions, and focused race/lint
results.

## Results

Added channel-controlled RED coverage that reproduced duplicate concurrent
fetches (8 direct requests and duplicate background lookup refreshes), then
coalesced every refresh path through one per-client `singleflight.Group`.
Refresh failures and cancellation now release the guard for a later retry;
valid disk and in-memory data remain intact. Cache replacement writes a unique
same-directory temporary file, syncs and closes it before rename, and defers
cleanup on every path. Indexes and `loadedAt` are swapped only after commit.

Focused race verification passed:

```text
go test -race ./internal/office/costs/modelsdev
51 passed in 1 package
```
