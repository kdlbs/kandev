---
created: 2026-09-05
status: complete
requirements:
  - REQ-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002
system_design:
  - ../../specs/system-page/system-design/storage-analysis-progress.md
legacy_specs: []
---

# Implementation Plan: Progressive Storage Analysis

## Overview

Reduce scan time with bounded filesystem concurrency. Then make overview reads nonblocking and
stream coarse progress to the Storage page.

The order protects the cache contract. The scanner lands first, the backend state contract follows,
and the web work consumes that contract.

## Scope

### In scope

- Bounded, context-aware byte measurement for read-only storage analysis.
- Nonblocking overview reads with stale-while-revalidate behavior.
- Coarse progress notifications with a polling fallback.
- First-scan partial results and atomic replacement of older snapshots.
- Scan duration, cache lifetime, and next-refresh details.
- Desktop and phone access to the timing disclosure.
- Focused public operator documentation.

### Out of scope

- Changes to destructive cleanup or quarantine rules.
- A persistent overview snapshot across backend restarts.
- A user-configurable scan worker count or cache lifetime.
- File-level progress events or exact time-remaining estimates.
- Remote-executor disk analysis.
- Automatic scans when no client reads the Storage overview.

## Technical approach

### Bounded filesystem measurement

Add `apps/backend/internal/system/storage/filescan`. Its shared limiter will permit four active
directory partitions across workspace, Go-cache, and temporary-artifact analysis.

Partition each root by immediate children. Preserve provider-specific symlink rejection, marker
exclusion, warning order, and path ownership.

Inject the limiter through `storageOverview` provider construction. Keep destructive provider paths
on their current serial code.

### Progressive cache and API

Extend `OverviewCache` with process-local analysis state and a nonblocking read. Keep `Refresh` as
the tracked manual path and join every concurrent request to one scan generation.

Add source progress callbacks to `storageOverview`. Keep the last successful `Summary` atomic and
store cold-scan partial results separately.

Extend `GET /api/v1/system/storage` with nullable first-scan snapshot fields and an `analysis`
object. Include scan timestamps, duration, cache lifetime, refresh time, source progress, and a
partial summary.

Publish `system.storage.analysis.updated` with generation and state only. Add the event constant,
gateway action, subscription, and frontend revision field.

### Web state and presentation

Update storage API types and the Zustand system slice for analysis state and revision ordering.

Refactor `useStorageMaintenance` so WebSocket revisions reload only the overview. Poll every 1.5
seconds while analysis is active and stop in a terminal state.

Schedule one read at `refresh_due_at` while the Storage page remains mounted. Preserve request
generation checks and terminal Analyze refresh behavior.

Update `StorageOverviewCard` to show cold-scan source progress and counted-so-far totals. Keep a
stale completed snapshot visible during revalidation.

Add an information icon beside the completed snapshot time. Reuse `StorageSettingHelp` interaction
and permit structured content for the three timing values.

### Documentation and observability

Update `docs/public/operations.md` with the asynchronous scan, 15-minute cache, and Analyze behavior.

Add structured scan completion logs and storage scan metrics. Do not include task paths in logs or
metrics.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.1`, `.10` | Cache and handler tests block the provider and prove immediate, single-flight reads. |
| `.2`, `.3` | Provider, cache, hook, and card tests publish sources progressively and mark partial totals. |
| `.4`, `.9` | Cache tests keep the last snapshot through expired and failed refreshes. |
| `.5` | Event, hook, and fake-timer tests cover WebSocket refresh and the 1.5-second fallback. |
| `.6`, `.7` | Component and Playwright tests cover timing values, focus, hover, tap, and touch size. |
| `.8` | Hook fake-timer tests request at `refresh_due_at`. Analyze tests retain forced refresh. |

The filesystem scanner tests will cover exact bytes, no-follow behavior, deterministic warnings,
cancellation, and a four-worker ceiling.

The provider tests will use barriers instead of elapsed-time assertions. They will prove that two
partitions enter before either partition is released.

## E2E tests

Update `apps/web/e2e/tests/system/storage-maintenance.spec.ts` for progressive cold-scan values,
stale refresh, and pointer or keyboard timing disclosure.

Update `apps/web/e2e/tests/system/mobile-storage-maintenance.spec.ts` for tap disclosure, a 44-pixel
trigger, progressive values, and no horizontal overflow.

Both flows map to `AC-SYSTEM-PAGE-STORAGE-OVERVIEW-PARALLEL-SCAN-002.2` through `.8`.

## Mobile design contract

Desktop and phone users enter through `/settings/system/storage`. Both receive the same snapshot,
progress, and timing data.

The existing Storage page and `StorageSettingHelp` are the nearest shipped examples. They provide
the one-column card flow and the hover, focus, and pinned-tap help interaction.

Storage analysis remains an inline card in the page scroll. No drawer or full-height surface is
necessary because the timing content contains three short values.

The information trigger will use the existing 44-pixel phone hit area and compact desktop size.
The page keeps one scroll owner and no horizontal overflow.

Business state, progress derivation, and refresh scheduling remain shared across viewports. Only the
responsive trigger size differs.

## Work orders

- [x] [Task 01: Build bounded filesystem scanner](task-01-bounded-filesystem-scanner.md)
- [x] [Task 02: Add progressive overview state](task-02-progressive-overview-state.md)
- [x] [Task 03: Render live analysis details](task-03-live-analysis-ui.md)
- [x] [Task 04: Prove progressive storage behavior](task-04-storage-progress-e2e.md)

## Dependency order

1. Task 01 creates the scan and progress primitives.
2. Task 02 uses those primitives for the cache, API, events, and observability.
3. Task 03 consumes the backend contract and updates public operator documentation.
4. Task 04 proves the integrated desktop and phone behavior.

All tasks are sequential. They share contracts, fixtures, and storage test files.

## Verification results

- Task 01: `go test -race ./internal/system/storage/filescan ./internal/system/storage/workspaces ./internal/system/storage/gocache ./internal/system/storage/tempartifacts ./internal/backendapp` passed (853 tests).
- Task 01 benchmark: `go test ./internal/system/storage/filescan -run '^$' -bench 'BenchmarkMeasureTrees' -benchmem` passed at 81,604 ns/op and 11,367 B/op.
- Task 02: `go test -race ./internal/system/storage ./internal/backendapp ./internal/gateway/websocket` passed (1,423 tests).
- Task 03: the focused web suite passed (46 tests), the extracted terminal-refresh suite passed (2 tests), typecheck, lint, i18n checks, and public-doc validation passed.
- Task 04: `make build-web`, `make build-backend`, Chromium storage E2E (8 tests), and mobile Chromium storage E2E (6 tests) passed.
- Changed-file Go lint passed with `golangci-lint run ./... --new-from-rev=481359ddb282fd7d3e8c9cdff99da6f9bdd24c7c --timeout=5m`.
- Specification lint passed with `python3 scripts/lint-spec-files.py --all`.

## Risks

- Parallel directory reads can increase contention on rotating disks. The shared four-walker limit
  bounds this risk, and timing evidence will record regressions.
- Partial results can look final. The API separates them from completed snapshots, and the UI marks
  aggregates as counted so far.
- Late progress can corrupt a new scan. Generation checks must guard every progress and completion
  write.
- Broadcast events can disclose storage facts. The event carries only generation and state.
- A missed event can leave stale UI. The active-scan poll provides recovery.
- A page timer can drift after sleep. The timer callback reads the server state instead of assuming
  that a scan started.
