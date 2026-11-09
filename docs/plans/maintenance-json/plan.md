---
created: 2026-10-28
status: implemented
requirements:
  - REQ-PLATFORM-MAINTENANCE-JSON-001
system_design:
  - ../../specs/platform/system-design/maintenance-json.md
legacy_specs: []
---

# Implementation plan: Maintenance JSON output

## Overview and scope

Add opt-in structured CLI presentation of the existing SQLite maintenance
outcome in one sequential slice. Preserve human output, dry-run behavior,
retention policy, lock, backup and compaction. No UI, new database query,
metric, dependency or schema migration is included. The public CLI reference
is updated with the implementation; its draft edit accompanies this package
for contract review and must not be published independently.

## Technical approach

`apps/backend/internal/launcher/maintenance_database.go` owns parsing, help,
outcome selection, diagnostics and a purpose-built JSON representation.
Map the existing `maintenance.Outcome`, `Report`, `ExecutionResult`, and
`CompactionResult` as specified in the design. Keep committed outcomes visible
after a later error and propagate writer failures. Keep the domain `Run` and
its safety guards unchanged. `docs/public/cli.md` is the user-facing reference.

## Tests and end-to-end evidence

`maintenance_database_test.go` exercises `runMaintenance` with isolated
SQLite and captured stdout/stderr/exit for JSON dry run, execution, compaction,
lock failure, unchanged default text and usage/help. Decode exactly one object
and EOF, assert field values, absence and limits, and prove dry-run
nonmutation and backup. Test post-commit failure with a narrow deterministic
seam and writer failure separately. This real native CLI entrypoint supplies
end-to-end evidence for ACs .1-.4; public documentation supplies .5. No browser
or mobile behavior changes.

## Work orders

- [x] [Task 01: Present maintenance JSON](task-01-maintenance-json.md)

## Verification results

ROOT accepted the design. The entrypoint RED was the existing unknown-flag
exit; a separate short-write regression also failed before its fix. Focused
launcher tests passed with `-race -trimpath -tags fts5 -p=1 -count=1`;
changed-Go lint passed. The documented jq query returned `1` for an isolated
fixture. Specification catalog and lint, public-doc validators (63 tests), and
`git diff --check` passed. See the work order for commands and results.
A subsequent SIGPIPE review correction passed a real closed-stdout subprocess
regression, the focused launcher race suite, and changed-Go lint. Hosted CI
and reviews remain pending.

## Risks

A command may have committed work even when it exits 1. `partial` represents
bounded maintenance work and does not encode every failure; scripts must check
the exit status. A failed stdout write can leave incomplete JSON. Samples and
estimates are not complete totals or promised physical space savings.
