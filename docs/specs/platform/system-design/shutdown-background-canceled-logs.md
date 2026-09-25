---
status: current
system: platform
requirements:
  - REQ-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001
created: 2026-09-16
owners:
  - cfl12
---
# Background subsystem context-cancellation log severity System Design

## Purpose and boundaries

The platform system owns backend shutdown observability. This design covers the
three background subsystems whose in-flight work is bound to the root context
and can therefore observe `context.Canceled` during graceful shutdown: the
GitHub PR-watch service, the agent profile reconciler, and the host-utility
profile migration. It changes only how those sites classify log severity. It
does not change control flow, returned errors, retries, or lifecycle
sequencing. The session, orchestrator, plugin, and launcher sites remain owned
by [Quiet benign teardown log noise on shutdown](../requirements/shutdown-log-noise.md).

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-PLATFORM-SHUTDOWN-BACKGROUND-CANCELED-LOG-001` | [Components and responsibilities](#components-and-responsibilities) and [Failure and recovery](#failure-and-recovery) |

## Components and responsibilities

- `github.Service.logSyncError` classifies a PR-watch sync error: it appends
  `zap.Error(err)`, records `DEBUG` when the error wraps `context.Canceled`, and
  otherwise records `ERROR`. The single and batched PR-watch sync sites route
  their store failures through it.
- `settings/controller.ProfileReconciler.logReconcileError` classifies a
  reconcile store error the same way, recording `DEBUG` for a wrapped
  `context.Canceled` and otherwise `WARN`. The reconcile, orphan-cleanup, and
  profile-heal sites route their store failures through it.
- `backendapp` classifies the host-utility profile migration result inline,
  recording `DEBUG` when the migration error wraps `context.Canceled` and
  otherwise `WARN`.

Each helper mirrors the existing `github.Poller.logCleanupError` precedent so
the three subsystems share one classification shape.

## Data and contracts

No HTTP, WebSocket, database, or agent-protocol payloads change. The only
observable change is the severity and message suffix of the affected log
entries; a demoted entry appends `(context canceled during shutdown)` to its
message and carries the same structured fields, including the original error.

## Control flow

1. A background subsystem issues a root-context-bound store or lookup call.
2. During graceful shutdown the root context is cancelled and the call returns
   an error that wraps `context.Canceled`.
3. The site passes the error to its classification helper (or inline check).
4. `errors.Is(err, context.Canceled)` selects `DEBUG`; any other error keeps the
   original `ERROR` or `WARN` level.

## Failure and recovery

Classification is conservative. Only `context.Canceled` is treated as benign
teardown. `context.DeadlineExceeded` and every other error keep their original
severity, matching the existing convention, so a real fault during shutdown is
never hidden. Because only the log level changes, retries, returned errors, and
cleanup paths are unchanged.

## Persistence

None. This design changes log severity only. It does not change store
transactions, migrations, or restart behavior.

## Security

No permission, trust-boundary, or sensitive-data handling changes. The demoted
entries carry the same fields as before.

## Observability

The three subsystems no longer emit `ERROR`/`WARN` lines for expected
root-context cancellation during shutdown; those become `DEBUG`. Genuine faults,
including timeouts, remain at their original level. This restores agreement
between a clean shutdown's log output and the supervisor's `error_count: 0`.

## Related decisions

None.
