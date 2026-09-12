---
id: "02-evidence"
title: "Startup evidence"
status: done
wave: 2
depends_on: ["01-lifecycle"]
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

# Startup evidence

## Scope

Isolated populated SQLite backup and migration timing, deferred registry and
retention findings, public troubleshooting documentation.

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

Task 01.

## Parallelism

Sequential. No delegation.

## Risks

See plan and system design for cancellation and retry retention constraints.

## Results

`TestStartupMigrationCostsPopulated` creates a fresh schema, then rebuilds
only the message table into the legacy shape needed by the recurring startup
backfills. It inserts 24 task sessions and 6,000 messages (one third user
messages), leaves `updated_at` nullable and empty, and omits `prompt_seq` so a
replay exercises both migration paths. The test also runs the same
`VACUUM INTO` snapshot used by production and measures the two backfills
separately after replay. The fixture is isolated under `t.TempDir()` and can be
reproduced with:

```text
go test ./internal/task/repository/sqlite -run '^TestStartupMigrationCostsPopulated$' -v -count=1
```

One recorded run on Linux amd64, Go 1.26.0, 11 CPUs, reported:

| Measurement | Result |
| --- | ---: |
| Populated database file before replay | 929,792 bytes |
| `VACUUM INTO` backup | 2,252,800 bytes in 18.99 ms |
| Fresh schema initialization | 48.75 ms |
| Populated replay | 285.06 ms |
| `task_session_messages.updated_at` backfill | 19.25 ms |
| Prompt sequence update plus counter aggregation | 41.52 ms |

The values are observations from one local run, not startup budgets. The
fixture does not model a production database's row distribution, WAL state,
filesystem, CPU contention, provider recovery, or full backup directory. It
does distinguish the liveness and readiness contract: the bootstrap tests keep
`/health` at 200 while `/ready` and application routes remain 503 until the
initializer publishes the router, and the launcher health deadline is not
used as the readiness deadline. The evidence does not change `VACUUM INTO`,
backup retry behavior, or backup-registry and retention design.

Lifecycle regression coverage now includes delayed success past a short health
deadline, active SQL cancellation through `NewWithDBContext`, worker-only
restore quiescing with `restart_required`, explicit post-readiness signal
handling, and initializer joining after cancellation.

PR review follow-up also verifies the retry boundaries: pre-migration snapshots
are created in a private staging directory, validated, set to mode `0600`, and
installed only after validation; a failed `VACUUM INTO` destination is removed
when the path did not exist before the attempt. The authenticated system-job
notification path remains on the process lifetime while restore workers are
quiesced. These changes do not alter backup retention or the deferred backup
registry redesign.
