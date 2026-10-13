---
id: "01-complete-snapshot"
title: "Complete dev production SQLite snapshots"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-GO-DEV-LAUNCHER-001
acceptance_criteria:
  - AC-PLATFORM-GO-DEV-LAUNCHER-001.10
  - AC-PLATFORM-GO-DEV-LAUNCHER-001.11
  - AC-PLATFORM-GO-DEV-LAUNCHER-001.12
  - AC-PLATFORM-GO-DEV-LAUNCHER-001.13
  - AC-PLATFORM-GO-DEV-LAUNCHER-001.14
  - AC-PLATFORM-GO-DEV-LAUNCHER-001.15
  - AC-PLATFORM-GO-DEV-LAUNCHER-001.16
system_design:
  - ../../specs/platform/system-design/go-dev-launcher.md
---

# Task 01: Complete dev production SQLite snapshots

## Summary

Make the dev production guard create and verify a standalone SQLite snapshot
including committed WAL before it permits child launch. Independently author
real SQLite regressions, preserve the current missing-source and family-retention
contracts, and reconcile public instructions in the same delivery.

## In scope

- Thread `context.Context` from `runDev` through `backupProductionDBIfNeeded` to
  `backupProductionDb`; derive the design's bounded attempt context. Update every
  affected call site and test signature together.
- Use read-only escaped SQLite file URIs and the existing `db.BackupSQLite`.
  Keep source main/WAL unchanged, without bootstrap, migration or checkpoint.
- Use owned private staging, full `integrity_check`, closed readers, injected
  timestamp, exclusive final-path installation and failure cleanup. Reuse current
  directory, filename and retention logic; skip pruning on failed attempts.
- Add `backup_sqlite_test.go`; adapt affected `backup_test.go` arbitrary-byte
  source fixtures to real SQLite. Retention-only foreign-file sentinels can stay
  arbitrary bytes because they are never opened as source databases.
- Extend the real `runDev` backup guard regression using existing injected seams,
  with no real child launch. Restore all swapped seams with `t.Cleanup` and run
  environment/CWD tests sequentially.
- Update `docs/public/remote-cloud-environment.md` when the implementation lands.
  Its development section must say that an existing external database gets a
  verified standalone SQLite snapshot including committed WAL, retains five
  snapshots and aborts on failure. Replace the raw-main-file limitation and the
  obsolete manual-backup `VACUUM INTO` attribution; retain the link to SQLite
  operations, independent backup advice and deliberate `make dev-prod-db` intent.
  State that this snapshot does not bypass the existing exclusive runtime
  ownership guard; development against production must still have sole ownership.
- Separately reviewed and released amendment: snapshot-only `signal.NotifyContext` derived
  after dev-local skip, stopped after attempt cleanup and before supervisor signal
  registration. Implementation followed ROOT's later explicit release.

## Out of scope

Shared engine rewrites, shared reader behavior changes, schema/migrations,
source recovery, storage selection, other snapshot families, backup reuse,
compaction, restore, ownership redesign, new configuration, UI/i18n/screenshots,
frontend suites, broad QA/review/verification, delegates, sessions or tasks.
Global launcher signal ownership and automatic crash-stage scavenging are excluded.

## Acceptance

1. The two independent WAL regressions fail RED with the observed two-source-row
   versus one-snapshot-row mismatch, then pass GREEN with exact schema and sentinel
   records. Open-WAL, offline-WAL and rollback fixtures prove source data and stable
   main/WAL bytes remain unchanged. A destination-only reader proves no source
   sidecars are required; URI-sensitive paths select the intended source.
2. Invalid/read-failed sources, native-copy and verification failures, canceled or
   expired attempts and exclusive filename collisions fail without successful
   backup output, old-snapshot pruning or child launch. Owned partial artifacts
   are cleaned and existing destinations/sentinels survive. Missing input and
   dev-local targets still skip; successful files retain names, timestamps and
   permissions, and only the newest five matching-family snapshots remain.
3. The public how-to reflects the strengthened contract while preserving explicit
   production-development intent and ownership constraints. All task checks pass;
   synchronize actual results and `done` status in this order and the manifest
   before the first publication. Never mark planned commands as executed.

## Regression fixture instructions

