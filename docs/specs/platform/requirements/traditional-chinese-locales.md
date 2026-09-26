---
status: active
system: platform
created: 2026-08-12
updated: 2026-08-12
owners:
  - chen
---
# Traditional Chinese locales (Taiwan and Hong Kong) Requirements

## Overview

Kandev already ships Simplified Chinese (`zh-cn`) for Chinese-speaking users, but Taiwan and Hong Kong users read Traditional characters and expect local software vocabulary (for example 軟體 vs 軟件, 設定 vs 設置, 登入 vs 登录). Without dedicated catalogs they either stay on Simplified Chinese or fall back to English. Adding `zh-tw` and `zh-hk` makes the already-localized surface usable for those regions while keeping English as the source locale and `zh-cn` as the conversion base for Chinese copy.

## Requirements

### REQ-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001: Traditional Chinese locales (Taiwan and Hong Kong)

**Intent:** Kandev already ships Simplified Chinese (`zh-cn`) for Chinese-speaking users, but Taiwan and Hong Kong users read Traditional characters and expect local software vocabulary (for example 軟體 vs 軟件, 設定 vs 設置, 登入 vs 登录). Without dedicated catalogs they either stay on Simplified Chinese or fall back to English. Adding `zh-tw` and `zh-hk` makes the already-localized surface usable for those regions while keeping English as the source locale and `zh-cn` as the conversion base for Chinese copy.

#### Acceptance criteria

- **AC-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001.1:** The shipped human locales SHALL include `zh-tw` (Traditional Chinese, Taiwan) and `zh-hk` (Traditional Chinese, Hong Kong), in addition to the existing `en`, `pt-pt`, and `zh-cn`. Locale ids stay BCP-47-style lowercase tags consistent with `zh-cn` / `pt-pt`.
- **AC-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001.2:** The Settings language switcher SHALL list both new locales by fixed endonyms:
- **AC-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001.3:** `zh-tw` → `繁體中文（台灣）`
- **AC-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001.4:** `zh-hk` → `繁體中文（香港）` Selecting either re-renders the UI without a full reload, persists via the existing `kandev_locale` cookie, and sets `<html lang>` to the active tag.
- **AC-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001.5:** Frontend catalogs SHALL live at `apps/web/src/locales/zh-tw/*.json` and `apps/web/src/locales/zh-hk/*.json`, with the same namespaces and key set as `en` (parity remains advisory for real locales, same as `zh-cn` / `pt-pt`).
- **AC-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001.6:** Backend browser-facing catalogs SHALL add `apps/backend/internal/i18n/locales/zh-tw.json` and `zh-hk.json`, and both tags SHALL be accepted by `Supported` / `Normalize` / `FromRequest` (cookie and `Accept-Language`).
- **AC-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001.7:** Chinese Traditional catalogs SHALL be **derived from `zh-cn`**, not re-translated from English. Pipeline: 1. Character/phrase conversion (OpenCC-class `s2twp` for Taiwan, `s2hk` for Hong Kong). 2. Product glossary overrides for UI terms that conversion alone gets wrong or that must differ between Taiwan and Hong Kong (see plan glossary). 3. Human review for high-traffic namespaces before shipping.
- **AC-PLATFORM-TRADITIONAL-CHINESE-LOCALES-001.8:** Region vocabulary SHALL differ where product terms diverge. At minimum the glossary covers: software/network/data/file/folder/settings/default/login/ save/search/load/session/repository/project/user/server/PR wording, and any other pairs recorded in the plan glossary. Brand nouns (Kandev, GitHub, Jira, …) stay untranslated.

## Migrated source detail

## Why

Kandev already ships Simplified Chinese (`zh-cn`) for Chinese-speaking users,
but Taiwan and Hong Kong users read Traditional characters and expect local
software vocabulary (for example 軟體 vs 軟件, 設定 vs 設置, 登入 vs 登录).
Without dedicated catalogs they either stay on Simplified Chinese or fall back
to English. Adding `zh-tw` and `zh-hk` makes the already-localized surface usable
for those regions while keeping English as the source locale and `zh-cn` as the
conversion base for Chinese copy.

## What

- The shipped human locales SHALL include `zh-tw` (Traditional Chinese, Taiwan)
  and `zh-hk` (Traditional Chinese, Hong Kong), in addition to the existing
  `en`, `pt-pt`, and `zh-cn`. Locale ids stay BCP-47-style lowercase tags
  consistent with `zh-cn` / `pt-pt`.
- The Settings language switcher SHALL list both new locales by fixed endonyms:
  - `zh-tw` → `繁體中文（台灣）`
  - `zh-hk` → `繁體中文（香港）`
  Selecting either re-renders the UI without a full reload, persists via the
  existing `kandev_locale` cookie, and sets `<html lang>` to the active tag.
- Frontend catalogs SHALL live at
  `apps/web/src/locales/zh-tw/*.json` and
  `apps/web/src/locales/zh-hk/*.json`, with the same namespaces and key set as
  `en` (parity remains advisory for real locales, same as `zh-cn` / `pt-pt`).
- Backend browser-facing catalogs SHALL add
  `apps/backend/internal/i18n/locales/zh-tw.json` and
  `zh-hk.json`, and both tags SHALL be accepted by `Supported` / `Normalize` /
  `FromRequest` (cookie and `Accept-Language`).
- Chinese Traditional catalogs SHALL be **derived from `zh-cn`**, not
  re-translated from English. Pipeline:
  1. Character/phrase conversion (OpenCC-class `s2twp` for Taiwan,
     `s2hk` for Hong Kong).
  2. Product glossary overrides for UI terms that conversion alone gets wrong
     or that must differ between Taiwan and Hong Kong (see plan glossary).
  3. Human review for high-traffic namespaces before shipping.
