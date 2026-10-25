---
id: "04-windows-database-path"
title: "Open the repair database at native Windows paths"
status: complete
wave: 4
depends_on:
  - "01-preview-repair"
plan: "plan.md"
requirements:
  - REQ-TASKS-WORKTREE-INVENTORY-REPAIR-001
acceptance_criteria:
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.1
  - AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.6
system_design:
  - ../../specs/tasks/system-design/worktree-inventory-repair.md
---

# Task 04: Open the repair database at native Windows paths

## Summary

`openDatabase` built its SQLite DSN with `url.URL`. For a Windows drive-letter
path that produces `file://C:%5C...`, which SQLite parses as a URI authority
and rejects. Preview, verification, and the journal backup check could not open
any database on Windows, although the design keeps preview and verification
available on platforms other than Linux.

## In scope

- Build the DSN as `file:<path>?<encoded options>`, with only `%`, `?`, and
  `#` in the path percent-escaped. All remaining characters, including a drive
  letter and backslashes, stays literal. Keep every mode and connection option.
- Add table tests that open a real SQLite file at a native absolute path whose
  directory name contains a space, `#`, `%41`, or, on platforms other than
  Windows, `?`. The read-only connection reads a seeded row and rejects a
  write; the read-write connection writes to the seeded table. Neither creates
  another file.
- Run those tests in the targeted Windows CI step and the matching
  `apps/backend/Makefile` `test-windows` line.

## Out of scope

- Windows handling of Git paths during registration inspection. Git reports
  forward-slash paths there, so Windows preview can still stop at that check.
- Application and rollback on platforms other than Linux.
- The live database openers in `internal/db`.

## Acceptance

- `AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.6`: for each path above, the
  read-only connection reads the database file, rejects a write, and keeps
  foreign keys on; the read-write connection writes to it and keeps
  `synchronous=FULL`; the journal backup check accepts the same path; no other
  file is created.
- `AC-TASKS-WORKTREE-INVENTORY-REPAIR-001.1`: the read-only connection used by
  preview rejects writes and creates no file.

## Verification

```bash
cd apps/backend && go test -tags fts5 ./internal/task/inventoryrepair -run '^TestOpenDatabase' -count=1 -v
cd apps/backend && go test -race -v ./internal/task/inventoryrepair -run '^TestOpenDatabase(Reads|Writes)NativeAbsolutePath$'
cd apps/backend && golangci-lint run ./internal/task/inventoryrepair/... --new-from-rev=fba6f2b3281bc379904bf17509184f09b81e97d9 --timeout=10m
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Results

- On Windows against the unmodified package, every Windows case of both new
  tests failed with `invalid uri authority`. After the change they pass. The
  `#` and `%41` cases also fail when the path follows `file:` unescaped.
- On Windows the existing preview tests now pass the database open and stop at
  the separate Git registration check described above.
- The `?` case and the other tests were not run on Linux or macOS locally; the
  Linux backend CI suite runs them with the rest of the package.
