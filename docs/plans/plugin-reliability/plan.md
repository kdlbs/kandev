---
status: implemented
created: 2026-08-02
owner: cfl
spec: docs/specs/plugins/requirements/plugins.md
---

# Plugin reliability repair

## Goal

Make plugin failures recoverable and diagnosable from Settings, and make the
event-buffer overflow signal useful under sustained failure. The repair covers
the installed-plugin symptoms found in `~/kandev/logs.txt`: persisted `error`
states with no reason, an unavailable retry action, and one warning per dropped
buffered event. This remediation also closes the review regressions around
concurrent recovery, diagnostic secrecy, authoritative retry refresh, and phone
layout safety.

## Evidence and root cause

- The backend state machine already permits `error -> active`, but both plugin
  settings surfaces only offer **Enable** for `registered` and `disabled`.
- The plugin record persists status but no failure reason or timestamp, so a
  restart leaves the operator with an opaque `Error` badge.
- Runtime health callbacks carry only healthy/unhealthy state, so health and
  restart failures cannot be persisted with their originating error.
- The delivery ring buffer intentionally retains `error` plugins, but its
  overflow path logs every dropped event. The current log contains a sustained
  stream of Kandy overflow warnings.
- Restart exhaustion publishes `gaveUp` before its final synchronous status
  callback returns. A concurrent `claimStart` holds `Manager.mu` while
  `p.stop()` waits for that callback, which can re-enter `RestartCount` and wait
  forever on `Manager.mu`. A barrier repro in the runtime package fails on the
  current code.
- `handleEnable` catches a failed POST without refetching the backend record, so
  the store keeps the previous `last_error` even though the service persisted a
  replacement diagnostic.
- `normalizePluginError` only collapses whitespace and truncates raw
  `err.Error()`, allowing plugin stdout and credential-like subprocess details to
  become durable API data.
- Phone recovery controls use `min-h-9` (36px), and the diagnostic container has
  no wrapping rule for an unbroken bounded token.

## Behavioral changes

1. Persist a bounded, single-line `last_error` (maximum 2048 bytes) and UTC
   `last_error_at` for failed spawn, handshake, health, restart, and install-path
   checks. Do not persist configuration values or secrets. Clear both fields
   after a successful handshake and health check.
2. Include those nullable fields in the existing plugin list/detail API response.
   Keep old records compatible when the fields are absent.
3. Treat `error` as a recoverable state in both desktop and mobile plugin
   settings: show the diagnostic when present and expose **Enable** as the manual
   retry action. A failed retry replaces the diagnostic; a successful retry clears
   it. Boot must not automatically retry persisted `error` records.
4. Keep ring-buffer capacity and TTL unchanged. Emit at most one overflow warning
   per plugin per 60 seconds and include the number of drops aggregated since the
   previous warning.
5. A manual Enable concurrent with restart exhaustion must not deadlock. The
   manager must not hold its registry mutex while waiting for a process to stop;
   the final exhaustion callback and replacement start must both be allowed to
   finish.
6. Failed Enable must refetch the authoritative plugin record and upsert it so a
   replacement diagnostic is visible immediately. Recovery controls must be at
   least 44px high on phone layouts, and diagnostic text must wrap without page
   overflow.
7. Persisted diagnostics must redact PATs, bearer tokens, labeled secrets, and
   host home paths before the 2048-byte bound is applied.

## Implementation design

### Backend failure diagnostics

- Extend `plugins/store.Record` with nullable `LastError` and `LastErrorAt`
  fields, preserving YAML/JSON compatibility for existing records.
- Add registry/service helpers that atomically update status and diagnostic data,
  roll back the in-memory record when persistence fails, and normalize errors to a
  safe single-line bounded message. Redact credential-like values and home paths
  before truncation.
- Change runtime status callbacks to carry the triggering error for unhealthy
  transitions. Pass the ping/restart/exit cause through `runtime.Manager` and
  `runtime.Process`; healthy callbacks clear the diagnostic.
