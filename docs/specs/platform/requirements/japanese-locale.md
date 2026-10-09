---
status: active
system: platform
created: 2026-09-20
updated: 2026-09-21
owners:
  - y4tk
---
# Japanese Locale Requirements

## Overview

Kandev ships English, European Portuguese, Simplified Chinese, Traditional Chinese (Taiwan and Hong Kong), and Japanese catalogs. `ja` makes the already-localized web surface usable by Japanese-speaking users while preserving English as the source locale, the JSON catalog contract, the `kandev_locale` cookie persistence, and the real-locale parity gate.

## Requirements

### REQ-PLATFORM-JAPANESE-LOCALE-001: Japanese locale

**Intent:** Kandev ships English, European Portuguese, Simplified Chinese, Traditional Chinese (Taiwan and Hong Kong), and Japanese catalogs. `ja` makes the already-localized web surface usable by Japanese-speaking users while preserving English as the source locale, the JSON catalog contract, the `kandev_locale` cookie persistence, and the real-locale parity gate.

#### Acceptance criteria

- **AC-PLATFORM-JAPANESE-LOCALE-001.1:** The shipped human locales SHALL include `ja` (Japanese) in addition to the existing `en`, `pt-pt`, `zh-cn`, `zh-tw`, and `zh-hk`. Locale id SHALL be the lowercase BCP-47-style tag `ja`, consistent with `pt-pt` / `zh-cn`.
- **AC-PLATFORM-JAPANESE-LOCALE-001.2:** The Settings language switcher SHALL list Japanese by its fixed endonym `日本語`. Selecting it re-renders the UI without a full reload, persists via the existing `kandev_locale` cookie, and sets `<html lang>` to `ja`.
- **AC-PLATFORM-JAPANESE-LOCALE-001.3:** Frontend catalogs SHALL live at `apps/web/src/locales/ja/*.json`, with the same namespaces and key set as `en` (real-locale parity gates, same as `pt-pt` / `zh-cn` / `zh-tw` / `zh-hk`; each key must preserve `{{placeholders}}`, plural `_one`/`_other` suffixes, `<Trans>` tag structure, code tokens, shortcuts, and brand names).
- **AC-PLATFORM-JAPANESE-LOCALE-001.4:** The backend browser-facing catalog SHALL add `apps/backend/internal/i18n/locales/ja.json`, and `ja` SHALL be accepted by `Supported` / `Normalize` / `FromRequest` (cookie and `Accept-Language` negotiation).
- **AC-PLATFORM-JAPANESE-LOCALE-001.5:** Locale-aware date/time/number/relative-time formatting SHALL use Japanese locale data (e.g. relative time reads `5分前`, dates use Japanese ordering), driven by the active locale.
- **AC-PLATFORM-JAPANESE-LOCALE-001.6:** CI, lint, and catalog-parity checks SHALL discover `ja` automatically as a real locale; the identical-to-English tolerance SHALL use the shared `_verbatim.json` registry plus a `src/locales/ja/_verbatim.json` (if needed). A build that adds an `en` key without the `ja` translation SHALL fail, exactly as it does for the other real locales (`pt-pt`, `zh-cn`, `zh-tw`, `zh-hk`).
- **AC-PLATFORM-JAPANESE-LOCALE-001.7:** The Playwright `language-switch.spec.ts` SHALL cover selecting `日本語`, verifying `lang="ja"` and a known translated label, cookie persistence (`kandev_locale=ja`), reload restoration, and restoring English.

## Explicit exclusions

- Translating the CLI launcher, logs, ACP/agent output, or the ~1,100 diagnostic API error strings (these stay English by design; see the i18n spec out-of-scope).
- Translating user/domain content carried in the boot payload and store (task titles, workflow/step names, repo names, chat messages, diff content, agent transcripts).
- RTL layout support; `<html dir>` stays `ltr`.
- Translating integration provider names, brand names, code identifiers, keyboard-shortcut glyphs, and other proper nouns.
- A translation-management platform / vendor sync.
