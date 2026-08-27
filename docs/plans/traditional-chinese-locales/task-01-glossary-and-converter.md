---
id: "01-glossary-and-converter"
title: "Glossary machine form and zh-cn→zh-hant converter"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/traditional-chinese-locales.md"
---

# Task 01: Glossary machine form and zh-cn→zh-hant converter

## Intent

Make the product [glossary](glossary.md) executable: a checked-in machine-readable
term table plus a script that converts `zh-cn` JSON catalogs to `zh-tw` / `zh-hk`
via OpenCC (`s2twp` / `s2hk`) and glossary overrides, preserving placeholders,
Trans tags, and brands.

## Acceptance

- A machine glossary (JSON or structured data next to the plan/script) encodes
  at least the core product and general UI rows from `glossary.md`, with
  distinct `zh-tw` / `zh-hk` values where the regions diverge (e.g. 軟體/軟件,
  網路/網絡, 專案/項目).
- `apps/web/scripts/convert-zh-cn-to-zh-hant.mjs` (name may match repo script
  conventions) accepts `--locale zh-tw|zh-hk|all`, dry-run by default, and
  `--write` to emit namespace JSON; it never mutates `en`, `pseudo`, or `zh-cn`.
- Unit coverage proves: placeholder `{{x}}` and `<1>…</1>` survive; brand
  `GitHub` unchanged; a glossary override beats bare OpenCC for at least one
  TW/HK diverging pair.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps/web && pnpm exec node --test scripts/convert-zh-cn-to-zh-hant.test.mjs
# or the project's equivalent vitest path if the test is co-located as .test.ts
```

Also dry-run once against a single small namespace:

```bash
cd apps/web && node scripts/convert-zh-cn-to-zh-hant.mjs --locale all --namespace common
```

## Files likely touched

- `docs/plans/traditional-chinese-locales/glossary.md` (only if terms need edit)
- `apps/web/scripts/convert-zh-cn-to-zh-hant.mjs` (or similar)
- `apps/web/scripts/convert-zh-cn-to-zh-hant.test.mjs` (or `.test.ts`)
- `apps/web/scripts/lib/zh-hant-glossary.json` (or embedded table)
- `apps/web/package.json` (devDependency for OpenCC if required)

## Dependencies

- None.

## Parallelism

`parallel-safe` with task 02 (disjoint files: scripts/docs vs runtime).

## Inputs

- Spec: conversion base `zh-cn`, tags `zh-tw`/`zh-hk`, no `zh-cn` fallback.
- Plan: Frontend → Catalogs; [glossary](glossary.md).
- Pattern: existing `apps/web/scripts/i18n-*.mjs` for locales dir discovery.

## Output contract

- Converter + tests land; dry-run works; task/plan status updated.

## Results

- Converter now performs one OpenCC pass followed by one compiled glossary
  pass. Source, base-Traditional, and target forms are matched so surrounding
  text cannot bypass a product term, while longer target phrases remain
  idempotent.
- Reviewed per-key overrides are applied last. Unknown override keys and
  residual Simplified or malformed output fail before a catalog is written.
- `node --test scripts/convert-zh-cn-to-zh-hant.test.mjs`: 19 passed.
- Full dry-run: 15,878 web messages, 0 web/backend residual warnings.
