---
created: 2026-10-13
status: done
requirements:
  - REQ-PLATFORM-GO-DEV-LAUNCHER-001
system_design:
  - ../../specs/platform/system-design/go-dev-launcher.md
legacy_specs: []
---

# Implementation Plan: Complete dev production SQLite snapshots

## Overview

Replace the dev launcher's raw-main-file safeguard with a verified standalone
SQLite snapshot that includes committed WAL data before development children
start. One sequential work order delivered the launcher adapter, its real
SQLite regressions, pre-launch guard and public instructions. Implementation
followed the separate explicit release after the completed design review.

## Confirmed defect and evidence

Qualified baseline: `9e541b07d6ee00a162a88393ff4bb8d3bb237262`, matching the
remote main head at design time. `backupProductionDb` streams only `main.db` with
`io.Copy`. `runDev` correctly aborts on a backup error, but the incomplete file
copy returns success, so its guard cannot detect the omission.

The supplied isolated evidence at
`/tmp/kandev-root-dev-backup-discovery-20261013-m7_r2kwu/qualified-proof.json`
and `repro.log` records two causal failures: an open stable WAL database and an
owned stable offline WAL image each have two independently queried committed
rows, while the successful standalone snapshot has one. The first row and schema
were checkpointed before committing the second row into a 4,152-byte WAL. The
offline source has no writer or connection bound to its path during backup.
Rollback-journal and missing-file controls pass. The native launcher package
run exited 1 with two causal failures and two passing controls, in 0.089 seconds.
This is inherited qualified evidence, not a test executed during this design turn.
Do not read or copy the discoverer's private candidate test.

The owning requirement promises a pre-dev snapshot guard and five-deep family
retention; public remote-environment instructions explicitly document the raw-copy
limitation. The new criteria strengthen that documented behavior. They do not
retroactively claim a WAL-aware guarantee. The completed original
[launcher plan](../go-dev-launcher/plan.md), especially tasks 02, 05 and 08,
records the previous migration and remains historical; its old broad validation
and live `make dev-prod-db` steps are not this repair's checks.

## Scope

### In scope

- AC-PLATFORM-GO-DEV-LAUNCHER-001.10-.15: complete native snapshots, read-only
  source admission, bounded cancellation, verified publication, failure cleanup,
  existing naming/permissions/retention and missing-source behavior.
- Thread the dev context through the guard without moving its pre-launch position.
- Replace affected arbitrary-byte backup fixtures with real SQLite and add
  independently authored causal RED/GREEN regressions.
- Amend the existing public development-state instructions with the implementation.
- Review amendment: AC-PLATFORM-GO-DEV-LAUNCHER-001.16, a temporary snapshot-only
  signal owner before the existing supervisor, with isolated helper-process RED
  and owned cleanup evidence. Implementation followed the separate release.

### Out of scope

- Schema, migrations, storage selection, recovery, restore, compaction,
  maintenance admission, retention registries and other snapshot consumers.
- Broad ownership, security, lifecycle or atomic-publication frameworks.
- Frontend/UI/mobile interactions, screenshots, translations, browser E2E,
  new dependencies, runtime flags and operator-configurable timeouts.
- Native delegates, persistent tasks, additional sessions and model switches.

## Technical approach

