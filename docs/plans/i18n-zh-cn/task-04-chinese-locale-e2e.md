---
id: "04-chinese-locale-e2e"
title: "Chinese locale E2E"
status: done
wave: 3
depends_on:
  - "01-frontend-locale-catalogs"
  - "02-backend-locale-negotiation"
  - "03-catalog-parity-gate"
plan: "plan.md"
spec: "../../specs/platform/requirements/i18n.md"
---

# Task 04: Chinese locale E2E

## Acceptance

- Playwright proves selecting 简体中文 changes stable migrated copy and
  `<html lang>`, survives reload through the locale cookie, and restores English.
- The `mobile-chrome` project proves the same selection, translated copy,
  cookie persistence, reload, and English restoration on the supplied Pixel 5
  viewport without a per-test viewport override.
- The existing pseudo-locale scenario remains intact and passing.
- A fresh desktop Chinese Appearance screenshot is captured, inspected for
  secrets and layout problems, stored under ignored `apps/web/.pr-assets/`, and
  excluded from the feature branch.

## Verification

```bash
(cd apps/web && pnpm e2e:run --host --project chromium -- tests/i18n/language-switch.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome -- tests/i18n/mobile-language-switch.spec.ts)
```

Add the Chinese scenario first and observe its expected RED result before the
feature is complete, then rerun the focused managed headless spec after the final
relevant edit.

## Files likely touched

- `apps/web/e2e/tests/i18n/language-switch.spec.ts`
- `apps/web/e2e/tests/i18n/mobile-language-switch.spec.ts`
- Ignored screenshot assets under `apps/web/.pr-assets/` only; never commit them
  to the feature branch.

## Dependencies

Tasks 01-03 must be done so the E2E runs against the integrated runtime,
backend, and catalogs.

## Parallelism

Sequential. This task is the browser-level integration proof.

## Inputs

- Spec scenarios for the Chinese switch, reload, shell lang, and pseudo QA.
- Plan: **E2E Tests**.
- Existing language switch spec and repository `e2e` workflow.

## Output contract

Report RED/GREEN evidence, discovered test count, exact command and result,
cookie/lang/text assertions, screenshot path and redaction/layout review,
files changed, blockers/risks, and synchronized task/plan status.

## Results

- Desktop `language-switch.spec.ts` passed 3/3, including the existing pseudo
  scenario and the Simplified Chinese selection, canonical `lang="zh-cn"`,
  stable `显示语言` copy, `kandev_locale=zh-cn`, reload persistence, and
  restoration to English.
- Mobile `mobile-language-switch.spec.ts` passed 1/1 under the
  `mobile-chrome` Pixel 5 project. It scopes options through the active listbox,
  checks the Chinese label and cookie after selection and reload, then restores
  English to prevent cookie leakage.
- Captured
  `apps/web/.pr-assets/language-switch--simplified-chinese-locale-desktop.png`.
  Manual review found no secrets, raw keys, broken interpolation, or overflow.
  English copy on unmigrated sidebar surfaces is the documented migration
  boundary. The screenshot remains ignored and is excluded from the branch.
