---
spec: docs/specs/platform/diagnostic-logging.md
created: 2026-08-22
status: done
---

# Implementation Plan: Bounded Rolling Backend Logs

## Overview

Issue [#2929](https://github.com/kdlbs/kandev/issues/2929) exposed two related
diagnostic failures. The gateway logs normal WebSocket close code `1000` as an
error with a stack trace. The backend then stops file logging after the current
UTC-day file reaches 256 MiB, so later failures are absent from diagnostics.

The repair classifies normal closes correctly and replaces the daily hard stop
with bounded size segments. All retained backend logs share one 256 MiB budget.
Three UTC days is a maximum age, not a reserved budget for each day. A 16 MiB
active segment and oldest-segment eviction preserve the newest evidence while
reducing the former maximum by two thirds.

## Confirmed Root Causes

`Client.ReadPump` calls `websocket.IsUnexpectedCloseError` in
`apps/backend/internal/gateway/websocket/client.go`. Its expected-code list
omits `websocket.CloseNormalClosure` (`1000`). Gorilla therefore classifies a
normal browser close as unexpected, and the error-level Zap entry includes a
stack trace.

`dailyWriter.Write` in
`apps/backend/internal/common/logger/daily_writer.go` returns
`errDailyBackendLogLimit` when the active file reaches 256 MiB. The retrying
sink does not rotate or reactivate for this error. File logging stays stopped
until the UTC day changes.

A focused real WebSocket reproduction confirmed one error entry and stack for
code `1000`. The existing writer regression confirmed the hard daily stop. The
local debug instance also reached 256 MiB without this specific error burst,
which shows that the retention failure is independent of the reported noise.

## Backend Design

### Gateway close classification

- Add `websocket.CloseNormalClosure` to the expected close-code list in
  `Client.ReadPump`.
- Keep the current expected handling for codes `1001`, `1005`, and `1006`.
- Keep error-level stack traces for close codes outside the expected list.

### Segment storage

- Keep `backend-logs.log` as the active file and cap it at 16 MiB.
- Close full segments as `backend-logs-YYYY-MM-DD-NNNNNN.log` with an
  increasing six-digit sequence for each UTC day.
- Keep at most 256 MiB across the active file and all retained closed segments.
- Remove the oldest closed segments across retained days before opening space
  for newer entries.
- At UTC midnight, close the prior day's active segment and start a new active
  file for the current day.
- Reconstruct the active day, retained bytes, and next sequence at startup.
- Keep legacy `backend-logs-YYYY-MM-DD.log` files eligible for retention and
  bundles. Count them toward the global budget.
- Convert an oversized active file from the old format into bounded segments,
  preserving its newest content before accepting new writes. Process outputs
  newest-first, sync each output, truncate the source, and journal progress so
  migration I/O stays linear.
- Preserve owner-only file permissions and crash recovery for size rotation,
  day rotation, and legacy conversion.
- Keep segment metadata and filesystem operations in a focused companion file
  so the writer remains within backend function and file-size limits.

Rotation errors use the existing retrying sink behavior. The writer preserves
the existing files, does not exceed the total budget, records sink loss, and
retries activation after 30 seconds. The old permanent
`errDailyBackendLogLimit` path is removed.

### Diagnostic bundles

- Discover the active file, numbered segments, and legacy singleton files.
- Restrict candidates to the current UTC day and the two preceding UTC days.
- Order candidates by day, sequence, and active-file recency from newest to
  oldest before applying the existing archive source budget.
- Keep the current symlink, regular-file, byte-range, manifest, and privacy
  rules. Open each candidate once without following the final path component,
  use the opened handle for size and copying, and mark a disappeared candidate
  as partial.

## Tests

Implementation follows red-green-refactor in each task.

- Gateway integration tests send real normal and unexpected close frames and
  inspect an observed Zap core.
- Writer tests use small injected segment and total limits. They cover size
  rotation, continued writes after a full budget, cross-day oldest eviction, a
  later marker remaining readable, UTC rollover, restart sequencing, maximum
  age, legacy input, oversized active-file conversion, and crash recovery.
- Failure tests verify that rename, removal, or open failures do not replace
  existing files or exceed the configured test budget.
- Bundle tests cover numbered-segment discovery, newest-first ordering, legacy
  compatibility, archive truncation, symlink exclusion, future dates, and a
  rotation during collection.

No browser end-to-end test is required. The behavior changes only backend log
classification, storage, and collection.

## Public Documentation

The implementation updates the backend log-retention descriptions in:

- `docs/public/operations.md`
- `docs/public/configuration.md`
- `docs/public/docker.md`
- `docs/public/k8s.md`

The pages must explain that high-volume days keep the newest bounded window,
list the active and numbered filenames, and preserve the existing disk-budget
guidance. `docs/public/cli.md` needs no change because its current text only
identifies the log directory.

## Verification Results

- `cd apps/backend && go test ./internal/gateway/websocket -run
  TestReadPumpCloseLogging -count=1` passed.
- `cd apps/backend && go test ./internal/common/logger -run
  'TestDailyWriter|TestBackendLogSegment|TestRetryDailyWriter' -count=1` passed.
- `cd apps/backend && go test ./internal/system/logbundle -run
  'TestBackendOnlyBundle|TestBackendCandidates' -count=1` passed.
- `node --test scripts/validate-public-docs.test.mjs` passed all 61 tests and
  `node scripts/validate-public-docs.mjs` validated 41 pages.
- Affected package tests passed: logger 31, WebSocket 520, logbundle 22.
- `golangci-lint run ./... --new-from-rev="origin/main" --timeout=5m` passed
  with no issues. Changed Go files are gofmt-clean.

## Implementation Waves And Parallel Candidates

Wave 1:

- [x] [Task 01: Classify normal WebSocket closes](task-01-classify-normal-websocket-closes.md)
- [x] [Task 02: Implement bounded log segments](task-02-implement-bounded-log-segments.md)

Tasks 01 and 02 touch separate packages and are parallel candidates. The
default execution order remains sequential unless the user authorizes
delegation.

Wave 2, after Task 02:

- [x] [Task 03: Extend diagnostic bundle discovery](task-03-extend-diagnostic-bundle-discovery.md)

Wave 3, after Tasks 02 and 03:

- [x] [Task 04: Update public logging documentation](task-04-update-public-logging-documentation.md)

## Risks And Out Of Scope

- The close-code change must not suppress unexpected protocol or application
  closes.
- Upgrade conversion must survive interruption without duplicating, replacing,
  or losing the newest bounded content.
- Segment discovery must ignore unrelated files, malformed names, directories,
  and symlinks.
- Whole-segment eviction can retain slightly less than 256 MiB. It must never
  retain more than the total budget in steady state.
- This repair does not change browser reconnection behavior.
- This repair does not increase the 256 MiB total budget or three-day maximum
  age.
- This repair does not add a user-configurable rotation policy.
