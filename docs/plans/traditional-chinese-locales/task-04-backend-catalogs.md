---
id: "04-backend-catalogs"
title: "Backend zh-tw and zh-hk message catalogs"
status: done
wave: 2
depends_on:
  - "01-glossary-and-converter"
  - "02-register-locales-runtime"
plan: "plan.md"
spec: "../../specs/platform/requirements/traditional-chinese-locales.md"
---

# Task 04: Backend zh-tw and zh-hk message catalogs

## Intent

Provide Traditional Chinese for the small backend browser-facing catalog
(shell errors / shared-task artifacts), converted from `zh-cn.json` with the
same OpenCC + glossary pipeline.

## Acceptance

- `apps/backend/internal/i18n/locales/zh-tw.json` and `zh-hk.json` exist, key
  sets match `zh-cn.json` / `en.json` shape.
- `T("zh-tw", …)` and `T("zh-hk", …)` return Traditional strings distinct from
  `en` for a known key (e.g. `webapp.shellUnavailable`).
- Glossary overrides apply where those short strings contain product UI terms.

## Verification

```bash
cd apps/backend && go test ./internal/i18n/... -count=1
# if converter writes backend files:
cd apps/web && node scripts/convert-zh-cn-to-zh-hant.mjs --locale all --backend --write
```

## Files likely touched

- `apps/backend/internal/i18n/locales/zh-tw.json`
- `apps/backend/internal/i18n/locales/zh-hk.json`
- `apps/backend/internal/i18n/i18n_test.go`
- Converter script flags if backend output is added in task 01/03

## Dependencies

- Task 01 for conversion path.
- Task 02 for `Supported` map (already done).

## Parallelism

`parallel-safe` with task 03 if the converter supports simultaneous writes to
disjoint trees; otherwise sequential after shared converter flags stabilize.

## Inputs

- `apps/backend/internal/i18n/locales/zh-cn.json` as source.
- Spec backend catalog requirement.

## Output contract

- Backend catalogs committed; Go tests green; status updated.

## Results

- Regenerated both 34-key backend catalogs through the same glossary and
  override-aware converter; residual warning count is zero.
- `go test ./internal/i18n/... ./internal/webapp/ -count=1` passed.
