---
id: "01-resolved-backup-directory"
title: "Show the resolved backup directory"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001
acceptance_criteria:
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.1
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.2
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.5
  - AC-SYSTEM-PAGE-BACKUP-GUIDANCE-001.6
system_design:
  - ../../specs/system-page/system-design/backup-location-actions.md
---

# Task 01: Show the Resolved Backup Directory

## Summary

Expose the backend-derived SQLite backup directory in database statistics.
Show that value in the Backups section without a symbolic fallback.

## In scope

- Add the additive `backup_directory` database stats field.
- Return an empty directory for non-SQLite drivers.
- Update the frontend type and Backups description.
- Remove the static backup directory constant.
- Add backend, frontend copy, pseudo-locale, and desktop E2E evidence.

## Out of scope

- Change the backup list response.
- Change snapshot storage or permissions.
- Change path placeholders outside the Backups section description.

## Acceptance

- The backend returns the exact sibling directory for default and custom SQLite paths.
- The Backups description shows the returned full path and never guesses a path.
- A long path stays inside the page width.

## Verification

```bash
go test ./internal/system/database -run 'TestStats|TestHandleStats' -count=1
pnpm --filter @kandev/web exec vitest run components/settings/system/system-route-copy.test.ts components/settings/system/system-invisible-copy.test.tsx lib/api/domains/system-api.test.ts
pnpm --filter @kandev/web e2e:run tests/system/backups-page.spec.ts
pnpm --filter @kandev/web run i18n:check
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run the Go command from `apps/backend`.
Run the pnpm commands from `apps`.
Run the final two commands from the repository root.

## Files likely touched

- `apps/backend/internal/system/database/stats.go`
- `apps/backend/internal/system/database/stats_test.go`
- `apps/web/lib/types/system.ts`
- `apps/web/components/settings/system/data-storage-settings.tsx`
- `apps/web/components/settings/system/system-route-shell.tsx`
- `apps/web/components/settings/system/system-route-copy.test.ts`
- `apps/web/components/settings/system/system-invisible-copy.test.tsx`
- `apps/web/lib/api/domains/system-api.test.ts`
- `apps/web/e2e/tests/system/backups-page.spec.ts`

## Dependencies

None.

## Risks

- Duplicate database requests if two components invoke the loading hook.
- Incorrect path separators if the frontend derives the directory.

## Parallelism

`sequential`

## Inputs

- `REQ-SYSTEM-PAGE-BACKUP-GUIDANCE-001`
- The Location contract section in the system design.
- The completed SQLite backup path plan.

## Results

Passed.

- `go test ./internal/system/database -run 'TestStats|TestHandleStats' -count=1` passed (6 tests, including the relative-path absolute-directory regression).
- `pnpm --filter @kandev/web exec vitest run components/settings/system/system-route-copy.test.ts components/settings/system/system-invisible-copy.test.tsx lib/api/domains/system-api.test.ts` passed (52 tests, including the empty backup-directory regression).
- `pnpm --filter @kandev/web e2e:run tests/system/backups-page.spec.ts` passed (3 Chromium tests, including the resolved path and row-action guidance scenarios).
- `pnpm --filter @kandev/web run i18n:check` passed with the repository's existing advisory orphan-catalog warnings.
- `python3 scripts/lint-spec-files.py --all` passed.
- `git diff --check` passed.
- `pnpm --filter @kandev/web run typecheck` passed as an additional contract check.
- Review remediation resolves the derived sibling directory with `filepath.Abs`
  before returning it and propagates resolution errors. The desktop E2E test
  also asserts the API value is absolute and matches the expected sibling.
