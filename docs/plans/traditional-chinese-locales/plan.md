---
spec: docs/specs/platform/requirements/traditional-chinese-locales.md
created: 2026-08-12
status: done
---

# Implementation Plan: Traditional Chinese locales (zh-tw / zh-hk)

## Overview

Ship Traditional Chinese for Taiwan (`zh-tw`) and Hong Kong (`zh-hk`) by
registering the locales end-to-end (frontend runtime, date-fns, backend
negotiation), then generating catalogs from current `zh-cn` via OpenCC-class
conversion plus the product [glossary](glossary.md). High-traffic namespaces
get a human review pass before ship. Real-locale parity stays advisory.

Order: contracts and tooling first (so catalogs can be generated and checked),
then full catalog materialization, then switcher/docs/E2E polish.

---

## Backend

### Locale support map

- `apps/backend/internal/i18n/i18n.go`: add `"zh-tw": true`, `"zh-hk": true` to
  `supportedLocales` (keep comment sync with frontend `SUPPORTED_LOCALES`).
- `apps/backend/internal/i18n/locales/zh-tw.json` and `zh-hk.json`: convert from
  `zh-cn.json` with the same pipeline/glossary as the web catalogs.
- Tests in `i18n_test.go` and `shell_test.go`: cookie + Accept-Language
  normalization for both tags; message lookup differs from `en`.

---

## Frontend

### Runtime registration

- `apps/web/lib/i18n/index.ts`: extend `SUPPORTED_LOCALES` and `LOCALE_LABELS`
  (`繁體中文（台灣）`, `繁體中文（香港）`). Lazy globs already discover
  `src/locales/*/*.json` — no bundling change beyond the new directories.
- `apps/web/lib/i18n/date-locale.ts`: lazy loaders for date-fns `zhTW` / `zhHK`.
- Unit tests: `index.test.ts`, `date-locale.test.ts`, `formats.test.ts`,
  `boot.test.ts`, `lazy-catalogs.test.ts` updated for the two locales.

### Catalogs

- Create `apps/web/src/locales/zh-tw/` and `zh-hk/` with the same 30 namespace
  files as `zh-cn`.
- Generator script (devDependency OpenCC or equivalent CLI):
  `apps/web/scripts/convert-zh-cn-to-zh-hant.mjs`
  - Input: `src/locales/zh-cn/*.json` (+ backend `zh-cn.json`).
  - Modes: `--locale zh-tw|zh-hk|all`, `--write`, dry-run diff summary.
  - Steps: OpenCC (`s2twp` / `s2hk`) → glossary phrase replace (longest first)
    from `docs/plans/traditional-chinese-locales/glossary.md` or a machine
    sibling JSON derived from it → preserve placeholders / Trans tags / brands.
  - Optional: fail if residual Simplified-only characters remain.
- Check in generated JSON (same model as hand-maintained `zh-cn`). Re-run the
  script when `zh-cn` is bulk-updated; do not invent a runtime converter.

### Language switcher / settings

- Any hard-coded locale list outside `SUPPORTED_LOCALES` / `selectableLocales`
  must pick up the new tags (search settings appearance UI tests).
- Endonyms come only from `LOCALE_LABELS` (not catalog keys), matching `zh-cn`.

### Docs

- `docs/i18n.md` and `docs/specs/platform/requirements/i18n.md`: shipped locale list includes
  `zh-tw` / `zh-hk`.
- Keep this plan's [glossary](glossary.md) as the living term table.

---

## Tests

| What                           | File                                                           | How                                                                     |
| ------------------------------ | -------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Supported + normalize + labels | `apps/web/lib/i18n/index.test.ts`                              | Expect both locales in `SUPPORTED_LOCALES`, labels, `selectableLocales` |
| Date locale mapping            | `apps/web/lib/i18n/date-locale.test.ts`                        | `zh-tw` → zh-TW, `zh-hk` → zh-HK                                        |
| Intl formatting                | `apps/web/lib/i18n/formats.test.ts`                            | activate each locale; number/date path uses tag                         |
| Catalog load                   | `apps/web/lib/i18n/lazy-catalogs.test.ts` / `index.test.ts`    | resolve a known key (e.g. `settings:displayLanguage`)                   |
| Glossary-critical terms        | new `apps/web/scripts/zh-hant-glossary.test.ts` or `.mjs` test | sample keys differ TW vs HK where glossary requires                     |
| Backend Supported/FromRequest  | `apps/backend/internal/i18n/i18n_test.go`                      | cookie + Accept-Language                                                |
| Shell lang attribute           | `apps/backend/internal/webapp/shell_test.go`                   | `lang="zh-tw"` / `zh-hk`                                                |
| Conversion script integrity    | unit test next to script                                       | placeholders and brands preserved on fixture strings                    |

---

## E2E Tests

- **Scenario:** select 繁體中文（台灣）/ 繁體中文（香港）in Settings → Appearance;
  UI shows Traditional copy; reload keeps cookie.
- **File:** extend existing locale switcher coverage if present, else
  `apps/web/e2e/tests/settings/locale-switcher-zh-hant.spec.ts` (name may
  match the existing appearance/locale spec pattern).
- **What to verify:** language option labels, a stable chrome string (sidebar
  or settings heading) is Traditional, `document.documentElement.lang`.

---

## Verification Results

- `node --test scripts/convert-zh-cn-to-zh-hant.test.mjs` — 19 passed
- Full converter dry-run — 15,878 web messages; web/backend residual warnings = 0
- `pnpm run i18n:parity` — pass; zh-cn / zh-tw / zh-hk share 11 current en-gap keys
- `pnpm run i18n:check` — pass with 139 advisory baseline parity findings
- Focused i18n/runtime Vitest — 5 files, 84 tests passed
- `pnpm run typecheck` — pass
- Focused ESLint for changed source/tests — pass; full lint retains one
  pre-existing `formats.test.ts` duplicate-string warning
- Prettier check for changed source, catalogs, and docs — pass
- `go test ./internal/i18n/... ./internal/webapp/ -count=1` — pass
- Managed production-build E2E `tests/i18n/language-switch.spec.ts` — 5 passed

---

## Implementation Waves And Parallel Candidates

```
Wave 1:
- [x] [task-01-glossary-and-converter](task-01-glossary-and-converter.md)
- [x] [task-02-register-locales-runtime](task-02-register-locales-runtime.md)

Wave 2 (after wave 1):
- [x] [task-03-generate-web-catalogs](task-03-generate-web-catalogs.md)
- [x] [task-04-backend-catalogs](task-04-backend-catalogs.md)

Wave 3:
- [x] [task-05-high-traffic-review](task-05-high-traffic-review.md)
- [x] [task-06-docs-and-e2e](task-06-docs-and-e2e.md)
```

Wave 1 tasks touch disjoint areas (script/docs vs runtime registration) and
are parallel-safe if the user authorizes subagents. Wave 2 depends on the
converter. Wave 3 depends on generated catalogs.

Default execution is sequential in the primary conversation.

---

## Open Questions

(None — defaults fixed in the spec.)
