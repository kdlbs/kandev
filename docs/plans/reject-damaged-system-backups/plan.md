---
created: 2026-10-19
status: done
requirements:
  - REQ-SYSTEM-PAGE-SYSTEM-PAGE-001
system_design:
  - ../../specs/system-page/system-design/system-page-01.md
legacy_specs: []
---

# Implementation Plan: Reject damaged System backups

## Overview

Reject invalid SQLite snapshots before restore disrupts the working database.
One sequential work order adds service-level regressions, inserts staged-file
verification, adapts affected fixtures, and updates public restore guidance.
ROOT completed review of the design package and released implementation in
the same primary session. No delegation or model switch is authorized.

## Evidence and root cause

ROOT reported qualification on clean actual main
`92f9f5025d67852d127310eab8fb0fde2a2024d3`: a production-created snapshot truncated
to 128 bytes was rejected by `verifySnapshot` but accepted by `runRestore`,
which disabled persistence, quiesced, closed the pool, and installed the damaged
file over distinct newer live data. Valid restore and wrong-confirmation controls
passed. This is supplied evidence, not a reproduction performed in this turn;
private qualification sources are forbidden and were not read.

Public source independently shows `runRestore` copies/syncs/closes staging and
immediately calls disruptive hooks without verifying SQLite integrity.
`retention_preparation.go` already implements the required read-only verifier.
Creation-time checking cannot cover subsequent snapshot damage.

## Scope

### In scope

- Amend the existing requirement with `AC-SYSTEM-PAGE-SYSTEM-PAGE-001.9` and its
  owning design; no standalone repair specification.
- Verify the actual staged restore bytes before disruptive effects, propagate
  the verification failure through the restore job, and remove owned staging.
- Prove preservation of distinct current data, the usable pool, main file and
  sidecars, and callback non-invocation for invalid snapshots.
- Keep valid restore, configured paths, confirmation, checkpoint-busy, staged
  write failure, PostgreSQL rejection, and quarantine rollback controls meaningful.
- Update existing public restore-order and failure guidance during implementation.

### Out of scope

Backup creation/publication/retention redesign, schema compatibility, migrations,
cross-process concurrency, database selection, runtime/startup policy, UI markup,
translations, new API fields, new persistent tasks, and protected resources.

## Technical approach

In `apps/backend/internal/system/backups/store.go`, call existing
`verifySnapshot(ctx, stagedPath)` after successful `writeStagedRestore` and before
`PersistenceUnavailable`. Discard its digest, wrap/propagate its error with useful
restore context, and remove the owned staged file on failure. Reuse existing
read-only URI handling; do not create another verifier or scan the live DB.
Existing post-validation shutdown and replacement behavior stays intact.
No SQL schema, required-store descriptor, repository method, or migration changes.
The design/work order preserve enough rationale for this local correction;
no separate ADR is needed under `/record`.

| Database shape | Intended behavior | Evidence |
| --- | --- | --- |
| SQLite, valid staged file | Existing successful replacement and restart | Service and custom-path controls |
| SQLite, invalid staged file | Failed job before any disruptive effect | New corruption matrix |
| PostgreSQL pool | Existing rejection before staging/shutdown | Existing public test |
| Nil pool test seam | Valid SQLite fixture still required at restore boundary | Adapt custom-path test |

## Tests

`AC-SYSTEM-PAGE-SYSTEM-PAGE-001.9` maps to new
`apps/backend/internal/system/backups/restore_integrity_test.go` tests
`TestRestoreRejectsInvalidSnapshotBeforeDisruption` and
`TestRestoreValidSnapshotReplacesNewerData`. The first drives the public `Restore`
job through terminal failure with real SQLite and separate cases for truncated,
structurally damaged, and non-SQLite files. The second proves actual saved content
replaces distinct newer live content with required restart. Exact assertions,
existing controls, affected A/M paths, and commands are in Task 01.

## E2E tests and mobile parity

The real service restore job plus SQLite file/pool checks exercise the complete
changed backend behavior. No browser test is added: HTTP/UI markup, copy, touch
behavior, navigation, layout, and job projection do not change. Desktop and phone
use the same failed-job result, while valid restore still uses the existing
restart dialog. `/mobile-parity` assessment found no rendered change needing a
new UI spec or mobile E2E fixture. No UI previews apply.

## Public documentation

During implementation update `docs/public/operations.md` (how-to, Restore a
System snapshot) and `docs/public/configuration.md` (reference, `database.path`).
Both must describe staging and integrity validation before shutdown and explain
that invalid snapshots leave current data usable without a restore restart.
README and screenshot references need no changes; no navigation or screenshot
changes. Both public documents now describe the verified rejection behavior.

## Work orders

