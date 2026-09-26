---
id: "01-frontend-ja-locale-catalogs"
title: "Frontend ja locale catalogs and runtime integration"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-JAPANESE-LOCALE-001
acceptance_criteria:
  - AC-PLATFORM-JAPANESE-LOCALE-001.1
  - AC-PLATFORM-JAPANESE-LOCALE-001.2
  - AC-PLATFORM-JAPANESE-LOCALE-001.3
  - AC-PLATFORM-JAPANESE-LOCALE-001.5
system_design:
  - ../../specs/platform/system-design/i18n.md
---

# Task 01: Frontend ja catalogs and locale runtime integration

## Outcome

`ja` is a shipped, production-selectable locale labeled `日本語`; all 37
`apps/web/src/locales/ja/*.json` catalogs mirror `en` key-for-key and preserve
placeholders, plural suffixes, `<Trans>` tag structure, code tokens, shortcuts,
and brand names; activation and boot restoration set i18next, `<html lang>`,
and the `kandev_locale` cookie to `ja`; Intl/date-fns formatting uses Japanese
locale data.

## In scope

- Frontend catalog files `apps/web/src/locales/ja/*.json` (37 namespaces,
  ~10,837 keys).
- `apps/web/lib/i18n/index.ts`: add `ja` to `SUPPORTED_LOCALES`, `LOCALE_LABELS`
  (`ja: "日本語"`), and the canonical `SupportedLocale` union; only `pseudo`
  stays production-hidden.
- `apps/web/lib/i18n/date-locale.ts` (or the equivalent `Intl`/`date-fns` locale
  mapping): register Japanese locale data so `formatRelative`/`formatDate`/
  `formatNumber` use it.
- `src/locales/ja/_verbatim.json` (only if a value must stay English verbatim
  with a reason).
- Unit test updates: `lib/i18n/index.test.ts` (SUPPORTED_LOCALES /
  selectableLocales), `boot.test.ts`, `formats.test.ts` / `date-locale.test.ts`
  (Japanese relative time e.g. `5分前`), `bundling.test.ts` (lazy chunk), and
  `scripts/check-i18n-keys.test.ts` if locale discovery behavior is asserted.

## Exclusions

- Backend (`apps/backend/internal/i18n`) — Task 02.
- E2E specs — Task 03.
- Repo-wide docs/lint config — Task 04.

## Applicable requirements

`REQ-PLATFORM-JAPANESE-LOCALE-001.1`, `.2`, `.3`, `.5`; `AC-PLATFORM-I18N-001.3`
(as extended by this package).

## System design

`docs/specs/platform/system-design/i18n.md` — Catalogs and locale runtime,
Data model, API surface (client boot), Scenarios.

## Implementation acceptance conditions

1. `SUPPORTED_LOCALES` includes `ja`, `LOCALE_LABELS["ja"] === "日本語"`, and
   `selectableLocales(false)`/`selectableLocales(true)` reflect `ja`.
2. Selecting `ja` sets `<html lang="ja">` and writes `kandev_locale=ja`; a cold
   boot with that cookie activates `ja` before first paint (lazy chunk loaded).
3. `pnpm run i18n:check` is green: no missing keys, `pseudo` synced, and `ja`
   has exactly the same namespaces/keys as `en` (structurally) with no
   unexplained identical-to-English values (any intentional English-verbatim
   value declared in `src/locales/ja/_verbatim.json`).

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/i18n/index.test.ts lib/i18n/boot.test.ts lib/i18n/formats.test.ts lib/i18n/date-locale.test.ts)
(cd apps/web && pnpm exec vitest run lib/i18n/bundling.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run lint)
```

Run one focused behavioral test in RED before the production/catalog edit, then
re-run the exact focused suite after the final relevant edit. Record RED/GREEN
evidence, key counts by namespace at each commit, and translation-preservation
checks in Results.

## Files likely touched

- `apps/web/src/locales/ja/<37 namespaces>.json`
- `apps/web/src/locales/ja/_verbatim.json` (conditional)
- `apps/web/lib/i18n/index.ts`
- `apps/web/lib/i18n/index.test.ts`
- `apps/web/lib/i18n/boot.test.ts`
- `apps/web/lib/i18n/formats.test.ts`
- `apps/web/lib/i18n/date-locale.ts` and `date-locale.test.ts`
- `apps/web/lib/i18n/bundling.test.ts`

## Dependencies

None.

## Parallelism

Sequential. Catalogs and locale registration form one runtime contract. Commit
in three reviewable stages: (a) common+settings + registration + tests,
(b) high-traffic namespaces, (c) remaining namespaces to full parity.

## Inputs

- Spec: **What**, **Scenarios**, **Failure modes**, **Resolved decisions**.
- Plan: **Wave 1**.
- Existing patterns: `en` / `pseudo` / `zh-cn` catalogs, locale activation, boot
  resolution, current Intl wrappers and their tests.

## Output contract

Report RED/GREEN evidence, key counts by namespace, files changed, exact
commands and results, translation-preservation checks, `ja` verbatim registry
entries, blockers/risks, and synchronized task/plan status.

## Results

- Catalogs: 37 namespaces, 10,837/10,837 keys, 0 missing, 0 placeholder/Trans mismatches.
  Written to `apps/web/src/locales/ja/*.json`. No `ja/_verbatim.json` (none needed).
- Translation: OmniRoute `github/gpt-5.4-mini`, `stream:false`, 40-key batches,
  4 workers. Residual 131 identical-to-English copy keys retranslated one-by-one;
  6 leftovers patched by hand (`Webhook の URL`, `{{value}} 個の CPU`, `アプリ ID`,
  `課題 IID`, `- Git：{{git}}`, `GitHub の Issue #{{number}}`).
- Runtime: `SUPPORTED_LOCALES` already included `ja` / `日本語`. Added `ja-JP` → `ja`
  collapse in `normalizeLocale`/`isSupportedLocale` (matches Go `normalizeRegion`).
  `date-locale.ts` maps `ja` to `date-fns/locale/ja`.
- Gates:
  - `node scripts/check-i18n-keys.mjs`: `ja, pt-pt, zh-cn, zh-hk, zh-tw complete`
  - `node scripts/check-trans-indices.mjs` / inline plurals / module-scope t() /
    em-dash: OK
  - `node scripts/check-nonjsx-copy.mjs`: OK
  - `pnpm run i18n:parity`: ja missing=0 every namespace
- Tests: `pnpm exec vitest run lib/i18n/index.test.ts lib/i18n/boot.test.ts
  lib/i18n/formats.test.ts lib/i18n/date-locale.test.ts lib/i18n/bundling.test.ts`
  → 5 files, 82 tests, GREEN. `formatRelative(ja)` pins `5分前`.
- Later: `pnpm exec vitest run lib/i18n` → 9 files, 100 tests, GREEN.
- Status: catalogs + frontend runtime done.
