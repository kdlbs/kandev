---
id: "01-portable-backup-filenames"
title: "Publish backups for historical version strings"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-SQLITE-SNAPSHOT-001
  - REQ-PLATFORM-STARTUP-LIFECYCLE-001
acceptance_criteria:
  - AC-PLATFORM-SQLITE-SNAPSHOT-001.3
  - AC-PLATFORM-SQLITE-SNAPSHOT-001.4
  - AC-PLATFORM-STARTUP-LIFECYCLE-001.7
system_design:
  - ../../specs/platform/system-design/sqlite-snapshot-copy.md
---

# Task 01: Publish backups for historical version strings

## Summary

Make backup filename formatting accept stored version strings from historical
local builds. Cover the reported failure through the actual persistence
provider before changing production code.

## In scope

- Add `TestProvide_UpgradeFromNamespacedVersion` with the reported SDK version.
- Add `TestSnapshotPathVersionCompatibility` for portable filename cases.
- Apply the design's version normalization in `snapshotPath`.
- Preserve the existing source database, metadata, and publication gates.

## Out of scope

Build version discovery belongs to Task 02. Do not change copying, integrity
validation, backup eligibility, rotation policy, or the user's live database.

## Acceptance

1. The provider regression first fails with the missing-directory publication
   error. After the fix, it publishes a readable snapshot directly in `backups`.
   Check seeded data, unchanged stored metadata, `0600` permissions on POSIX,
   and removal of owned staging directories.
2. Filename cases preserve ordinary versions and the empty-version fallback.
   Both separator styles and reserved characters produce one portable component.
   Include traversal-shaped metadata and the exact reported SDK version.
   Cover the maximum filename component length and distinguish oversized versions
   with the same readable prefix without changing source metadata.
3. Existing publication, cancellation, upgrade eligibility, and failure tests
   continue to pass. A filename collision never replaces an existing backup.

## Verification

Run from the repository root. Run the focused regression before production edits.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/persistence -run 'TestSnapshotPathVersionCompatibility|TestProvide_UpgradeFromNamespacedVersion|TestSnapshotPathVersionLengthLimit|TestProvide_UpgradeFromOversizedVersion' -count=1)
(cd apps/backend && go test -trimpath -tags fts5 ./internal/persistence)
bash scripts/lint-go-changed apps/backend/internal/persistence/snapshot.go apps/backend/internal/persistence/snapshot_version_test.go
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/backend/internal/persistence/snapshot.go`
- `apps/backend/internal/persistence/snapshot_version_test.go`
- `docs/plans/upgrade-backup-version-paths/plan.md`
- `docs/plans/upgrade-backup-version-paths/task-01-portable-backup-filenames.md`

## Dependencies

None.

## Risks

Normalization only changes the filename representation.
Do not rewrite `kandev_meta.kandev_version` to make the test pass.
POSIX permission assertions need the existing Windows-aware test convention.

## Parallelism

`sequential`

## Inputs

- [Snapshot requirements](../../specs/platform/requirements/sqlite-snapshot-copy.md)
- [Startup requirements](../../specs/platform/requirements/startup-lifecycle.md)
- [Stored version compatibility design](../../specs/platform/system-design/sqlite-snapshot-copy.md#stored-version-compatibility)
- `provider.go`: `createPreMigrationBackup` and `provideSQLite`
- `snapshot_copy_test.go`, `upgrade_test.go`, and `provider_legacy_test.go`
- `.agents/skills/tdd/references/backend-tests.md`

## Results

- **RED:** Before production edits, the provider regression using
  `pluginsdk/v0.1.0-16-gbe9d2485554` failed during publication with the expected
  `no such file or directory` error at
  `backups/kandev-pluginsdk/v0.1.0-16-gbe9d2485554-...db`. The path cases also
  failed for namespaced, backslash, traversal-shaped, and reserved-character
  versions; ordinary versions and the empty fallback passed.
- **GREEN:** After filename-component normalization, the focused regression
  command passed:
  `GOCACHE=/tmp/kandev-go-cache go test -trimpath -tags fts5 ./internal/persistence -run 'TestSnapshotPathVersionCompatibility|TestProvide_UpgradeFromNamespacedVersion' -count=1`.
  The provider test verified the readable direct backup, unchanged stored
  metadata, POSIX `0600` permissions, and staging cleanup.
- The complete persistence package passed with
  `GOCACHE=/tmp/kandev-go-cache go test -trimpath -tags fts5 ./internal/persistence`.
- `GOCACHE=/tmp/kandev-go-cache bash scripts/lint-go-changed apps/backend/internal/persistence/snapshot.go apps/backend/internal/persistence/snapshot_version_test.go` passed with `0 issues`.
- `python3 scripts/list-docs.py validate` passed: 436 decisions and 1931
  specifications validated.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.

Go commands used a temporary cache under `/tmp` because the default Go build
cache is read-only in this environment. No files under `/root/.kandev` were
accessed.

### PR remediation: bounded filename components

- **RED:** The 229-character path case produced a 256-byte filename, and the
  actual provider failed with `file name too long`. A 255-byte final basename
  also failed during SQLite staging because its `-journal` sibling exceeded the
  filesystem component limit.
- **GREEN:** The version component now leaves room for that 8-byte journal
  suffix, keeping the published basename at or below 247 bytes. A 220-character
  version reaches the exact 247-byte boundary. Two oversized versions with the
  same normalized prefix produce distinct names from SHA-256 digests of their
  original values. The provider regression with a 229-character stored version
  publishes a readable direct backup and leaves source metadata unchanged.
- `GOCACHE=/tmp/kandev-go-cache go test -trimpath -tags fts5 ./internal/persistence -run 'TestSnapshotPathVersionLengthLimit|TestProvide_UpgradeFromOversizedVersion|TestSnapshotPathVersionCompatibility|TestProvide_UpgradeFromNamespacedVersion' -count=1` passed.
- `GOCACHE=/tmp/kandev-go-cache go test -trimpath -tags fts5 ./internal/persistence` passed.
- `GOCACHE=/tmp/kandev-go-cache bash scripts/lint-go-changed apps/backend/internal/persistence/snapshot.go apps/backend/internal/persistence/snapshot_version_test.go` passed with `0 issues`.
