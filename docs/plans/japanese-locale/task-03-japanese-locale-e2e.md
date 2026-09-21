---
id: "03-japanese-locale-e2e"
title: "Japanese locale E2E"
status: done
wave: 3
depends_on:
  - "01-frontend-ja-locale-catalogs"
  - "02-backend-ja-locale-negotiation"
plan: "plan.md"
spec: "../../specs/platform/requirements/japanese-locale.md"
---

# Task 03: Japanese locale E2E

## Outcome

Playwright proves selecting `日本語` changes stable migrated copy and
`<html lang>` to `ja`, survives reload through the `kandev_locale` cookie, and
restores English — in the desktop chromium project and, if the mobile project
convention applies, the mobile project.

## In scope

- `apps/web/e2e/tests/i18n/language-switch.spec.ts`: add a `ja` case
  (option `日本語`, display-language label `表示言語`, `<html lang="ja">`,
  cookie `kandev_locale=ja`, reload restoration, restore English) following the
  `zh-tw`/`zh-hk` loop pattern at the same file location.
- `apps/web/e2e/tests/i18n/mobile-language-switch.spec.ts` if the mobile
  convention covers this family.
- A fresh desktop Japanese Appearance screenshot under ignored
  `apps/web/.pr-assets/` (never committed), inspected for secrets and layout
  problems (Japanese text can truncate narrow labels).

## Exclusions

- Unit-level frontend and backend behavior — Tasks 01 and 02.
- Repository-level docs/lint final pass — Task 04.

## Applicable requirements

`REQ-PLATFORM-JAPANESE-LOCALE-001.2`, `.7`; `AC-PLATFORM-I18N-001.7`.

## System design

`docs/specs/platform/system-design/i18n.md` — Scenarios (switch, cookie,
reload, lang).

## Implementation acceptance conditions

1. Selecting `日本語` renders a known translated label (e.g. `表示言語`),
   sets `<html lang="ja">`, and writes `kandev_locale=ja`.
2. Reload keeps `lang="ja"` and the translated label; switching back to English
   restores `lang="en"` and `Display language`.
3. The existing pseudo-locale scenario stays intact and passing.

## Verification

```bash
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/i18n/language-switch.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/i18n/mobile-language-switch.spec.ts)
```

Add the Japanese scenario first and observe its expected RED result before the
feature is complete, then rerun the focused spec after the final relevant edit.

## Files likely touched

- `apps/web/e2e/tests/i18n/language-switch.spec.ts`
- `apps/web/e2e/tests/i18n/mobile-language-switch.spec.ts`
- Ignored screenshot assets under `apps/web/.pr-assets/` only; never commit them
  to the feature branch.

## Dependencies

Tasks 01-02 must be done so the E2E runs against the integrated runtime,
backend, and catalogs.

## Parallelism

Sequential. This task is the browser-level integration proof.

## Inputs

- Spec scenarios for the Japanese switch, reload, shell lang, and pseudo QA.
- Plan: **Wave 3**.
- Existing `language-switch.spec.ts` / `mobile-language-switch.spec.ts`
  conventions and the `zh-tw`/`zh-hk` loop.

## Output contract

Report RED/GREEN evidence, screenshot paths (under `.pr-assets/`, ignored), any
mobile-project divergence, exact commands and results, blockers/risks, and
synchronized task/plan status.

## Results

- Desktop: `language-switch.spec.ts` loop gained `{ id: "ja", option: "日本語",
  displayLanguage: "表示言語" }`.
- Mobile: `mobile-language-switch.spec.ts` gained Japanese switch/reload/restore.
- Commands:
  - `pnpm e2e:run --host --no-build --project chromium -- tests/i18n/language-switch.spec.ts`
    → 6 passed (39.9s), including 日本語.
  - `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/i18n/mobile-language-switch.spec.ts`
    → 2 passed (25.1s).
- Screenshot (ignored, not committed):
  `apps/web/.pr-assets/language-switch--japanese-locale-desktop.png` (1280x720).
  First E2E run RED was Playwright chromium v1228 missing; repo installer
  `scripts/install-playwright-browsers.sh` then GREEN. No mobile divergence.