- Record failures from initial activation, boot activation, config-change restart,
  health transitions, restart exhaustion, and missing install paths. Preserve the
  last diagnostic while disabled; clear it only on successful recovery.
- Keep `Manager.mu` out of process-stop waits and add a barrier regression test
  covering final restart exhaustion against concurrent manual Enable.
- Add unit tests for serialization compatibility, truncation/newline handling,
  every service transition, and runtime callback propagation.

### Delivery overflow diagnostics

- Add a per-worker overflow log accumulator with an injectable clock. The first
  drop logs immediately; subsequent drops within the 60-second interval are
  counted without logging; the first drop after the interval logs the aggregate
  count and starts a new interval.
- Keep the existing oldest-drop behavior, TTL purge, recovery flush, delivery
  ordering, and queue semantics unchanged.
- Add deterministic delivery tests asserting warning counts and `dropped_count`
  aggregation for each plugin independently.

### Frontend recovery

- Allow `error` in `canEnable` for both `PluginRow` and `PluginDetail`.
- Render a compact, accessible diagnostic region from `last_error` in the row and
  detail views without adding a new untranslated label; preserve the existing
  status badge and responsive action layout.
- Clear diagnostic fields in the optimistic enable action so the UI does not show
  stale failure text after a successful retry.
- On failed Enable, fetch `GET /api/plugins/{id}` with `no-store` and upsert the
  returned record before showing the toast.
- Use `min-h-11 sm:min-h-0` for phone recovery controls and add wrapping to the
  diagnostic region.
- Add component/action tests and a Playwright recovery flow. Exercise the same
  settings path at a phone viewport to preserve mobile parity.

### Documentation

- Update `docs/public/plugins.md` with the error-state retry and diagnostic
  behavior, and the aggregated overflow-warning policy.
- Run the public-doc validation scripts and the targeted backend/frontend tests.

## Task order

1. `task-01-backend-diagnostics.md` — record contract, runtime propagation, and
   lifecycle persistence.
2. `task-02-overflow-logging.md` — per-plugin rate-limited aggregation and tests.
3. `task-03-frontend-recovery.md` — retry action, diagnostics, mobile coverage,
   and frontend tests.
4. `task-04-docs-and-verification.md` — public docs and final targeted/full gates.

Tasks 01 and 02 are backend-only and can be implemented sequentially in this
session. Task 03 depends on the API fields from task 01. Task 04 follows the
behavioral implementation.

## Acceptance criteria

- A failed plugin record exposes a useful, bounded `last_error` and timestamp
  after backend restart; a successful manual Enable clears both.
- An errored plugin can be retried from both desktop and mobile settings, and a
  failed retry remains visibly errored with the new reason after an authoritative
  refetch; the runtime deadlock regression test completes under a barrier.
- Persisted diagnostic tests prove PAT, bearer, labeled-secret, and home-path
  redaction, while retaining useful non-secret error context.
- Phone recovery targets measure at least 44px and a long diagnostic does not
  create horizontal overflow.
- Sustained ring-buffer overflow produces no more than one warning per plugin per
  minute, with the suppressed-drop count reported on the next warning.
- Existing lifecycle, buffering, delivery, install, and plugin settings tests
  remain green; public plugin docs describe the new operator behavior.

## Out of scope

- Automatic boot retry for persisted `error` records.
- Changing the 100-event/5-minute ring-buffer policy or making the buffer durable.
- Per-plugin event admission rate limiting, package signing, or unrelated plugin
  startup failures.

## Validation

- Backend targeted tests for `internal/plugins/store`, `internal/plugins`,
  `internal/plugins/runtime`, and `internal/plugins/delivery`.
- Web plugin component/action tests and typecheck/lint.
- Plugin Playwright tests, including a phone viewport recovery flow.
- `node --test scripts/validate-public-docs.test.mjs` and
  `node scripts/validate-public-docs.mjs`.
- Required backend format/typecheck/test/lint gates before handoff.
