---
status: current
system: platform
requirements:
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
---

# Startup lifecycle design

## Ownership and flow

Platform owns this shared backend lifecycle. Configuration, runtime-state ownership,
logging, and startup security validation precede the bootstrap listener.
The listener precedes event-bus and database initialization. One handler switch,
HTTP server, and listener set survive through application-router installation.
Only the bootstrap handler serves traffic during initialization.

`backendapp.run` owns the listener and cancellation lifetime. Startup state travels
through context to persistence and router composition. Initialization remains
sequential; cleanup must not race constructors or database mutation.
Signal handling begins before database work. The first signal cancels startup;
the existing second-signal forced-exit behavior remains available.

The bootstrap owns separate process and worker cancellation contexts. A startup
signal cancels the process context and closes the bootstrap listener while the
initializer drains. Restore quiescing cancels only worker and runtime contexts;
the process context, HTTP listener, and job tracker remain alive until restore
has installed the staged database and published `restart_required`, after
which an explicit shutdown can close the process.

## Readiness and status

`/health` retains its existing body and desktop token header. Bootstrap `/ready`
returns 503 with a `startup` object containing a fixed phase name, total elapsed
milliseconds, and phase elapsed milliseconds. No paths, SQL, credentials, or raw
errors enter this unauthenticated payload. Application routes remain 503.

Phases cover database opening, backup, migrations, service initialization,
required session recovery, and ready. Structured logs record transitions and
phase duration. Launcher readiness polling reads JSON, reports transitions and
periodic elapsed status, and remembers the last phase for child-exit diagnostics.
Underlying errors remain in backend diagnostic output. Readiness waits retain
cancellation and child-exit checks without a separate deadline.

Router publication occurs only after required stores, recovery gates, final
persistence checks, and router construction succeed. The existing background
lifecycle-token sweep remains outside readiness: its watcher/scheduler ordering
and deadline are unchanged.

## Timeout policy

Keep release listener-health timeout at 45000 ms, development/E2E at 600000 ms.
Keep environment and YAML overrides. A 300000 ms default gives no database-work
benefit after early binding; it delays genuine pre-listener failure detection.
Desktop retains its existing independent 60-second liveness budget and unlimited
readiness wait. No timeout default changes are required.

Initialization checks cancellation before and after each repository schema step
and before each required store admission. The task repository passes the same
context to its migration SQL, including data backfills and transactional table
rebuilds. Other legacy constructors can finish an already admitted statement
because they do not expose a context API; the next admission barrier prevents
later stores from opening. A statement already executing in a context-aware
driver is also subject to that driver's cancellation behavior, so cancellation
does not promise an immediate interrupt at every SQLite virtual-machine or
user-function instruction. Cleanup waits until the synchronous initializer
returns, avoiding a pool-close race with in-flight SQL.

## Persistence safety

Keep `VACUUM INTO`, version-based backup eligibility, and two-snapshot retention.
Context cancellation applies to the backup SQL. Repository initialization failure
must release previously constructed resources; a completed backup remains intact.
The successful version marker stays behind the final persistence/router gates.

Pending migrations cannot be inferred from `MigrateLogger`: owners also execute
DDL, table rebuilds, data repairs, and seed changes directly, including stores
constructed after `provideRepositories`. A future registry must account for all
required stores, late constructors, dialect-specific DDL, data backfills, and
seed migrations, with an immutable applied ledger and completeness tests.
Only then can backup eligibility safely depend on pending work.

Retries before version recording take another backup. After partial migration,
that snapshot is not necessarily a pre-upgrade image. Two successful retries can
prune the original snapshot. Filename timestamps have one-second resolution and
can also collide on rapid retries. This existing risk requires a separate recovery
design: database identity, durable attempt state, original-snapshot pinning,
crash-safe retention, and restore tests. Never reuse by filename or target version.

## Requirement mapping

| Criteria | Mechanism |
| --- | --- |
| .1, .2, .3, .8 | Early bootstrap binding, atomic router handoff, existing launcher waits |
| .4, .5 | Typed phase snapshot, structured timing, child-exit diagnostics |
| .6, .7 | Separate process/worker cancellation ownership, schema/store admission barriers, sequential initialization, backup gate |

## Decisions

This extends the existing bootstrap architecture rather than creating a new
persistence contract. See [database upgrade safety](../../../decisions/0008-db-upgrade-safety.md).

## Implementation plans

- [Large database startup](../../../plans/large-database-startup/plan.md)