Use `/tdd` and the backend test reference. Do not read/copy the discoverer's
private candidate test. For each WAL case, create a valid SQLite table and first
sentinel row, checkpoint that fixture once, disable fixture autocheckpoint, then
commit a second distinguishable row into WAL. This checkpoint is fixture setup,
never production adapter behavior. Independently assert source row count is two.

For open WAL, retain the fixture writer through backup and source-preservation
assertions, with no concurrent writes. For offline WAL, copy the fixture's stable
committed main/WAL image into a different owned `t.TempDir()` path after the commit
while its original writer is still open. No connection or writer may be bound to
that copied path at backup time. Do not close/checkpoint the origin before copying
or strip the image's WAL. SQLite may establish transient `-shm` state; compare
source main/WAL bytes and records, not shared-memory lock bytes.

Independently open only the final snapshot, require full integrity success and
exact schema/sentinel rows. Include rollback-journal control and a filename with
`%`, `?` and `#` on supported platforms. Use existing-target sentinel contents
for collision assertions. Seed six prior snapshots before failed-copy/verification
cases so accidental retention is observable, and distinguish foreign families.
Deterministic narrow local failure seams are permitted for hard-to-induce errors;
all successful snapshots must use the real native primitive. Use canceled/expired
contexts and barriers rather than sleeps. Close handles before deleting fixtures
and register cleanup immediately. Use no live home/config/database/process.

## Verification

Design phase runs only the lightweight spec, traceability and diff checks below.
Implementation begins only after the caller's full design review and later
explicit implementation instruction. Obtain the caller-owned global heavy lease
before Go/lint/install activity. Preserve each native session handle, join every
owned command to its actual exit and release the lease only after those joins and
confirmed absence of owned active commands. Never kill foreign processes, clean
shared caches or volumes, or change executable bits on tracked helpers.

Use Go 1.26 and Node 24, the existing module/shared caches, `GOTOOLCHAIN=local`,
CGO, race, trimpath, fts5, one package worker and the stated resource limits.
In a fresh worktree only, run the frozen install once before tests or hooks:

```bash
(export PATH=/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH; cd apps && pnpm install --frozen-lockfile)
```

Run from the repository root. First run RED before production edits, recording
the actual causal assertion; then GREEN and the whole affected launcher package:

```bash
(export PATH=/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH GOTOOLCHAIN=local CGO_ENABLED=1 GOMAXPROCS=2 GOMEMLIMIT=512MiB; cd apps/backend && timeout --signal=TERM --kill-after=10s 280s go test -race -trimpath -tags=fts5 -p=1 ./internal/launcher -run '^TestBackupProductionDbCommittedWAL$' -count=1)
(export PATH=/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH GOTOOLCHAIN=local CGO_ENABLED=1 GOMAXPROCS=2 GOMEMLIMIT=512MiB; cd apps/backend && timeout --signal=TERM --kill-after=10s 280s go test -race -trimpath -tags=fts5 -p=1 ./internal/launcher -count=1)
```

Run the complete affected package once after GREEN; repeat only for a new edit,
failure or unresolved concern. Run additional native-backup package checks only
if a required adapter change actually alters the shared helper, reconciling this
order first. No initial full backend lint, frontend build/tests, SQL guard or store
conformance suite is justified: schema and persisted storage remain unchanged.

Before changed-Go lint, stage the actual new helpers/tests and all affected Go
files so the canonical command includes their complete packages. This staging
step applies only to the later implementation/delivery turn:

```bash
(export PATH=/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.nvm/versions/node/v24.18.0/bin:$PATH GOTOOLCHAIN=local CGO_ENABLED=1 GOMAXPROCS=2 GOMEMLIMIT=512MiB; git diff --cached --name-only --diff-filter=ACMR -z -- 'apps/backend/*.go' | xargs -0 -r bash scripts/lint-go-changed)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
git diff --cached --check
git status --short -- docs/plans/dev-production-sqlite-snapshots
```

Use Node 24 for the documentation checks as well. Validate the changed work order
and its linked manifest/requirement/design using the repository's exported
`.github/scripts/pr-docs.cjs:validateCoverage` with actual workspace file contents
and actual changed-file inventory (including untracked work-order paths before
staging); require `ok: true` and `errors: []`. Do not use the hosted status publisher
or fabricate an implementation result during the design checkpoint.

