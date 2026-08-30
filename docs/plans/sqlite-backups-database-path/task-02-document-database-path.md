---
id: "02-document-database-path"
title: "Document the SQLite backup location"
status: done
wave: 2
depends_on: ["01-backend-database-path"]
plan: "plan.md"
spec: "../../specs/system-page/requirements/system-page.md"
---

# Task 02: Document the SQLite backup location

## Intent

Replace the documented custom-path limitation with the corrected System behavior. Keep the legacy-snapshot and home-storage boundaries explicit.

## Acceptance

- Public references state that SQLite snapshots use `backups/` beside the configured database file.
- Restore documentation names `<configured-database-path>.new` and the exact configured destination.
- Restore documentation explains that scheduling, active executions, and database-backed workers stop, the SQLite pool closes, replacement is rollback-safe, PostgreSQL restore is rejected, and an immediate restart is required.
- Public and agent guidance states that old misrouted snapshots do not move automatically.

## Files likely touched

- `docs/public/cli.md`
- `docs/public/configuration.md`
- `docs/public/operations.md`
- `docs/public/feature-status.md`
- `docs/public/use-kandev.md`
- `apps/backend/AGENTS.md`
- `apps/web/components/settings/system/restore-dialog.tsx`
- `apps/web/src/locales/*/system.json`

## Dependencies

Task 01 must establish the final backend behavior.

## Parallelism

Sequential. These pages document the result of Task 01.

## Inputs

- Spec: `docs/specs/system-page/requirements/system-page.md`, configured database path scenarios and persistence guarantees.
- Plan: `plan.md`, public documentation and risk sections.
- Docs type: `configuration.md` and `cli.md` are references. `operations.md` is a how-to guide. `feature-status.md` is a reference. `use-kandev.md` is a tutorial.

## Verification

Run the focused terminology search and public-doc validators:

```bash
rg -n 'KANDEV_DATABASE_PATH|data/backups|database path|backup caveat' docs/public apps/backend/AGENTS.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

## Output contract

Report each changed public page and its primary documentation type. Report validator results and remaining intentional `data/backups` references. Update this task and `plan.md` in the same conversation.

## Restore safety remediation

- Updated the public and backend guidance for checkpoint validation, quarantine rollback, PostgreSQL rejection, and the closed-pool restart requirement.
- Updated the localized restore success dialog to invoke the existing restart flow and to provide manual quit-and-relaunch guidance.

## Results

- Public docs updated:
  - `docs/public/cli.md` (reference)
  - `docs/public/configuration.md` (reference)
  - `docs/public/operations.md` (how-to and recovery guide)
  - `docs/public/feature-status.md` (reference)
  - `docs/public/use-kandev.md` (tutorial)
- Backend guidance updated: `apps/backend/AGENTS.md` now names the configured
  SQLite path and its sibling backup directory.
- `rg -n 'KANDEV_DATABASE_PATH|data/backups|database path|backup caveat'
  docs/public apps/backend/AGENTS.md` passed. Remaining `data/backups`
  references describe the default path, development safety snapshots, or the
  home-based System Status walk.
- `node --test scripts/validate-public-docs.test.mjs` passed 61 tests.
- `node scripts/validate-public-docs.mjs` validated 41 published docs pages.
- `git diff --check` passed.
