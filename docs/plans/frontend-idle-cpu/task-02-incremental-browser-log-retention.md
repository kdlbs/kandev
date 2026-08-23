---
id: "02-incremental-browser-log-retention"
title: "Make browser-log retention incremental"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/browser-console-retention.md"
---

# Task 02: Make Browser-Log Retention Incremental

## Outcome

Browser diagnostics keep the existing exact limits. Normal append batches no
longer walk every retained row, and one tab cannot start overlapping writes.

## Scope

- Increase the existing IndexedDB schema to version 2 without changing its
  database name.
- Add transactionally maintained count and serialized-byte totals.
- Rebuild totals once for existing version-1 data.
- Combine append, expired-prefix deletion, cap eviction, and total updates in
  one transaction over both stores.
- Reset totals with entry clearing.
- Serialize scheduled drains and snapshot flushing in `runtime.ts`.
- Add `fake-indexeddb` as a development-only test dependency.
- Add store integration tests and runtime concurrency tests.

## Exclusions

- Retention limit, identity partition, entry schema, or bundle protocol changes.
- Backend changes.
- Continuous upload or retry behavior.
- New settings or user-facing copy.

## Requirements and design

- `REQ-PLATFORM-BROWSER-CONSOLE-RETENTION-001`
- `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.1` through
  `AC-PLATFORM-BROWSER-CONSOLE-RETENTION-001.7`
- `docs/specs/platform/system-design/browser-console-retention.md`
- `docs/decisions/2026-07-30-file-backed-diagnostic-bundles.md`

## Acceptance conditions

1. A version-1 database upgrades in place, preserves entries, initializes exact
   totals once, and uses incremental maintenance on later batches.
2. Appends from separate store instances preserve age, count, and byte limits
   through one shared transaction scope. Clearing entries also clears totals.
3. A held append proves that later scheduled work and a snapshot do not start a
   second append in the same tab.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
pnpm --filter @kandev/web exec vitest run lib/logger/indexeddb-store.test.ts lib/logger/runtime.test.ts --reporter=dot
cd web && pnpm run typecheck
```

## Files likely touched

- `apps/web/lib/logger/indexeddb-store.ts`
- `apps/web/lib/logger/indexeddb-store.test.ts`
- `apps/web/lib/logger/runtime.ts`
- `apps/web/lib/logger/runtime.test.ts`
- `apps/web/package.json`
- `apps/pnpm-lock.yaml`

## Dependencies

None.

## Parallelism

Sequential in the primary session. This keeps performance evidence attributable
to one change at a time.

## Inputs

- The browser console retention requirement and design.
- The amended diagnostic-bundle decision.
- Existing staging and per-batch limits in `runtime.ts`.
- Existing entry indexes and identity partition in `indexeddb-store.ts`.

## Output contract

Report the schema version, metadata record, transaction scope, cursor bounds,
maximum observed append concurrency, migrated test data, verification results,
risks, and blockers. Update this task and `plan.md` with exact results.

## Risks

- Any mutation outside the two-store transaction can make totals stale.
- An upgrade cursor must keep the upgrade transaction alive until totals are
  written.
- A failed batch must return its staged entries once and then enter the existing
  memory fallback without a retry loop.

## Results

- Upgraded `kandev-diagnostic-logs-v1` to IndexedDB schema version 2. The
  existing entries store and indexes remain unchanged, and the new metadata
  store contains one `retention` record with transactional `count` and `bytes`
  totals.
- Version-1 data rebuilds totals once during upgrade. Normal appends use one
  readwrite transaction over entries and metadata, delete the expired
  timestamp prefix, evict oldest rows only when a count or byte cap requires
  it, and commit the updated totals atomically. Clearing entries resets the
  metadata record.
- Runtime staging now owns one in-flight drain promise. Scheduled drains and
  snapshots join the same loop; the held-append test observed a maximum append
  concurrency of one.
- Added `fake-indexeddb` as a development-only dependency and coverage for
  schema initialization, upgrade totals, exact count and byte caps, retention
  ordering, identity partitioning, metadata repair, clearing, concurrent store
  instances, cursor bounds, and non-overlapping runtime drains.
- The focused store and runtime suite passed 12 tests. Frontend typecheck and
  the broader affected unit suite also passed.
- An IndexedDB upgrade blocked by an older tab now rejects promptly, clears the
  cached open promise, closes any late successful connection, and permits a
  later retry. The runtime fallback test confirms a rejected drain preserves the
  existing memory-mode behavior.
