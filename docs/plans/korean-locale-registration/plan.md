---
created: 2026-09-30
status: implemented
requirements:
  - REQ-PLATFORM-I18N-001
system_design:
  - ../../specs/platform/system-design/i18n.md
legacy_specs: []
---

# Implementation Plan: Register Korean Locale

## Overview

Register `ko` in backend and web locale negotiation so users can select Korean and retain that selection across reloads. This is the first delivery stage. A later PR will add the 37-namespace Korean UI catalog and its full key parity; this stage uses the existing English fallback for UI copy.

## Scope

- Add `ko` to supported locale lists, the fixed `한국어` label, date formatting, and the backend catalog.
- Keep the Settings selection, cookie, and `<html lang>` in sync.
- Verify the existing English fallback while the Korean UI namespace catalog is absent.
- Leave the translated UI catalog, its parity work, and the complete human-catalog list in AC-PLATFORM-I18N-001.3 to the next stage.

## Technical approach

The backend recognizes `ko` in `internal/i18n` and the Go shell accepts the locale cookie. The web i18n registry and date locale loader accept `ko`. The existing Settings language switcher reads the registry; its desktop and phone presentations keep their existing layout and interaction. No new page or control is introduced.

## UI preview

UI-01: Settings > General > Appearance, selected language (desktop and phone)

```text
Display language  [한국어 v]
```

The existing picker and phone presentation remain in place. Selection persists and sets the document language to `ko`; labels without a Korean UI catalog render from English. This preview illustrates the affected row, not a new layout.

## Verification

- Go locale and shell tests cover recognition, cookie negotiation, and `<html lang>`.
- Web i18n and date locale tests cover registration, formatting, and English fallback.
- The existing `language-switch.spec.ts` flow selects `한국어`, checks `lang=ko`, English fallback copy, persistence after reload, and restoration to English. Its synthetic screenshot records the selected state.
- Focused formatting, typecheck, lint, i18n, unit, and E2E checks were run for the registration branch; their results are recorded in the PR.

## Work orders

- [Task 01: Register the Korean locale](task-01-register-ko-locale.md) (done)
