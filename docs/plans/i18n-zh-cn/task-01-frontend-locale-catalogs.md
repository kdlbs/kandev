---
id: "01-frontend-locale-catalogs"
title: "Frontend zh-cn integration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n.md"
---

# Task 01: Frontend zh-cn integration

## Acceptance

- `zh-cn` is supported and selectable in production with fixed label `简体中文`;
  only `pseudo` remains production-hidden.
- All 18 Chinese namespace catalogs exactly mirror the English catalogs and
  preserve variables, plural suffixes, `<Trans>` tags, tokens, and brand names.
- Activation and boot restoration set i18next, `<html lang>`, and the locale
  cookie to `zh-cn`; Intl wrappers use Chinese locale data while pseudo uses en.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm exec vitest run lib/i18n/index.test.ts lib/i18n/boot.test.ts lib/i18n/formats.test.ts
cd apps/web && pnpm run typecheck
```

Run each changed behavioral test once in RED before production edits, then rerun
the exact focused suite after the final production or catalog edit.

## Files likely touched

- `apps/web/src/locales/zh-cn/common.json`
- `apps/web/src/locales/zh-cn/settings.json`
- `apps/web/lib/i18n/index.ts`
- `apps/web/lib/i18n/index.test.ts`
- `apps/web/lib/i18n/boot.test.ts`
- `apps/web/lib/i18n/formats.test.ts`

## Dependencies

None.

## Parallelism

Sequential. Catalogs and locale registration form one runtime contract.

## Inputs

- Spec: **What**, **Scenarios**, **Failure modes**, and **Resolved decisions**.
- Plan: **Frontend → Catalogs and locale runtime**.
- Existing patterns: `en` and generated `pseudo` catalogs, locale activation,
  boot resolution, and current Intl wrapper tests.

## Output contract

Report RED/GREEN evidence, key counts by namespace, files changed, exact commands
and results, translation preservation checks, blockers/risks, and synchronized
task/plan status.

## Results

- RED: `pnpm exec vitest run lib/i18n/index.test.ts lib/i18n/boot.test.ts lib/i18n/formats.test.ts` failed 8 of 20 tests because `zh-cn` normalized to `en`, was absent from both selectable lists, and formatted with English Intl data.
- GREEN: the same focused command passed 20 of 20 tests after registering the locale, loading the catalogs, and configuring i18next to retain the lowercase canonical resource id.
- Catalogs: 18 namespaces and 3,300 keys; every namespace, key, and key order
  matches English, with identical interpolation-placeholder and `<Trans>` tag
  sets. Two messages reorder named placeholders to follow natural Chinese
  grammar without changing their values or markup.
- `pnpm run typecheck` passed.
