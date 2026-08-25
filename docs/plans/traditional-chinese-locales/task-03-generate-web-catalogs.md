---
id: "03-generate-web-catalogs"
title: "Generate web zh-tw and zh-hk catalogs from zh-cn"
status: done
wave: 2
depends_on:
  - "01-glossary-and-converter"
  - "02-register-locales-runtime"
plan: "plan.md"
spec: "../../specs/platform/requirements/traditional-chinese-locales.md"
---

# Task 03: Generate web zh-tw and zh-hk catalogs from zh-cn

## Intent

Materialize full SPA catalogs under `apps/web/src/locales/zh-tw/` and
`zh-hk/` by running the converter against every `zh-cn` namespace, then spot-check
glossary-critical keys.

## Acceptance

- Both locale directories contain the same set of `*.json` namespace files as
  `zh-cn` (currently 30).
- `pnpm run i18n:parity` lists `zh-tw` and `zh-hk` with missing counts comparable
  to `zh-cn` (ideally 0 missing relative to `en` if `zh-cn` is complete).
- Sample assertions: diverging glossary terms render differently for TW vs HK
  on at least software/network/project-class strings when present in catalogs;
  `settings:displayLanguage` (or current key) is Traditional in both.
- No Simplified-only CJK remains in values except intentional exceptions
  documented by the converter (should be none).

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && node scripts/convert-zh-cn-to-zh-hant.mjs --locale all --write
cd apps/web && pnpm run i18n:parity
cd apps/web && pnpm run i18n:check
cd apps && pnpm --filter @kandev/web test -- lib/i18n/index.test.ts
```

## Files likely touched

- `apps/web/src/locales/zh-tw/*.json` (all namespaces)
- `apps/web/src/locales/zh-hk/*.json` (all namespaces)
- Optional glossary-assertion test file

## Dependencies

- Task 01 (converter).
- Task 02 (runtime must accept locales for load tests).

## Parallelism

Sequential with task 04 preferred if sharing the converter CLI; may run after
task 01 alone for generation, but verification that loads locales needs task 02.

## Inputs

- Current `apps/web/src/locales/zh-cn/**` (complete vs `en` as of main).
- Glossary + converter from task 01.

## Output contract

- Checked-in Traditional catalogs; parity/check commands run; status updated.

## Results

- Regenerated all 30 namespaces for each of `zh-tw` and `zh-hk` from the
  current `zh-cn` source with the reviewed glossary and override layer.
- `i18n:parity` confirms both Traditional catalogs match the `zh-cn` coverage:
  each has the same 11 keys missing from `en` as the source catalog.
- `i18n:check` passed; its 139 real-locale parity findings remain advisory
  baseline gaps across `pt-pt` and the three Chinese catalogs.
