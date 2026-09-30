---
id: "05-windows-backup-verification"
title: "Verify retention backups at native Windows paths"
status: completed
wave: 5
depends_on:
  - "02-guarded-cleanup"
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002
acceptance_criteria:
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.2
  - AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.8
system_design:
  - ../../specs/system-page/system-design/tool-payload-retention.md
---

# Task 05: Windows Backup Verification

## Summary

Backup verification built its read-only SQLite DSN with `url.URL`. For a
Windows drive-letter path that produces `file://C:%5C...`, which SQLite parses
as a URI authority and rejects. Verification therefore failed for every backup
on Windows, and preparation with a selected backup could never arm cleanup.

## In scope

- Open the snapshot in `verifySnapshot` as `file:<path>?mode=ro`, with only
  `%`, `?`, and `#` in the path percent-escaped. Every other character,
  including a drive letter and backslashes, stays literal. The connection
  options are unchanged.
- Add a table test that verifies a real SQLite file at a native absolute path
  whose directory name contains a space, `#`, `%41`, or, on platforms other
  than Windows, `?`. For each path it checks the SHA-256, reads a seeded row,
  rejects a write, rejects a non-database file at the same path, and finds no
  other file created.
- Run that test in the targeted Windows CI step and the matching
  `apps/backend/Makefile` `test-windows` line.

## Out of scope

- The live database openers in `internal/db`, which still write the path
  after `file:` without escaping.
- Persisting a failure detail for preparation or adding scheduler logging.
- Changing POSIX permission assertions in existing backup tests.

## Acceptance

- `AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.8`: verification checks the backup
  file itself through a read-only connection for each path above, and creates
  no other file.
- `AC-SYSTEM-PAGE-TOOL-PAYLOAD-RETENTION-002.2`: the verification that gates
  cleanup now opens Windows paths, and a file at such a path that is not a
  database fails it. Arming cleanup in `internal/system/toolretention` is
  unchanged.

## Verification

```bash
cd apps/backend && go test -p 2 -tags fts5 ./internal/system/backups ./internal/system/toolretention -count=1
cd apps/backend && go test -race -v ./internal/system/backups -run '^TestVerifySnapshotOpensNativeAbsolutePath$'
cd apps/backend && golangci-lint run ./internal/system/... --new-from-rev=fba6f2b3281bc379904bf17509184f09b81e97d9 --timeout=10m
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- On Windows against the unmodified package, every Windows case of the new
  test failed with `invalid uri authority`. After the change they pass. The
  `#` and `%41` cases also fail when the path follows `file:` unescaped.
- On Windows, `TestManualBackupIsPrivate` fails before and after this change
  because Windows reports `0666` for a writable file; it is unrelated.
- The `?` case and the other tests were not run on Linux or macOS locally; the
  Linux backend CI suite runs them with the rest of the package.
