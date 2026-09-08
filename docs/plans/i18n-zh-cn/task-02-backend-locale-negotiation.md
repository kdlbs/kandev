---
id: "02-backend-locale-negotiation"
title: "Backend locale negotiation"
status: done
wave: 2
depends_on: ["01-frontend-locale-catalogs"]
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n.md"
---

# Task 02: Backend locale negotiation

## Acceptance

- Backend support is synchronized with the frontend and canonicalizes `zh-cn`
  and `zh-CN` to `zh-cn` without weakening unknown-locale English fallback.
- Cookie choice wins over weighted `Accept-Language`; `zh-CN` negotiates to the
  Chinese catalog and server-rendered browser messages differ from English.
- Chinese and English backend catalogs have exactly the same keys, and the first
  shell response renders `<html lang="zh-cn">`.

## Verification

```bash
cd apps/backend && go test ./internal/i18n/... ./internal/webapp/... ./internal/backendapp/...
```

Add the smallest table-driven tests first and observe their expected RED result
before changing locale registration or normalization.

## Files likely touched

- `apps/backend/internal/i18n/locales/zh-cn.json`
- `apps/backend/internal/i18n/i18n.go`
- `apps/backend/internal/i18n/i18n_test.go`
- `apps/backend/internal/webapp/shell_test.go`

## Dependencies

Task 01 defines the canonical frontend locale list that the backend mirrors.

## Parallelism

Sequential because frontend/backend locale-list parity is a shared contract.

## Inputs

- Spec scenarios for cookie, Accept-Language, server errors, and shell lang.
- Plan: **Backend → Locale registration and negotiation**.
- Existing patterns: `parseAcceptLanguage`, `T`, `Keys`, and shell lang tests.

## Output contract

Report RED/GREEN evidence, exact commands and results, catalog key comparison,
files changed, fallback behavior, blockers/risks, and synchronized task/plan
status.

## Results

- RED: `go test ./internal/i18n/... ./internal/webapp/...` failed on Chinese normalization, message lookup, Cookie precedence, weighted `Accept-Language: zh-CN`, and server-rendered `lang`.
- GREEN: `go test ./internal/i18n/... ./internal/webapp/... ./internal/backendapp/...` passed after registering `zh-cn` and normalizing supported locale input to the lowercase canonical id.
- The 34-key backend Chinese catalog exactly matches English; tests reject missing and extra keys.
