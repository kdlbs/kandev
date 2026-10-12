---
id: "02-backend-ja-locale-negotiation"
title: "Backend ja locale negotiation"
status: done
wave: 2
depends_on:
  - "01-frontend-ja-locale-catalogs"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-JAPANESE-LOCALE-001
acceptance_criteria:
  - AC-PLATFORM-JAPANESE-LOCALE-001.4
system_design:
  - ../../specs/platform/system-design/i18n.md
---

# Task 02: Backend ja locale negotiation

## Outcome

The Go shell and server-rendered browser surfaces accept `ja`:
`apps/backend/internal/i18n/locales/ja.json` is embedded, `Supported` /
`Normalize` / `FromRequest` (cookie and `Accept-Language`) resolve `ja`, and a
`ja` runtime locale produces `<html lang="ja">` in the first server response.

## In scope

- `apps/backend/internal/i18n/locales/ja.json`: translate the three
  server-rendered browser error messages with an exact key match to `en.json`
  (72 keys, mirroring `zh-cn.json` in shape).
- `apps/backend/internal/i18n/i18n.go`: add `"ja": true` to `supportedLocales`
  (keep in sync with `apps/web/lib/i18n/index.ts`).
- `apps/backend/internal/i18n/i18n_test.go`: cover `ja` canonical normalization,
  case-insensitive BCP-47 input, Japanese message lookup, exact catalog parity,
  cookie precedence, q-value ordering, region matching, and English fallback.
- `apps/backend/internal/webapp/shell_test.go`: prove a `ja` runtime locale
  produces `<html lang="ja">` in the first server response.

## Exclusions

- Frontend `ja` catalogs and runtime — covered by Task 01.
- E2E specs — Task 03.

## Applicable requirements

`REQ-PLATFORM-JAPANESE-LOCALE-001.4`; `AC-PLATFORM-I18N-001.7` (`<html lang>`
reflects active locale).

## System design

`docs/specs/platform/system-design/i18n.md` — API surface (Go shell), Data
model, Scenarios (Accept-Language negotiation), Resolved decisions.

## Implementation acceptance conditions

1. `Supported("ja")`, `Normalize("ja-JP") === "ja"`, and a request with
   `kandev_locale=ja` resolves `ja`; `Accept-Language: ja` without a cookie
   also resolves `ja`.
2. `ja.json` has exactly the same keys as `en.json` (embedding loads without
   error; parity is exact for backend catalogs, as with `zh-cn`).
3. `shell_test.go` proves `lang="ja"` in the served HTML.

## Verification

```bash
(cd apps/backend && make test)   # or: go test ./internal/i18n/... ./internal/webapp/...
```

RED first: add the failing tests before production edits, confirm the expected
failure, then implement and re-run.

## Files likely touched

- `apps/backend/internal/i18n/locales/ja.json`
- `apps/backend/internal/i18n/i18n.go`
- `apps/backend/internal/i18n/i18n_test.go`
- `apps/backend/internal/webapp/shell_test.go`

## Dependencies

Task 01 (frontend registration is the reference runtime; backend mirrors it).

## Parallelism

Sequential after Task 01.

## Inputs

- Spec: **Acceptance criteria**, **Scenarios** (cookie, Accept-Language),
  **Failure modes**.
- Plan: **Wave 2**.
- Existing patterns: `zh-cn`/`zh-tw` addition in `i18n.go`, `i18n_test.go`
  coverage, and `shell_test.go`.

## Output contract

Report RED/GREEN evidence per test, exact commands and results, key-count
parity check for `ja.json` vs `en.json`, files changed, blockers/risks, and
synchronized task/plan status.

## Results

- Catalog: `apps/backend/internal/i18n/locales/ja.json` 72/72 keys vs `en.json`.
- `supportedLocales` includes `ja`. `Normalize("ja-JP")` collapses to `ja`.
- `FromRequest`: cookie `ja` wins; cookie `ja-JP` collapses; `Accept-Language: ja`
  and `ja-JP` resolve `ja`. Invalid cookie (`klingon`) is ignored and does not
  skip Accept-Language (regression: `Normalize` fallback to `en` would have).
- `shell_test.go`: `lang="ja"` and `ja-JP` → `ja`.
- Tests: `gofmt` + `go test ./internal/i18n/... ./internal/webapp/...` GREEN.
