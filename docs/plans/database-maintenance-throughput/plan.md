---
created: 2026-10-12
status: done
requirements:
  - REQ-PLATFORM-SQLITE-SNAPSHOT-001
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
system_design:
  - ../../specs/platform/system-design/sqlite-snapshot-copy.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention-throughput.md
legacy_specs: []
---

# Database maintenance throughput

## Outcome and evidence

Make backup-only operations copy SQLite pages without rebuilding the database, and
run productive retention continuously without artificial sleeps. The user explicitly
rejected the proposed 1-second continuation/10-millisecond batch delays. Idle polling
and contention/error waits remain necessary to avoid spinning when no work can run.

The existing startup backup used VACUUM INTO; the measured 9.41 GB artifact took
818.4 seconds including verification. A separate read-only 6.87 GB snapshot copied
with the existing native API in 32.677 seconds and passed full integrity_check in
101.844 seconds total. These are different inputs, not a controlled speedup ratio.
Temporary output was removed; the live database was untouched by that experiment.

The worker spends about four seconds of each five-second pass in 100 ms sleeps,
then sleeps another minute. Historical reconciliation makes raw payloads eligible;
the retention worker must still remove them. Ordinary arrivals can also create a
backlog if production outpaces cleanup. No fixed total database-size guarantee follows.

## Scope and implementation

Task 01 adds the native copy primitive and migrates backup-only consumers, preserving
verified publication and pre-migration safety. Explicit maintenance compaction retains
VACUUM INTO. Task 02 removes initial and productive sleeps from the existing worker,
retaining small transactions, bounded pass accounting, cancellation and phase fairness.
No new schema, runtime flag, setting, worker, UI or pruning authority is introduced.
Public operations/configuration wording now describes the new backup and scheduling behavior.

| Boundary | Intended behavior | Verification |
| --- | --- | --- |
| SQLite file/WAL/in-memory | Native consistent copy; bounded cancellation | Small db/persistence fixtures |
| Unsupported driver | Fail closed; PostgreSQL backup unchanged | Driver rejection |
| Upgrade/reset/manual backup/repair/adoption | Existing verification and atomic publication | Owning package tests |
| Explicit compact | Physical page reclamation | Deleted-page compact regression |
| SQLite/PostgreSQL retention | Same eligibility and recovery; continuous productive work | Fake clock and small historical sweep |
| Desktop/phone | History and recovery survive cleanup | Existing two retention E2E specs |

## Tests and work orders

Exact test names, criterion mappings, files and commands are in the sequential orders:

- [x] [01: Copy consistent SQLite backups](task-01-snapshot-copy.md)
- [x] [02: Drain productive retention continuously](task-02-continuous-retention.md)

CI uses small fixtures; no million-row test is added. Large snapshot comparisons are
opt-in operational evidence outside CI. Do not restart or replace the live database
as part of implementation tests. Follow the existing local repair package for any
separately coordinated physical compaction.

## Companion records

[Large database startup](../large-database-startup/plan.md) remains complete.
[Legacy payload reclamation](../legacy-delivery-payload-reclamation/plan.md) retains
its pending physical local-repair work; merged eligibility changes are already active.
This package changes throughput only and does not repeat their full rollout audit.

## Verification results

Design validation passed: list-docs validate, lint-spec-files --all, git diff --check,
and PR-documentation coverage preflight against the planned source paths.
Implementation complete. Affected package race tests, small historical sweep/protected-gap
regressions, SQL guard, persistence store conformance, targeted lint (0 issues), and
public-doc/harness/spec validation passed. Initial local verification omitted unchanged
browser and PostgreSQL layers. CI remediation subsequently reran the affected recovery
browser scenarios and worker fixtures; see work-order results.
The isolated backup experiment above is feasibility evidence only.

## Risks

Full integrity checking remains significant. Page-copy backups may include free pages.
Continuous work increases resource use while a backlog exists; bounded leases, gate
contention and cancellation must remain effective. Protected discovery wrap must not
be mistaken for productive continuation, or the worker could spin forever.

## Concurrent WAL commits follow-up

The original active-writer control holds an uncommitted write; it does not prove
progress across repeated independent commits. An isolated SQLite Online Backup
API observation reproduced unchanged remaining pages after eight committed writes.
A deferred reader snapshot completed the same four bounded steps while three
independent writes committed, retaining the initial view. The actual Go
`BackupSQLite` entrypoint then confirmed the distinction: the baseline was
cancelled after eight repeated commits; the candidate completed with snapshot
epoch 0 and live epoch 3. The candidate and cancellation/pool-release controls
passed under `-race` (1.269s). No live backup or operating retry was performed,
and the original failing job's writer remains unknown.

Task 01 now has a narrow follow-up to pin an autocommit WAL reader snapshot and
release only its own transaction on every exit. Original publication, five-second
stall handling, non-WAL/caller-transaction behavior and Task 02 remain unchanged.

Existing locked-copy cancellation, active-WAL-writer and stalled-copy controls
passed. The memory/failure-cleanup control also passed under `-race` after its
disposable fixture removed only a filename character Windows forbids. The
pre-migration POSIX-mode privacy assertion fails identically on this Windows
host with pristine and candidate code; it is not reported as passed or weakened.