- [x] [Task 01: Validate staged SQLite restore](task-01-validate-staged-restore.md)

## Verification results

Design checks passed on 2026-10-19:

- `python3 scripts/list-docs.py validate`: exit 0, 388 decisions and 1656 specs.
- `python3 scripts/lint-spec-files.py --all`: exit 0.
- `git diff --check` and plan-directory status: exit 0; two new plan files present.
- Node 24.21.0 local `validateCoverage` preflight: actual four documentation
  A/M paths are exempt; projected future `store.go` modification with this same
  package is covered with no reference errors. This projection is not a code
  change or an implementation result. External exact script:
  `/tmp/child149-reject-damaged-snapshots/check-doc-coverage.cjs`, invoked with
  `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`.

Assumption check: desired rejection, preserved current work, scope exclusions,
and later implementation barrier are settled by the user. No material product
question remains. All four artifact references resolve to the existing owner;
no adjacent companion plan needs a status change for this new bounded repair.
Implementation checks completed after explicit release:

- RED: `TestRestoreRejectsInvalidSnapshotBeforeDisruption` failed all six cases
  with unexpected successful restore, after each fixture independently failed
  verification. The first wrong-working-directory invocation was a command
  setup error and is excluded from RED evidence.
- Final GREEN: `go test -race -trimpath -tags fts5 -p 1
  ./internal/system/backups ./internal/persistence/storeconformance -count=1
  -timeout=240s`: exit 0; backup package 1.392s and conformance 55.355s.
  The combined invocation covers both work-order scopes serially with `-p 1`.
- `go run -trimpath -tags fts5 -p 1 ./cmd/sqlguard ./internal`: exit 0.
- Canonical initial changed-Go lint over the four listed Go A/M paths: exit 0,
  zero issues, default 3GiB with sufficient measured available host memory.
- Actual ten-path documentation coverage preflight: covered, no errors.
  Catalog validation, specification lint, and diff checks pass.

Go commands used Go 1.26.0 local, GOMAXPROCS=2, GOMEMLIMIT=512MiB,
GNU timeout 280 seconds/kill-after 10 seconds (tests also inner 240 seconds).
Canonical lint used its 5-minute budget under GNU 360 seconds/kill-after 10.
Native handles were joined to actual exit. No full backend lint or web/E2E
suite was run; no web behavior changed. Hosted CI/review and delivery evidence
are kept in the external operational plan rather than embedded head claims. External operational
recovery/checkpoint record: `/tmp/child149-reject-damaged-snapshots/recovery-plan.md`.
No production or permanent tests changed before the reviewed design checkpoint.

## Risks

- Read-only integrity verification and the existing verifier's digest add a
  bounded-by-file-size pre-shutdown scan; this intentionally reuses the existing
  verifier without changing retention contracts.
- Invalid byte fixtures in old success and checkpoint tests must become valid
  SQLite snapshots or they would silently test early rejection instead.
- SQLite SHM can change when SQL reads occur. Capture/compare live bytes around
  the rejected job before further pool reads; use an otherwise idle fixture.
- Integrity checks do not establish application schema compatibility or protect
  against unrelated external file mutations. Those remain outside this fix.

### Zero-byte admission correction

Public-Restore regressions independently confirmed that a fully truncated file
still passed SQLite integrity checking and was restored by the initial fix.
The restore-specific staging copy now rejects zero copied bytes before any
disruption, preserving the shared verifier contract. The corruption matrix now
includes zero-byte and 128-byte truncation under modern and legacy hooks.
Earlier local evidence above is historical; the corrective backup GREEN and
scoped lint results are recorded in Task 01. No new schema or repository logic
requires repeating earlier sqlguard/conformance checks.

### Required CI fixture compatibility

Backend shard 3/4 exposed an additional stale success fixture in
`internal/backendapp/startup_database_test.go`:
`TestBootstrapRestoreKeepsProcessAliveUntilResultAndShutdown` uses non-SQLite
backup bytes and fails before quiescence under the new integrity contract.
Task 01 now owns its valid-SQLite fixture correction, exact targeted regression,
and changed-Go lint. Preserve all lifecycle assertions; no startup production
behavior or contract changes. Exact current-head CI evidence and lease gating
are checkpointed externally; the earlier local passing package evidence does
not imply this newly identified integration fixture passed.

Fixture compatibility local validation completed after ROOT review/release:
targeted startup test exit0 (1.156s); scoped changed-Go lint exit0/zero issues;
catalog/spec/diff and actual11-path coverage passed. Hosted original failure is
the causal RED and was not replayed. Separate ledger diagnosis and current-head
hosted gates remain in the external operational record; local task completion
does not claim that the PR is ready or that the ledger cause is established.