Follow [the owning design](../../specs/platform/system-design/go-dev-launcher.md#production-snapshot-adapter).
`backup.go` owns a narrow adapter using `db.BackupSQLite`, an escaped read-only
SQLite URI, a five-minute context bound, private same-directory staging, full
integrity verification, timestamping and exclusive `os.Link` publication.
`dev.go` passes its context through the existing production guard. Pruning runs
only after successful publication. The shared native primitive and shared URI
escape helper remain unchanged; `OpenSQLiteReader` currently does not escape
delimiters, so source-opening adaptation is local to the launcher.

| Source/capability | Intended behavior | Evidence or fallback |
| --- | --- | --- |
| Open WAL SQLite file with stable committed data | Copy schema and both sentinel rows through native SQLite visibility | Open-WAL regression; independent destination-only reader |
| Owned offline image with committed WAL | Include WAL despite no bound source writer | Offline-WAL regression; unchanged source main/WAL bytes |
| Rollback-journal SQLite file | Complete standalone snapshot | Rollback control and exact sentinel data |
| Missing file / `.kandev-dev` target | Preserve skip behavior | No source/artifact creation; existing classification checks |
| Invalid/unreadable source, unavailable WAL reader capability | Abort before children | Error, cleanup, no success message or old pruning |
| Existing final filename / filesystem without hard links | Abort without overwriting | Exact collided sentinel preservation; no fallback copy |
| Canceled/deadline-expired caller | Abort and release handles/owned staging | Controlled context tests; no sleep or real process |
| Native SQLite unavailable without CGO | Propagate helper error | Existing non-CGO stub; no raw-copy fallback |

There are no attached-database, in-memory, PostgreSQL or application-bootstrap
consumers in this launcher adapter. Existing shared backup tests already cover
its native engine; additional engine checks are conditional on an actual shared
helper change, which is outside the planned scope.

## Tests

Permanent evidence, all under `apps/backend/internal/launcher/`:

| Acceptance | Regression or control |
| --- | --- |
| .10, .11 | `backup_sqlite_test.go`: `TestBackupProductionDbCommittedWAL` with `open_WAL` and `offline_WAL_image`; assert schema, exact sentinel rows, main/WAL hashes and source rows |
| .10, .11 | `TestBackupProductionDbRollbackJournal`, `TestBackupProductionDbURIPath`; independent snapshot reader |
| .12, .13 | `TestBackupProductionDbInvalidSource`, `TestBackupProductionDbFailureCleanup`, `TestBackupProductionDbCanceled`; copy/verification failure, no pruning of existing six-file history |
| .12 | Existing `dev_test.go`: `TestRunDevAbortsOnBackupFailureBeforeLaunch`; new `dev_backup_test.go`: `TestRunDevProductionBackupAdmission` adds invalid/canceled cases and valid positive control through injected child seams |
| .13 | `backup_test.go`: adapt `TestBackupProductionDbCreatesStampedSnapshot`; add `TestBackupProductionDbPreservesExistingDestination` |
| .14 | Existing `TestBackupProductionDbSkipsMissingSource`, `TestBackupProductionDbPropagatesSourceErrors`, `TestIsProductionDb`; add no-artifact assertions |
| .15 | Adapt existing `TestBackupProductionDbPrunesToFiveNewest` and `TestBackupProductionDbPruneLeavesOtherFamilies` to valid SQLite sources |
| .16 | Unix `dev_backup_signal_test.go`: `TestRunDevSnapshotInterruptCleanup`, private helper/pipe barrier/actual SIGINT; success handoff control and full launcher integration |

Use actual SQLite copying for completeness tests. Small explicit local failure
seams are allowed only where deterministic verification/publication errors cannot
be induced portably; they do not replace native success-path coverage. Fixtures,
home/config state and source/destination files belong to `t.TempDir()` only.

## End-to-end evidence and surface assessment

The real `runDev` entrypoint with existing injected launch seams proves the
backup failure stops startup before backend, Vite or browser work. No real
process launch or live database is needed. The change has no rendered UI or
phone interaction, so browser E2E and mobile previews are inapplicable. Backend
CLI messages stay English; no new localized UI copy is introduced.

Public docs impact: the existing remote/cloud how-to guide's development-state
paragraphs must describe native WAL-complete snapshots and failure aborts, keep
intentional `make dev-prod-db` use and independent backup advice, and explain
that snapshots do not bypass the existing exclusive runtime ownership guard.
Remove its raw-copy/stop-writers claim
as a condition for snapshot completeness and the obsolete claim that ordinary
Backups uses `VACUUM INTO`. Do not rewrite unrelated operational policies.

## Work orders

- [x] [Task 01: Complete dev production snapshots](task-01-complete-snapshot.md)

Dependencies: none. Completed sequentially in the same primary conversation
after the explicit implementation release. Others' edits, shared caches,
dependencies, foreign refs, processes and protected resources were preserved.

## Verification results

Design checks passed on 2026-10-13:

- `python3 scripts/list-docs.py validate`: 380 decisions and 1,580 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- Workspace documentation preflight using `pr-docs.cjs:validateCoverage` and the
  actual four changed paths: `ok: true`, `errors: []`, documentation-only exemption.
  This exemption does not itself validate work-order traceability.
- Separate traceability check using the repository frontmatter parser: one
  pending work order, one draft manifest, an existing owning requirement/design,
  six valid linked acceptance criteria and no missing dependencies.
  The Node check completed successfully; the mixed shell command's exit 1 came
  from its trailing no-match `rg`, not from the traceability check.
- Catalog discovery finds the existing owning pair; diff whitespace check passed.

External design checkpoint: the supervising task completed full review of all
four artifacts on 2026-10-13 and accepted the bounded design with one correction.
The work order now omits the unchanged public-doc validator's unit suite; the
actual public-doc prose validator remains required for implementation. Previously
passed checks were not replayed solely for that deletion. Design handoff is
complete. Implementation remained unreleased at that checkpoint and began only
after the later explicit release in this same session.

Implementation checks completed on 2026-10-13 before publication; full commands,
native handles and truthful failure receipts are in [Task 01 results](task-01-complete-snapshot.md#results).
Independently authored open/offline WAL RED cases both observed source two rows
and snapshot one. The native adapter passed GREEN, then the complete launcher
package passed 263 top-level tests with race detection in 17.927 seconds.
After the lint-required test-only switch rewrite, all 15 affected tests passed
in 1.294 seconds and staged complete-package lint returned zero issues.

Catalog/spec lint, the actual Node 24 public-doc validator (48 pages),
actual runtime-change documentation coverage (`covered`, `ok: true`, `errors: []`)
and diff checks passed. Public remote-environment instructions were updated.
No shared engine, schema, restore/recovery, ownership, frontend or UI changes
were made; no broad checks or extra semantic reviews were added.

Required hosted CI later identified the verifier's missing precise SQLite-only
SQL-guard registration. The exact guard command reproduced that one finding and
passed after adding one file/function/rule entry in the existing exemption JSON.
No analyzer or runtime behavior changed; [Task 01](task-01-complete-snapshot.md#required-ci-remediation)
records the exact RED/GREEN commands and joins. This is a required CI repair,
not an initial speculative schema audit or a broadened persistence change.

Signal-admission amendment checkpoint: drafted after ROOT read the full
CodeRabbit finding that ordinary Ctrl-C arrives before supervisor registration.
The [same owning design](../../specs/platform/system-design/go-dev-launcher.md#temporary-snapshot-signal-owner)
limits notification to the production snapshot guard, preserving every other
launch mode and existing supervised second-signal ownership. At that design
checkpoint no production signal edit/helper test existed. ROOT then read all four
actual amendment diffs and separately released this same work order's extension.
Pure test-precision corrections passed targeted tests/lint; no full package was
rerun merely for them.

Signal extension completed before batch publication: isolated real-SIGINT RED
78563 joined 1, focused GREEN 92070 joined 0, whole launcher 19662 joined 0
(264 top-level tests, 18.004s), and canonical staged-package lint 52909 joined 0
(zero issues). The temporary owner stops after snapshot cleanup and before the
existing supervisor registration. The actual default-signal handoff probe passed.
Source/history/owned-staging and no-child assertions passed, with no parent or
foreign process signal. [Task 01](task-01-complete-snapshot.md#snapshot-signal-amendment)
records exact commands/joins and platform limits. The public how-to now states
ordinary-interrupt cleanup and SIGKILL/power-loss limitations without scavenging.

## Risks

- Full verification and free-page-preserving copies require disk space and can
  exceed the five-minute bound for unusually large databases; startup then fails
  closed. No performance claim follows from tiny regression fixtures.
- SQLite WAL reader permissions and transient shared-memory bookkeeping differ
  from a promise of zero filesystem activity. Never mask failures with immutable
  mode, checkpoints, sidecar copies or a writer opener.
- Hard-link publication can be unsupported on the backup filesystem. Preserve
  the no-replace guarantee and report the error before child launch.
- A verified backup is not runtime ownership or a restore policy. Keep explicit
  development/production intent and independent backup advice visible.
- Fresh-worktree hooks need one frozen pnpm install; heavy local checks require
  the caller-owned serial lease. Do not disturb shared caches to obtain it.