Keep normal hooks active. If formatting changes staged files, re-stage and create
a new commit; no hook bypass or amend. Delivery remains the authorized normal
commit/push/ready-PR workflow after actual checks and results, with the caller's
serial publication/observer and expected-head merge gates intact. The design
checkpoint itself remains unstaged and uncommitted.

## Files likely touched

- `apps/backend/internal/launcher/backup.go`
- `apps/backend/internal/launcher/backup_test.go`
- `apps/backend/internal/launcher/backup_sqlite_test.go` (new)
- `apps/backend/internal/launcher/dev.go`
- `apps/backend/internal/launcher/dev_test.go`
- `docs/public/remote-cloud-environment.md`
- The owning requirement/design and this plan/order for accurate status/results.

A small launcher-local helper/test file is allowed if needed to keep Go limits;
name it in the actual result and include it in staged changed-package lint.
After signal-amendment release only: `dev_backup_signal_test.go` (Unix-only) and
the existing `testmain_test.go` private-helper dispatch may change for its isolated
real-signal regression. No new TestMain or production helper mode is introduced.

## Dependencies

None. Existing `db.BackupSQLite`, URI escaping and launcher test seams already
exist at the qualified baseline. No shared package changes are planned.

## Risks

Full integrity validation can dominate duration and disk demand; deadline expiry
must remain an error. WAL reader capability and hard-link support vary by host.
Source isolation and cleanup must not turn a collision into foreign-file deletion.
Backend-only fixture tests cannot prove performance on very large databases.

## Parallelism

`sequential`. Same primary session; no delegation.

## Inputs

