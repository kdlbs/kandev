---
id: "01-owned-repair-store"
title: "Add the owned repair store"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-MANAGED-RUNTIME-RECOVERY-007
acceptance_criteria:
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.2
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.5
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.6
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.7
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.8
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.9
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.10
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.11
  - AC-AGENTS-MANAGED-RUNTIME-RECOVERY-007.12
system_design:
  - ../../specs/agents/system-design/managed-npm-repair-reuse.md
---

# Task 01: Add the owned repair store

## Summary

Add executor-local generation storage, context identity, publication, and
consumer-safe lifetime. Existing launch behavior remains unchanged until Task 02.

## In scope

- Proposed `RepairStore`, `RepairContext`, and generation handles under `managedruntime`.
- Bounded context/record encoding, private HMAC key, ownership and path validation.
- Stable generation allocation and atomic selection after explicit initialization success.
- Cancellable per-context Unix/Windows OS locks across agentctl processes.
- Existing private cleanup for unpublished roots; published roots survive handle closure.
- Corrupt record, symlink, permission, publication failure, and independent-owner tests.

## Out of scope

- Launch wiring, authenticated DTO changes, UI, automatic collection, and shared-tree mutation.

## Acceptance

1. `TestRepairStorePublication` first demonstrates that owner teardown loses the
   current repair. A published generation then survives a fresh owner and lookup.
   File validation alone cannot publish a generation before initialization success.
2. `TestRepairContext` rejects changed user, version, toolchain, npm configuration,
   or arbitrary installer environment input. Bounded records contain no raw secrets.
3. `TestRepairStoreConcurrency`, `TestRepairStoreInvalidation`, and lifetime cases
   prove independent contexts, cancelled waiters/builders, atomic selection, and old-consumer file safety.

## Verification

Run from the repository root. Preserve the failing behavioral regression before production changes.
Use subprocess helpers for cross-process locking; an in-memory mutex test is insufficient.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/managedruntime -run 'RepairStore|RepairContext|PreparedRuntime' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 -race ./internal/agent/managedruntime -run 'RepairStore|RepairContext|PreparedRuntime' -count=1)
```

Run native Windows lock/bin tests in the Windows verification lane.
Cross-compilation alone does not establish Windows locking or replace semantics.
Record that platform evidence before the package claims cross-platform completion.

## Files likely touched

- New `apps/backend/internal/agent/managedruntime/repair_store.go`, `repair_context.go`, and matching tests.
- New platform-specific repair lock and atomic publication helpers in that package.
- `apps/backend/internal/agent/managedruntime/prepared_runtime.go` and its lifetime tests.
- `apps/backend/internal/agent/managedruntime/cache_inventory.go`, `cache_unix.go`, and `cache_windows.go` only for shared contained validation.

## Dependencies

None. The completed incomplete-cache package supplies the preparation primitive.

## Risks

- A published handle must not fall through to recursive `PreparedRuntime.Close` cleanup.
- Replacing the directory after process start can invalidate absolute paths.
- Do not couple the new lock namespace to passthrough MCP ownership.

## Parallelism

`sequential`

## Inputs

- Design: Store and installation identity; Publication and file lifetime; Cancellation and concurrency.
- `prepared_runtime_test.go`, `prepared_runtime_consumer_test.go`, and `cache_inventory_test.go`.
- `common/passthroughconfig/lock*.go` as an existing platform lock pattern.

## Results

Implemented on 2026-11-08.

- Added `repair_context.go`, `repair_store.go`, `repair_record.go`,
  `repair_lock.go`, platform lock executable helpers, and focused tests under
  `apps/backend/internal/agent/managedruntime/`.
- `PreparedRuntime` now supports an owned-store allocation path. Published
  handles close without removing files; unpublished handles remove only their
  exact owner root after consumers stop.
- Context identity uses a private HMAC-SHA256 key and bounded records. The
  store validates owner markers, symlinks, permissions, package identity, the
  root manifest, executable containment, and record size before selection.
- Per-context locks are real OS locks (`flock` on Unix, `LockFileEx` on
  Windows) and are cancelable. Independent contexts never serialize together.
- Context identity now includes the authenticated owner scope, the effective
  sanitized environment digest, custom/global npm config files, and the npm
  toolchain. Selection and generation records are HMAC-signed so a replaced or
  checkout-controlled identity file cannot authorize execution.
- Store roots reject configured workspace/source exclusions, foreign ownership,
  symlinked or oversized identity files, and non-private permissions. The
  owner marker and fingerprint key are read through bounded no-follow regular
  files.
- Publication failure retires the ticket before releasing its lock, and
  `RepairPublished` reads its state through the repair mutex.
- Tests: `go test -trimpath -tags fts5 ./internal/agent/managedruntime -run
  'RepairStore|RepairContext|PreparedRuntime' -count=1` passed;
  the same selection passed with `-race`, including concurrent publication
  observation, tampered-record, owner-boundary, and unsafe-identity-file
  cases.
- Cross-process locking is covered by a `go test` helper subprocess.
- Windows-native lock and replace evidence remains pending in the Windows
  verification lane.
