---
id: "02-remove-obsolete-catalog-entries"
title: "Remove obsolete catalog entries"
status: done
wave: 2
depends_on: ["01-localize-watcher-and-task-fallbacks"]
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n-audit-watcher-copy.md"
---

# Task 02: Remove Obsolete Catalog Entries

## Acceptance

- All 12 audit-reported keys, confirmed obsolete through current-source and source-history review,
  are absent from English, pseudo, Portuguese, and Simplified Chinese catalogs.
- `i18n:check` reports none of those orphan entries and pseudo remains synchronized; existing
  real-locale parity advisories are not repaired in this task.
- The required component/lib sweep reports no hardcoded user-facing copy introduced or retained by
  this repair.

## Verification

```bash
cd apps/web && pnpm run i18n:check && pnpm run i18n:sweep -- components lib
```

## Files likely touched

- `apps/web/src/locales/en/agents.json`
- `apps/web/src/locales/en/github.json`
- `apps/web/src/locales/en/settings.json`
- `apps/web/src/locales/en/task.json`
- matching files under `apps/web/src/locales/pseudo/`
- matching files under `apps/web/src/locales/pt-pt/`
- matching files under `apps/web/src/locales/zh-cn/`
- this task file and `plan.md`

## Dependencies

Task 01.

## Parallelism

Sequential. It validates shared catalogs after Task 01 adds the new fallback references.

## Inputs

- The source-history classification recorded in `plan.md`.
- Task 01's final catalog key set.
- User constraint excluding the real-locale parity finding.

## Output contract

Report the removed key list, exact checker/sweep results including unchanged advisory count, files
changed, blockers and risks, then update this task and `plan.md` statuses.

## Results

`cd apps/web && pnpm run i18n:check && pnpm run i18n:sweep -- components lib` passed. The checker reports 0 orphan entries, pseudo in sync, and the unchanged 127 out-of-scope Portuguese/Chinese parity advisories. The sweep scanned 1,850 files and reported only the two documented prompt-builder plural concatenations plus 91 review-by-eye literals; none is introduced or retained by this repair.

All 12 named obsolete keys were removed from English, pseudo, Portuguese, and Simplified Chinese catalogs. External side effects and security/trust boundaries: None.