- [Requirement](../../specs/platform/requirements/go-dev-launcher.md), .10-.15.
- [Design](../../specs/platform/system-design/go-dev-launcher.md#production-snapshot-adapter).
- [Manifest](plan.md), including qualified causal evidence and exclusions.
- `apps/backend/AGENTS.md` and `.agents/skills/tdd/references/backend-tests.md`.
- Existing launcher backup/guard tests, `internal/db/sqlite_backup.go`,
  `sqlite_backup_nocgo.go`, `sqlite_uri.go`, and persistence no-replace publication.

## Results

Completed 2026-10-13 under the separately granted local heavy lease, in the same
primary session without delegation. Actual changed runtime files are `backup.go`,
new `backup_sqlite.go`, and context wiring in `dev.go`. Tests are adapted
`backup_test.go`, new `backup_sqlite_test.go`, and new `dev_backup_test.go`.
The existing `dev_test.go` guard and process tests remain unchanged. The owning
requirement/design and public how-to are reconciled with the completed behavior.

Native command receipts, all actually joined:

| Command / evidence | Native handle | Actual result |
| --- | --- | --- |
| Frozen `pnpm install --frozen-lockfile`, Node 24 | 36179 | Exit 0, once, pnpm 9.15.9, 1.8s |
| RED `go test -v -race -trimpath -tags=fts5 -p=1 ./internal/launcher -run '^TestBackupProductionDbCommittedWAL$' -count=1` | 53647 | Exit 1, both open/offline causal two-source-row versus one-snapshot-row failures, 0.090s |
| RED same Go flags with `-run 'TestBackupProductionDbInvalidSource\|TestRunDevProductionBackupAdmission'` | 4626 | Exit 1, invalid source and canceled guard admitted backend; valid positive control passed, 0.208s |
| GREEN same Go flags with `-run 'TestBackupProductionDb\|TestRunDevProductionBackupAdmission\|TestRunDevAbortsOnBackupFailureBeforeLaunch'` | 22671 | Exit 0, 1.262s |
| First whole package, same Go flags and `-count=1` | 35320 | Exit 1, 11.742s; new positive-control fixture armed a real signal handler and interfered with the existing process test/race cleanup |
| Whole package after correcting only new fixture's injected signal seam | 42802 | Exit 0, 263 top-level tests, 17.927s |
| First canonical staged changed-package lint | 69609 | Actual exit 123 from xargs, one gocritic ifElseChain finding in the new cancellation test |
| Same Go flags with `-run 'TestIsProductionDb\|TestBackupProductionDb\|TestRunDevProductionBackupAdmission\|TestRunDevAbortsOnBackupFailureBeforeLaunch'` after test-only switch correction | 95181 | Exit 0, all 15 affected top-level tests, 1.294s |
| Canonical staged changed-package lint after correction | 27805 | Exit 0, zero issues |

Every Go run used the verification block's Go 1.26 environment and GNU
280-second timeout with 10-second kill grace. No assertion was weakened and no
existing lifecycle/production signal implementation was changed. The initial
failed whole package remains failure evidence, not a successful suite receipt.
The later test-only switch edit required focused validation and affected-package
lint; the successful whole package already covered the corrected signal fixture.

The exact lint pipeline was
`git diff --cached --name-only --diff-filter=ACMR -z -- 'apps/backend/*.go' | xargs -0 -r bash scripts/lint-go-changed`
with the documented Go/Node/resource environment. All new helpers/tests were
staged; the tracked helper's mode was preserved. No full-backend lint ran.

- `python3 scripts/list-docs.py validate`: passed, 380 decisions / 1,580 specs.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Node 24 `node scripts/validate-public-docs.mjs`: passed, 48 published pages.
- Actual staged-file `pr-docs.cjs:validateCoverage`: `covered`, `ok: true`,
  `errors: []`; resolves this work order, manifest and owning requirement/design.
- `git diff --check` and staged diff check: passed; final records checked again
  before the normal commit. Work-order inventory contains this one order.
- Public docs updated: `docs/public/remote-cloud-environment.md`, how-to guide.
  No UI/mobile/screenshots/localized-copy changes or browser E2E required.

Raw logs and structured documentation coverage are retained in the owned local
evidence directory `/tmp/kandev-child120-implementation-kALaKN/` (`install.log`,
`red.log`, `red-admission.log`, `green.log`, `launcher.log`, `launcher-final.log`,
`targeted-final.log`, `lint.log`, `lint-final.log`, `doc-coverage.json`). Those local
paths supplement the durable command/results above and are not repository inputs.
Normal hook/commit/push/PR receipts live in the external task delivery record;
publication and hosted review follow these completed local checks.

### Required CI remediation

Hosted `Backend Static Checks`, job `115264964136` in run `38388120019` at
`a7b2ff8f87330b1da719042ada3acc78417e6755`, failed its required-store SQL
portability step on this verifier's `PRAGMA integrity_check`. Build, dependency
graph and settings-catalog steps passed; golangci-lint had not run in that job.

Under the separately re-granted heavy lease, exact reproduction from
`apps/backend` with Go 1.26, `GOTOOLCHAIN=local`, `GOMAXPROCS=2`,
`GOMEMLIMIT=512MiB` and `timeout --signal=TERM --kill-after=10s 280s`:

- `go run -trimpath ./cmd/sqlguard ./internal`: native 77468, actual joined
  exit 1, one `sqlite-catalog` violation in `verifyProductionSnapshot`.
- Added one exact existing registration in
  `internal/db/sqlguard/exemptions.json` for this file/function/rule. The reason
  states that the launcher opens only read-only `sqlite3` and verifies integrity
  before publication. The analyzer, shared stores, schema and runtime are unchanged.
- The same exact command: native 34022, actual joined exit 0, SQL guard passed.
  Its normal scan validates precise matching and rejects unused exemptions.

This configuration-only correction needs no new permanent test or full-backend
suite. The canonical staged Go-file lint pipeline has no affected Go inputs for
this fixup, so it is a no-op rather than a new package-lint pass. Normal hooks
and spec/documentation/diff coverage remain required before the fixup push.
The single original hosted observer continues across this correction; no new
FULL review request was sent. Optional style/coverage suggestions and hard-kill
staging reclamation do not expand this required-CI correction.

### Review fixture corrections

The supervising task explicitly authorized two bounded test corrections after
reading the full Greptile and CodeRabbit findings. The offline case now verifies
records at the origin before cloning, captures the copied main/WAL bytes without
opening SQLite there, and leaves the copied source unopened until the actual
backup adapter. Both source and standalone destination are queried afterward.
The dev admission fixture pins both `HOME` and Windows `USERPROFILE` to the same
owned temporary directory, preventing a valid snapshot from reaching a real home.

The documented Go/race/trimpath/fts5/p1/resource/timeout command with
`-run 'TestBackupProductionDbCommittedWAL|TestRunDevProductionBackupAdmission'`
joined native 64093 with exit 0 in 1.204s. No runtime or existing process-test
assertion changed for these corrections. Staged affected-package lint and normal
hooks precede their batch publication; signal-admission findings are assessed
separately before any production extension.

### Snapshot signal amendment

Design maps AC-PLATFORM-GO-DEV-LAUNCHER-001.16 to the guard's temporary signal owner.
Write the isolated Unix helper regression first and observe causal RED before
production changes. An exact helper argument plus private environment marker is
required, dispatched in existing `TestMain` before normal tests. Signal only that
test-owned helper after its inherited-pipe ready barrier; join it before inspecting
owned stage/history/source records. Use bounded contexts/pipes and no sleeps.
The old normal CLI path must die from SIGINT before reporting/cleanup; the fixed
path must report non-zero admission failure with cleanup complete, no children and
source unchanged. A successful admission control proves the temporary owner was
stopped before the existing supervisor seam. Keep all normal signal lifecycle
implementation and second-signal semantics unchanged.

After explicit release, use the existing verification environment and GNU
280-second bound from `apps/backend` for these exact commands:

```bash
go test -v -race -trimpath -tags=fts5 -p=1 ./internal/launcher -run '^TestRunDevSnapshotInterruptCleanup$' -count=1
go test -v -race -trimpath -tags=fts5 -p=1 ./internal/launcher -run 'TestRunDevSnapshotInterruptCleanup|TestRunDevProductionBackupAdmission|TestBackupProductionDbCanceled|TestBackupProductionDbCommittedWAL' -count=1
go test -v -race -trimpath -tags=fts5 -p=1 ./internal/launcher -count=1
```

Record RED/GREEN and exact native joins. One full launcher run is justified by
the new runtime signal boundary; no full backend suite, schema audit or manual
FULL review request follows. Stage actual helpers/tests and runtime Go edits
before canonical affected-package lint; use normal hooks and reconcile this
order/manifest before the next batch push. SIGKILL/power loss cannot run defers;
historical-stage reclamation remains a separate excluded ownership contract.

ROOT completed full review of the four-file amendment (`d4ce7a`, exact review
receipt in `signal-design-review.json`) and separately released implementation.
Only `dev.go:backupProductionDBIfNeeded` gains temporary `signal.NotifyContext`
after the dev-local skip, defer-stop after attempt cleanup, and a context check
before any success message/return. Global launcher dispatch, other modes and the
existing supervisor signal implementation remain unchanged.

Actual runtime/test receipts with the same documented Go environment and bounds:

| Evidence | Native handle | Actual joined result |
| --- | --- | --- |
| Isolated real-SIGINT RED, exact first command above | 78563 | Exit 1, 0.247s; copy helper died by SIGINT before cooperative result/cleanup; handoff control passed |
| Signal plus existing WAL/cancel/admission GREEN, second command | 92070 | Exit 0, 1.419s; both signal subcases and all selected controls passed |
| Complete owning launcher, third command, after all runtime/test edits | 19662 | Exit 0, 264 top-level tests, 18.004s, race clean |
| Canonical actual-staged complete-launcher-package lint | 52909 | Exit 0, zero issues |
| Exact required SQL guard `go run -trimpath ./cmd/sqlguard ./internal` after signal edits | 89508 | Exit 0, with the documented Go/resource/280-second bound |

The copy-phase helper reports dev exit 1 with no success message, supervisor or
backend, and parent assertions prove no owned staging/history/source change.
The successful handoff helper actually dies by the default SIGINT action at the
injected supervisor registration boundary, proving the temporary notification
was removed. Every signal targets only the newly created helper PID; parent tests
and foreign processes are untouched. Both helpers are joined before assertions.
Old-history mtimes are fixed for deterministic handoff/retention evidence.
The cold-WAL first-open and Windows home corrections are included in this batch.

New test files are `dev_backup_signal_test.go` (Unix-only) and the small private
dispatch addition in the existing `testmain_test.go`; no product helper mode or
new TestMain exists. Full Windows OS-signal injection is unavailable through Go's
Unix process-signal transport; platform-neutral fixtures still pin both homes.
Public how-to documents ordinary-interrupt cleanup and the explicit lack of
SIGKILL/power-loss staging reclamation. Normal hooks and hosted checks follow;
no new manual FULL review request or broad backend suite is added.