- Region vocabulary SHALL differ where product terms diverge. At minimum the
  glossary covers: software/network/data/file/folder/settings/default/login/
  save/search/load/session/repository/project/user/server/PR wording, and any
  other pairs recorded in the plan glossary. Brand nouns (Kandev, GitHub, Jira,
  …) stay untranslated.
- Date, time, relative-time, and number formatting SHALL use locale-aware
  formatters with `zh-tw` → date-fns `zhTW` / Intl `zh-TW`, and `zh-hk` →
  date-fns `zhHK` / Intl `zh-HK`.
- Missing keys in `zh-tw` / `zh-hk` SHALL fall back to English (`en`), not to
  `zh-cn`. Showing Simplified fallback under a Traditional language label is
  worse UX than English fallback (same contract as other real locales).
- Real-locale parity for the new catalogs SHALL remain **advisory**
  (`i18n:check` / `i18n:parity` warn, do not fail). `en` ↔ `pseudo` stays the
  hard gate.
- Docs that enumerate shipped locales (`docs/i18n.md`,
  `docs/specs/platform/requirements/i18n.md`) SHALL be updated when the locales ship.

## Data model

No new persistent entities. Extends existing locale preference:

| Field | Store | Values after this feature |
|---|---|---|
| `kandev_locale` cookie | browser cookie | `en` \| `pt-pt` \| `zh-cn` \| `zh-tw` \| `zh-hk` \| `pseudo` (dev/e2e only) |

Catalogs remain checked-in JSON under the existing layout.

## API surface

No new HTTP/WS endpoints. Contract extensions:

- Frontend: `SUPPORTED_LOCALES` and `LOCALE_LABELS` in
  `apps/web/lib/i18n/index.ts` gain `zh-tw` and `zh-hk`. Lazy catalog loading
  continues via the existing Vite glob over `src/locales/*/*.json`.
- Frontend date locale map in `apps/web/lib/i18n/date-locale.ts` gains loaders
  for `zh-tw` and `zh-hk`.
- Backend: `supportedLocales` in `apps/backend/internal/i18n/i18n.go` gains both
  tags; embedded `locales/zh-tw.json` and `locales/zh-hk.json` supply
  server-rendered browser strings.

## Scenarios

- **GIVEN** Settings → language switcher in a production build, **WHEN** the
  user opens the list, **THEN** it includes English, Português (Portugal),
  简体中文, 繁體中文（台灣）, and 繁體中文（香港）, and does not include Pseudo.
- **GIVEN** the user selects 繁體中文（台灣）, **WHEN** the UI re-renders,
  **THEN** messages present in `zh-tw` resolve from that catalog, missing keys
  fall back to `en` (not `zh-cn`), `<html lang>` is `zh-tw`, and
  `kandev_locale=zh-tw` is written.
- **GIVEN** the user selects 繁體中文（香港）, **WHEN** the UI re-renders,
  **THEN** the same contract holds with `zh-hk` / `kandev_locale=zh-hk`.
- **GIVEN** no locale cookie and `Accept-Language: zh-TW` (or `zh-HK`),
  **WHEN** the Go shell resolves locale, **THEN** it selects `zh-tw` (or
  `zh-hk`), emits the matching `<html lang>`, and uses that backend catalog for
  shell error pages.
- **GIVEN** `zh-tw` is active, **WHEN** number/date helpers format a value,
  **THEN** they use `zh-TW` Intl / date-fns `zhTW` data; **AND GIVEN** `zh-hk`
  is active, **THEN** they use `zh-HK` / `zhHK`.
- **GIVEN** a product glossary entry that maps 简体「设置」→ 台灣「設定」and
  香港「設定」, **WHEN** catalogs are generated/reviewed, **THEN** both
  Traditional catalogs use the region form rather than a bare character
  conversion that leaves 設置 where the glossary requires 設定.
- **GIVEN** a Taiwan vs Hong Kong diverging term (for example 軟體 vs 軟件,
  網路 vs 網絡), **WHEN** the same English key is rendered under each locale,
  **THEN** the visible strings differ according to the glossary.
- **GIVEN** brand nouns and type-to-confirm tokens, **WHEN** Traditional
  catalogs are produced, **THEN** brands stay Latin and confirm tokens that are
  compared with `===` remain untranslated English.
- **GIVEN** `i18n:check` after the locales land, **WHEN** `zh-tw` or `zh-hk`
  lags `en` keys, **THEN** the command warns and still exits 0; **AND WHEN**
  `pseudo` drifts from `en`, **THEN** it still fails.

## Out of scope

- Re-translating Chinese catalogs from English instead of converting from
  `zh-cn`.
- Shipping other Chinese variants (`zh-mo`, `zh-sg`, generic `zh-hant` without
  region).
- Forcing hard CI failure on real-locale parity for the new catalogs.
- Translating agent output, API diagnostic strings, task titles, diffs, or
  other non-UI domain data.
- Changing the English source catalogs or the pseudo generation pipeline beyond
  registering the new locales where discovery already covers them.
- Full professional L10n vendor review of every string in the first ship (first
  ship is conversion + glossary + review of high-traffic namespaces; remaining
  strings may be refined later).

## Open questions

- None material for planning. Defaults locked for implementation:
  - Tags: `zh-tw`, `zh-hk`.
  - Conversion base: `zh-cn`.
  - Fallback: `en` only (not via `zh-cn`).
  - Tooling: OpenCC-class conversion + checked-in product glossary overrides.
