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
spec: "../../specs/platform/i18n.md"
---

# Task 04: Chinese locale E2E

## Acceptance

- Playwright proves selecting 简体中文 changes stable migrated copy and
  `<html lang>`, survives reload through the locale cookie, and restores English.
- The existing pseudo-locale scenario remains intact and passing.
- A fresh desktop Chinese Appearance screenshot is captured, inspected for
  secrets and layout problems, and staged only as an ignored PR asset.

## Verification

```bash
cd apps/web && pnpm e2e:run tests/i18n/language-switch.spec.ts
```

Add the Chinese scenario first and observe its expected RED result before the
feature is complete, then rerun the focused managed headless spec after the final
relevant edit.

## Files likely touched

- `apps/web/e2e/tests/i18n/language-switch.spec.ts`
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

- Added a Simplified Chinese scenario covering the `简体中文` option, canonical
  `lang="zh-cn"`, stable `显示语言` copy, `kandev_locale=zh-cn`, reload
  persistence, and restoration to English. The existing English and pseudo
  scenarios remain unchanged and passing.
- The Windows host run exposed an existing fixture cleanup defect: the POSIX
  negative-PID signal path left the backend process tree alive. Added a
  taskkill-based Windows branch with three focused lifecycle tests.
- RED: `pnpm exec vitest run lib/e2e/backend-process.test.ts` failed 3/3 before
  the Windows process-tree implementation existed. GREEN: the same command
  passed 3/3.
- The repository runner built all artifacts but its Windows global setup looked
  for extensionless binaries. After using the built `.exe` through the existing
  `KANDEV_E2E_BIN` override and installed Chrome through a temporary local-only
  config, `pnpm exec playwright test --config
e2e/playwright.local-chrome.config.ts --project=chromium
tests/i18n/language-switch.spec.ts` passed 3/3 in 15.4 seconds. The temporary
  config was removed after the run.
- Captured
  `apps/web/.pr-assets/language-switch--simplified-chinese-locale-desktop.png`.
  Manual review found no secrets, raw keys, broken layout, or overflow. English
  copy on unmigrated sidebar surfaces is the documented migration boundary.
