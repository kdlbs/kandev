---
created: 2026-11-07
status: implemented
requirements:
  - REQ-PLATFORM-SQLITE-SNAPSHOT-001
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
system_design:
  - ../../specs/platform/system-design/sqlite-snapshot-copy.md
legacy_specs: []
---

# Implementation Plan: Upgrade backup version compatibility

## Overview

Restore upgrade backups for existing databases whose stored version contains
filename separators. Then prevent SDK tags from becoming local runtime
version defaults. Execute both work orders sequentially before publication.

Platform owns this repair because it owns upgrade backup publication and
startup safety. The existing snapshot and startup requirements apply.
No new requirement or architecture decision is necessary.

## Confirmed evidence

The reported startup failed on 2026-11-07 at 12:05:41 UTC.
The stored version was `pluginsdk/v0.1.0-16-gbe9d2485554`.
The backup copied 14,697,992,192 bytes before publication failed.
`snapshotPath` interpolates that version without filename normalization.
The resulting target contains a missing `kandev-pluginsdk` directory.
`createPreMigrationBackup` creates its staged file from `filepath.Base`,
so copying succeeds before the final hard link fails.

Read-only inspection confirmed the stored version in the existing database.
`git describe --tags --always be9d2485554` returned the same SDK description.
Both runtime Makefiles currently describe all tags.

## Scope

### In scope

- Normalize historical stored versions only when formatting backup filenames.
- Preserve verified, private, atomic backup publication and existing retention.
- Exercise the persistence provider with a small existing database fixture.
- Filter default runtime build versions to Kandev release tags.
- Retain explicit overrides and existing version fallback behavior.

### Out of scope

- Changes to user databases, metadata, or existing backup files.
- Skipping required backups or weakening integrity checks.
- Changes to backup reuse, rotation policy, SQLite copying, or compaction.
- Release channel changes, SDK version changes, or new configuration.
- UI changes and browser automation.

## Technical approach

Task 01 changes `snapshotPath` in
`apps/backend/internal/persistence/snapshot.go`. Use the filename component
rules from the paired design. Keep `provider.go` publication gates intact.
Add regressions in a focused `snapshot_version_test.go` file.

Task 02 adds black-box version assertions to `scripts/check-make-shells`.
Use disposable Git repositories with both runtime and SDK tags.
Filter the root `RUNTIME_VERSION` and backend `VERSION` defaults with
`git describe --tags --match "v[0-9]*" --exclude "*/*" --always --dirty`.
Preserve each Makefile's redirect and fallback logic.

| Boundary | Intended behavior | Evidence |
| --- | --- | --- |
| Existing SQLite database | Namespaced stored versions allow a verified upgrade backup | Persistence provider fixture |
| Portable backup filename | Version metadata cannot create nested paths or reserved filename characters | Filename table cases |
| Existing backup target | Atomic publication preserves an existing target | Existing persistence publication controls |
| POSIX and Windows build recipes | Runtime defaults exclude SDK tags | Real Git fixture and existing shell simulations |
| Explicit version override | Caller value wins over Git discovery | Both Makefile probes |
| PostgreSQL | Existing operator-managed backup behavior | No PostgreSQL changes |

## Tests

- `TestSnapshotPathVersionCompatibility` covers plain versions, empty versions,
  SDK namespaces, backslashes, traversal-shaped values, and reserved characters.
- `TestProvide_UpgradeFromNamespacedVersion` seeds a small database with the
  reported version, then exercises the actual persistence provider.
  It checks readable data, private permissions, direct publication, unchanged
  source metadata, and staging cleanup.
- Existing persistence tests cover cancellation, publication collisions,
  same-version eligibility, and backup failures.
- `scripts/check-make-shells` exercises actual version discovery for both
  Makefiles with a newer SDK tag and an older runtime tag.
  It also covers SDK-only history, dirty worktrees, explicit overrides,
  and the non-Git fallback.

The criteria are AC-PLATFORM-SQLITE-SNAPSHOT-001.3,
AC-PLATFORM-SQLITE-SNAPSHOT-001.4 and
AC-PLATFORM-STARTUP-LIFECYCLE-001.7.
The provider fixture is the end-to-end evidence for this backend repair.
It uses temporary data instead of the user's database.

## Work orders

- [x] [Task 01: Publish backups for historical version strings](task-01-portable-backup-filenames.md)
- [x] [Task 02: Exclude SDK tags from runtime build versions](task-02-runtime-version-tags.md)

## Delivery

The design-package handoff is complete. The user authorized implementation
through a GPT-6 Luna worker at max reasoning, followed by PR creation.
The primary session coordinates the sequential work orders and publication.
The primary session opened [PR 5154](https://github.com/kdlbs/kandev/pull/5154)
and completed the requested 15-minute hold before PR fixup. Do not merge the PR.

Public documentation does not need changes. This repair restores existing
upgrade behavior without changing commands, configuration, or recovery steps.
The historical database-maintenance-throughput package remains complete.
This follow-up does not change its copy primitive or recorded verification.

## Verification results

Design validation passed on 2026-11-07:

- `python3 scripts/list-docs.py validate`: 436 decisions and 1,931 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check -- docs/specs docs/plans/upgrade-backup-version-paths`: passed.
- PR documentation preflight used the repository's exported `validateCoverage`.
  The prospective production paths and both changed work orders returned
  `covered`, with no errors. Both requirement documents and the paired design
  resolved from the working tree.

Implementation verification passed:

- The provider regression reproduced the reported publication failure before
  the filename fix. The focused regressions and full persistence suite passed
  with `go test -trimpath -tags fts5` after the fix.
- Changed-file Go lint passed with zero issues. Go commands used
  `GOCACHE=/tmp/kandev-go-cache` because the default cache is read-only.
- `bash scripts/check-make-shells` reproduced SDK tag selection before the
  Makefile changes and passed afterward. Both version defaults, fallback
  cases, overrides, dirty suffixes and existing shell simulations passed.
  The native mkdir execution probe skipped because `cmd.exe` is unavailable.
- Documentation catalog validation, specification lint and `git diff --check`
  passed after each work order.

The work orders contain exact commands and RED/GREEN evidence.
Hosted check results and review dispositions belong to the PR delivery record.
No implementation or verification step requires changes to the user's database.

## Risks

- Restricting build tags alone leaves existing databases affected.
- Filename normalization can collide. Preserve atomic no-replace publication.
- Windows reserves characters that Linux accepts. Cover both separator styles.
- Backup integrity checks still take time on large databases.
- Git fixtures must not create tags or commits in the working repository.

## PR remediation

After the requested 15-minute hold, review identified three concrete edge cases:
portable filenames can exceed SQLite's usable component limit, namespaced tags
can begin with a version-like prefix, and temporary directories can inherit
a parent Git repository. Each case failed before its correction and passed
afterward.
Luna owns the code and targeted checks in the writable delivery checkout.
The primary session owns integration, review dispositions and publication.

Unique-name retry for same-second collisions remains an optional extension.
Existing atomic publication deliberately fails without replacing a target.
This package retains that safety behavior and its documented collision risk.

The filename bound reserves eight bytes for SQLite's `-journal` sidecar.
An exact 255-byte database name passed path formatting but failed the actual
provider while SQLite created that sibling. Limit database names to 247 bytes
and retain the existing private staging flow.

The final persistence suite, changed-file Go lint (zero issues), and shell
harness passed after remediation. The shell harness also passed with `TMPDIR`
inside the delivery worktree. Documentation validation, specification lint and
diff checks passed. The work-order results record each RED/GREEN observation.
