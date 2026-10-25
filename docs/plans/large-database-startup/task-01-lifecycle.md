---
id: "01-lifecycle"
title: "Startup lifecycle"
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
acceptance_criteria:
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.1
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.2
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.3
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.4
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.5
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.6
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.7
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.8
system_design:
  - ../../specs/platform/system-design/startup-lifecycle.md
---

# Startup lifecycle

## Scope

Early bootstrap ownership, phase reporting, cancellation, backup ordering, and launcher diagnostics.

## Exclusions

No backup mechanism replacement, registry conversion, retention redesign, or publication.

## Acceptance

- Meet the referenced lifecycle criteria with isolated regression evidence.
- Preserve health identity, timeout overrides, and required persistence gates.
- Record exact verification results and measurement limitations.

## Files likely touched

`apps/backend/internal/backendapp`, `internal/persistence`, `internal/launcher`,
`internal/db`, task repository measurement tests, and owning/public documentation.

## Verification

Use the corresponding commands in [plan](plan.md#verification), from their stated directories.

## Dependencies

None.

## Parallelism

Sequential. No delegation.

## Risks

See plan and system design for cancellation and retry retention constraints.

## Results

Implemented and tested the lifecycle changes. The bootstrap listener now binds
before database opening, backup, migrations, service initialization, and
session recovery. `/health` is live during initialization, `/ready` reports a
typed phase with elapsed measurements, cancellation closes listeners while
initialization drains, and the launcher reports the last phase on early exit.

The backend lifecycle and launcher race suites passed. Backup failure remains
before migrations, canceled persistence does not start backup work, and
restore quiescing cancels workers without ending the process lifetime before
the restore result is delivered. Repository startup checks cancellation at
schema and store admission boundaries; task migrations pass cancellation into
their data statements and table rebuilds. An in-flight database statement can
still finish according to the driver's cancellation semantics, and cleanup
waits for the synchronous initializer to return.
