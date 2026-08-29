---
status: active
system: platform
created: 2026-08-23
updated: 2026-08-28
owners:
  - kandev
---
# Expected runtime log severity Requirements

## Overview

Expected runtime conditions and forwarded child logs can use the wrong severity.
This behavior obscures actionable errors and makes diagnostic bundles harder to triage.

## Requirements

### REQ-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001: Expected runtime log severity

**Intent:** Normal review and first-launch timing conditions currently look like backend failures in the logs. This obscures actionable errors and makes diagnostic bundles harder to triage.

#### Acceptance criteria

- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.1:** A `workspace.file.get` request for a path that is absent from the current checkout returns the existing `not_found` WebSocket error code and records a debug entry. It does not record an error entry.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.2:** A `workspace.file.get` failure that is not a missing-file condition keeps the existing `internal_error` response and error-level log entry.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.3:** When initial worktree materialization runs before its task environment exists, the typed `ErrEnvironmentNotResolved` condition records a debug entry. The physical worktree remains available and the existing launch transaction still owns persistence of its record.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.4:** Other worktree persistence failures keep their existing return, cleanup, and logging behavior.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.5:** **GIVEN** an active session and an absent current-checkout path, **WHEN** `workspace.file.get` requests that path, **THEN** the response has the `not_found` error code and exactly one debug-level missing-file entry is emitted.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.6:** **GIVEN** an active session and a dependency failure unrelated to a missing file, **WHEN** `workspace.file.get` requests a path, **THEN** the response has the `internal_error` error code and an error-level entry is emitted.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.7:** **GIVEN** a physical worktree has been materialized before its task environment row exists, **WHEN** worktree persistence receives `ErrEnvironmentNotResolved`, **THEN** the call succeeds, emits a debug-level deferred-persistence entry, and does not remove the physical worktree.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.8:** **GIVEN** worktree persistence receives any other store error, **WHEN** the manager handles it, **THEN** the existing error and cleanup behavior remains unchanged.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.9:** The launcher shall preserve a recognized child log level from Zap console records and Go `slog` text records.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.10:** The launcher shall record unrecognized child stderr at warning level so that unstructured panics and tracebacks remain visible.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.11:** A missing task environment referenced by a session shall use the existing task fallback without a warning entry.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.12:** A task-environment lookup error that is not a typed not-found condition shall retain its warning entry.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.13:** **GIVEN** a child emits a recognized `INFO` record, **WHEN** the launcher forwards stderr, **THEN** the parent records the line at info level.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.14:** **GIVEN** a child emits unstructured stderr, **WHEN** the launcher forwards the line, **THEN** the parent records it at warning level.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.15:** **GIVEN** a session references a missing task environment, **WHEN** workspace information is loaded, **THEN** the task fallback succeeds without a warning entry.
- **AC-PLATFORM-EXPECTED-RUNTIME-LOG-SEVERITY-001.16:** **GIVEN** a task-environment lookup returns another error, **WHEN** workspace information is loaded, **THEN** the existing fallback and warning behavior remains unchanged.

## Migrated source detail

## Why

Normal review and first-launch timing conditions currently look like backend
failures in the logs. This obscures actionable errors and makes diagnostic
bundles harder to triage.

## What

- A `workspace.file.get` request for a path that is absent from the current
  checkout returns the existing `not_found` WebSocket error code and records a
  debug entry. It does not record an error entry.
- A `workspace.file.get` failure that is not a missing-file condition keeps the
  existing `internal_error` response and error-level log entry.
- When initial worktree materialization runs before its task environment exists,
  the typed `ErrEnvironmentNotResolved` condition records a debug entry. The
  physical worktree remains available and the existing launch transaction still
  owns persistence of its record.
- Other worktree persistence failures keep their existing return, cleanup, and
  logging behavior.
- Forwarded child stderr keeps recognized Zap and Go `slog` levels. Unknown
  stderr stays at warning level.
- A missing session task environment uses the existing task fallback without a
  warning. Other lookup errors keep their warning.

## API surface

The existing `workspace.file.get` WebSocket action keeps its request and
response shapes. Only the response classification for a missing current file
is made explicit as `not_found`; no new action or payload field is added. The
agentctl file-content endpoint carries this classification with HTTP 404 and a
typed client sentinel. Other file-content failures keep their existing error
status and message path.

## Failure modes

- Missing files are expected during deleted-file review and stale diff
  requests. They are handled as not-found results and are not promoted to
  application failures.
- Permission, transport, malformed-response, and other non-missing failures
  remain internal errors and continue to be logged at error level.
- A worktree store that reports `ErrEnvironmentNotResolved` is a normal launch
  ordering condition. The manager skips that persistence attempt and does not
  remove the newly created physical worktree.
- Any other worktree store failure remains an actual persistence failure and
  follows the current cleanup and retry boundary.
- The launcher accepts only anchored child log formats. Unrecognized stderr
  remains visible at warning level.
- A typed missing task environment is an expected stale-reference condition.
  The task fallback remains authoritative for workspace information.
- Other task-environment lookup errors can indicate a storage problem and
  remain warnings.

## Persistence guarantees

This repair changes log severity and missing-file classification only. The
agentctl file-content transport now distinguishes missing files with HTTP 404;
it does not change task-environment ownership, worktree creation, launch
persistence, or cleanup behavior across backend restarts.

## Scenarios

- **GIVEN** an active session and an absent current-checkout path, **WHEN**
  `workspace.file.get` requests that path, **THEN** the response has the
  `not_found` error code and exactly one debug-level missing-file entry is
  emitted.
- **GIVEN** an active session and a dependency failure unrelated to a missing
  file, **WHEN** `workspace.file.get` requests a path, **THEN** the response
  has the `internal_error` error code and an error-level entry is emitted.
- **GIVEN** a physical worktree has been materialized before its task
  environment row exists, **WHEN** worktree persistence receives
  `ErrEnvironmentNotResolved`, **THEN** the call succeeds, emits a debug-level
  deferred-persistence entry, and does not remove the physical worktree.
- **GIVEN** worktree persistence receives any other store error, **WHEN** the
  manager handles it, **THEN** the existing error and cleanup behavior remains
  unchanged.
- **GIVEN** a child emits a recognized `INFO` record, **WHEN** the launcher
  forwards stderr, **THEN** the parent records the line at info level.
- **GIVEN** a child emits unstructured stderr, **WHEN** the launcher forwards
  the line, **THEN** the parent records it at warning level.
- **GIVEN** a session references a missing task environment, **WHEN** workspace
  information is loaded, **THEN** the task fallback succeeds without a warning
  entry.
- **GIVEN** a task-environment lookup returns another error, **WHEN** workspace
  information is loaded, **THEN** the existing fallback and warning behavior
  remains unchanged.

## Out of scope

- Changing frontend diff rendering or review-file status behavior.
- Changing worktree ownership, task-environment transactions, cleanup policy,
  or startup reconciliation.
- Reclassifying integration failures, raw ACP frames, host-utility idle repair,
  network disconnects, or authentication warnings.
