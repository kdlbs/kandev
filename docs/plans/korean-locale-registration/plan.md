---
created: 2026-09-30
status: in_progress
requirements:
  - REQ-PLATFORM-I18N-001
system_design:
  - ../../specs/platform/system-design/i18n.md
legacy_specs: []
---

# Implementation Plan: Korean Locale

## Overview

Register `ko` in backend and web locale negotiation so users can select Korean and retain that selection across reloads, then ship the Korean UI catalog. Delivery has two stages. Task 01 registered the locale and relied on the English fallback for UI copy. Task 02 adds the 37-namespace Korean UI catalog with full key parity, so `ko` becomes a real locale gated by `i18n:check`.

## Scope

- Add `ko` to supported locale lists, the fixed `한국어` label, date formatting, and the backend catalog.
- Keep the Settings selection, cookie, and `<html lang>` in sync.
- Task 01 verified the existing English fallback while the Korean UI namespace catalog was absent.
- Task 02 adds the translated UI catalog, reconciles it with the current `en` keys, and adds `ko` to the complete human-catalog list in AC-PLATFORM-I18N-001.3.

## Technical approach

The backend recognizes `ko` in `internal/i18n` and the Go shell accepts the locale cookie. The web i18n registry and date locale loader accept `ko`. The existing Settings language switcher reads the registry; its desktop and phone presentations keep their existing layout and interaction. No new page or control is introduced. The Korean catalog is discovered by the existing locale glob and loaded as a lazy chunk like every other real locale, so it needs no runtime code change.

## UI preview

UI-01: Settings > General > Appearance, selected language (desktop and phone)

```text
표시 언어  [한국어 v]
```

The existing picker and phone presentation remain in place. Selection persists and sets the document language to `ko`. With Task 02 the row label and the rest of the UI render from the Korean catalog; under Task 01 they rendered from English. This preview illustrates the affected row, not a new layout.

## Verification

- Go locale and shell tests cover recognition, cookie negotiation, and `<html lang>`.
- Web i18n and date locale tests cover registration, Korean catalog activation, and catalog-backed relative time.
- The existing `language-switch.spec.ts` flow selects `한국어`, checks `lang=ko`, the Korean `표시 언어` label, persistence after reload, and restoration to English. Its synthetic screenshot records the selected state.
- `i18n:check` gates Korean catalog parity: namespaces, keys, placeholders, `<Trans>` tags, and undeclared English values.
- Each task records its own check results in its PR.

## Work orders

- [Task 01: Register the Korean locale](task-01-register-ko-locale.md) (done)
- [Task 02: Ship the Korean UI translation catalog](task-02-korean-ui-catalog.md) (in progress)
