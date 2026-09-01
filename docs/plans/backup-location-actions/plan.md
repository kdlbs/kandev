---
created: 2026-08-27
status: done
requirements:
  - REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001
system_design:
  - ../../specs/system-page/system-design/backup-location-actions.md
legacy_specs: []
---

# Implementation Plan: Backup Location and Action Guidance

## Overview

First, expose the absolute, resolved SQLite backup directory through database statistics.
Then show that directory and add guidance to each backup row action.

The order establishes one server-owned path before the frontend renders it.

## Scope

### In scope

- Add the resolved SQLite backup directory to database statistics.
- Show the directory in the Backups section description.
- Add hover and focus tooltips to Download, Restore, and Delete.
- Keep action targets usable on coarse pointers.
- Prove desktop and mobile behavior with focused Playwright tests.

### Out of scope

- Change backup storage, retention, or operation behavior.
- Add per-snapshot absolute paths to the backup list.
- Change backup permissions.
- Add PostgreSQL snapshot support.
- Change backup placeholders in other dialogs.
- Change public documentation that already describes the sibling directory.

## Technical approach

### Resolved directory

Add `BackupDirectory` to `database.Stats` in `apps/backend/internal/system/database/stats.go`.
Return the absolute `backups` sibling path for SQLite, resolving the derived
path with `filepath.Abs`, and an empty value for PostgreSQL.

Add `backup_directory` to `DatabaseStats` in `apps/web/lib/types/system.ts`.
Use the shared database state in `data-storage-settings.tsx` for the Backups description.

Remove the static `BACKUP_DIR` value from `system-route-shell.tsx`.
Keep `BACKUP_SQL_COMMAND` as a stable interpolated command value.

### Action tooltips

Wrap each control in `BackupRowActions` with the shared Tooltip primitives.
Use localized operation-only labels as tooltip content. Keep each
snapshot-specific `aria-label` on the interactive control.

Add pointer-specific 44-pixel minimum dimensions to the three controls.
Keep the current compact dimensions for fine-pointer desktop use.

## Tests

- `apps/backend/internal/system/database/stats_test.go` covers default, custom, and non-SQLite directory values.
- `apps/web/components/settings/system/system-route-copy.test.ts` covers the exact dynamic English description.
- `apps/web/components/settings/system/system-invisible-copy.test.tsx` keeps the resolved path literal in the pseudo-locale.
- `apps/web/components/settings/system/backups-table.test.tsx` covers tooltip labels, accessible names, and touch-target classes.

## E2E tests

- `apps/web/e2e/tests/system/backups-page.spec.ts` covers the resolved path and all three hover tooltips for `AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.1` through `.3`.
- `apps/web/e2e/tests/auth/mobile-system-data-storage-member-gating.spec.ts` covers action size and horizontal containment for `.4` and `.5`.

## Work orders

- [x] [Task 01: Show the resolved backup directory](task-01-resolved-backup-directory.md)
- [x] [Task 02: Describe backup row actions](task-02-backup-row-action-guidance.md)

## Verification results

Passed.

- `go test ./internal/system/database -run 'TestStats|TestHandleStats' -count=1` passed (6 tests, including the relative-path absolute-directory regression).
- `pnpm --filter @kandev/web exec vitest run components/settings/system/system-route-copy.test.ts components/settings/system/system-invisible-copy.test.tsx lib/api/domains/system-api.test.ts` passed (52 tests, including the empty backup-directory regression).
- `pnpm --filter @kandev/web exec vitest run components/settings/system/backups-table.test.tsx` passed (4 tests).
- `pnpm --filter @kandev/web e2e:run tests/system/backups-page.spec.ts` passed (3 Chromium tests).
- `pnpm --filter @kandev/web e2e:run --project mobile-chrome tests/auth/mobile-system-data-storage-member-gating.spec.ts` passed (2 mobile Chromium tests).
- `pnpm --filter @kandev/web run typecheck` passed.
- `pnpm --filter @kandev/web run i18n:check` passed with the repository's existing advisory orphan-catalog warnings.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- Review remediation passed: the backend resolves the derived directory with
  `filepath.Abs` and returns an error if resolution fails; the desktop E2E test
  independently asserts that the API value is absolute and matches the
  database-path sibling.

## Risks

- A frontend path reconstruction can break Windows separators. The backend must own the derived value.
- A second database hook can send duplicate requests. The section must use the existing shared state.
- A tooltip wrapper can change link composition. The download anchor must remain the interactive element.
- Long filenames can widen a tooltip or the table. Both surfaces need wrapping and containment checks.
