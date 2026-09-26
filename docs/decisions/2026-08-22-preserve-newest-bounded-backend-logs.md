# ADR-2026-08-22-preserve-newest-bounded-backend-logs: Preserve Newest Bounded Backend Logs

**Status:** accepted
**Date:** 2026-08-22
**Area:** backend, infra, operations

## Context

The backend stops file logging after the active UTC-day file reaches 256 MiB.
This behavior bounds disk use, but it preserves the earliest entries and
removes later diagnostic evidence.

Issue #2929 exposed this problem through repeated normal WebSocket close
errors. A local debug instance reached the same limit through normal debug
traffic. In both cases, the useful evidence occurred after the file stopped.

## Decision

Kandev keeps at most 256 MiB across all retained backend log segments. The
three-day UTC window is a maximum age, not a guaranteed allocation for each
day. Quiet installations can retain three days. Busy installations retain a
shorter newest-first window within the same total budget.

The active file remains `backend-logs.log`. It accepts at most 16 MiB. Before
the next entry exceeds that limit, the logger closes the file as
`backend-logs-YYYY-MM-DD-NNNNNN.log` and opens a new active file.

Closed segment sequence numbers increase during one UTC day. The logger
removes the oldest closed segments across retained days when all recognized
backend logs need more than 256 MiB. The active file and newest closed segments
remain available.

At UTC midnight, the logger closes the active file as the final segment for
the prior day. It opens a new active file for the new day and removes segments
outside the current-plus-two-day window.

The segment size, total budget, UTC boundary, and maximum age are not
configurable. Files remain owner-only on supported Unix systems.

Restarts reconstruct the current day, next sequence, and total retained bytes
from recognized files and the day marker. Existing singleton files named
`backend-logs-YYYY-MM-DD.log` remain valid inputs for budget cleanup and
diagnostic bundles.

An upgrade can find an active file from the old format that is larger than one
segment. The logger converts its newest content into bounded segments before
it continues normal writes. This conversion and all rotations are
crash-recoverable.

Diagnostic bundles discover the active file, numbered segments, and legacy
singleton files. They select candidates newest-first and retain their existing
source budgets and manifest byte-range reporting.

This decision amends only the backend file-retention policy in
[ADR-2026-07-30-file-backed-diagnostic-bundles](2026-07-30-file-backed-diagnostic-bundles.md).
The bundle, privacy, and access decisions remain unchanged.

## Consequences

Backend logging continues during sustained traffic. A failure that occurs late
in a busy day remains available until newer traffic replaces its segment.

High volume can retain less than one day of evidence. Low volume can retain up
to three days. Retained backend log data uses at most 256 MiB in steady state,
which reduces the former three-day maximum by two thirds.

One day can contain multiple retained log files instead of one file. Diagnostic
collection and manual searches must recognize and order these segments.

Rotation adds filesystem rename, open, cleanup, and directory-scan operations.
These operations run on the asynchronous file-sink path and do not block
application producers. File-sink queue limits still apply during slow storage
operations.

The first upgrade from the single-file format can copy bounded tail data once.
The conversion needs temporary disk space of at most one segment and crash
recovery.

## Alternatives Considered

- **Keep the hard daily stop.** Rejected because it removes the newest evidence
  after sustained traffic.
- **Create unlimited size-rotated files.** Rejected because one busy day can
  consume unbounded disk space.
- **Use one 256 MiB rolling file.** Rejected because removing an old prefix
  requires large repeated copies or a non-text circular format.
- **Keep 256 MiB for each retained day.** Rejected because rolling segments do
  not need a reserved budget for each day. This option can use approximately
  768 MiB.
- **Use a smaller global budget.** Rejected because 256 MiB matches the old
  single-file ceiling and retains more busy-period evidence without preserving
  the old three-file maximum.
- **Use a generic rotation library.** Rejected because the existing contract
  requires UTC-day ownership, legacy discovery, owner-only files, and bundle
  ordering.
