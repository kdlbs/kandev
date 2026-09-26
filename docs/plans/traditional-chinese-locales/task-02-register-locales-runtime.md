---
id: "02-register-locales-runtime"
title: "Register zh-tw and zh-hk in FE/BE runtime"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/platform/requirements/traditional-chinese-locales.md"
---

# Task 02: Register zh-tw and zh-hk in FE/BE runtime

## Intent

Teach the SPA and Go shell that `zh-tw` and `zh-hk` are supported locales:
switcher labels, normalization, date-fns mapping, backend cookie /
Accept-Language negotiation. Empty or partial catalogs are OK at this step;
loading must not throw.

## Acceptance

- `SUPPORTED_LOCALES` includes `zh-tw` and `zh-hk`; `LOCALE_LABELS` are
  `繁體中文（台灣）` and `繁體中文（香港）`; production `selectableLocales`
  lists both and still hides `pseudo`.
- `date-locale` maps `zh-tw` → date-fns `zhTW`, `zh-hk` → `zhHK`.
- Backend `supportedLocales` accepts both tags; `FromRequest` honors cookie and
  `Accept-Language: zh-TW` / `zh-HK` (and case variants).
- Existing unit tests updated; new cases cover activation without requiring
  full catalog content (missing keys may fall back to `en`).

## Verification

```bash
cd apps && pnpm install --frozen-lockfile
cd apps && pnpm --filter @kandev/web test -- lib/i18n/index.test.ts lib/i18n/date-locale.test.ts lib/i18n/formats.test.ts lib/i18n/lazy-catalogs.test.ts lib/i18n/boot.test.ts
cd apps/backend && go test ./internal/i18n/... ./internal/webapp/ -count=1
```

## Files likely touched

- `apps/web/lib/i18n/index.ts`
- `apps/web/lib/i18n/index.test.ts`
- `apps/web/lib/i18n/date-locale.ts`
- `apps/web/lib/i18n/date-locale.test.ts`
- `apps/web/lib/i18n/formats.test.ts`
- `apps/web/lib/i18n/lazy-catalogs.test.ts`
- `apps/web/lib/i18n/boot.test.ts`
- `apps/backend/internal/i18n/i18n.go`
- `apps/backend/internal/i18n/i18n_test.go`
- `apps/backend/internal/webapp/shell_test.go`

## Dependencies

- None (catalog directories may be empty until task 03/04).

## Parallelism

`parallel-safe` with task 01.

## Inputs

- Spec: What + Scenarios for switcher, cookie, Accept-Language, formatting.
- Pattern: existing `zh-cn` / `pt-pt` registration.

## Output contract

- Runtime accepts both locales; tests green; task/plan status updated.

## Results

- Focused i18n/runtime Vitest suite: 5 files, 84 tests passed.
- Runtime catalog assertions now verify both reviewed local terminology and
  the Taiwan/Hong Kong pull-request wording difference.
- Backend locale negotiation and shell tests passed via
  `go test ./internal/i18n/... ./internal/webapp/ -count=1`.
