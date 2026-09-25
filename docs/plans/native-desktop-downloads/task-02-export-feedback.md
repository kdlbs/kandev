---
id: "02-export-feedback"
title: "Align export feedback and Blob lifetime"
status: blocked
wave: 2
depends_on:
  - "01-desktop-save-handler"
plan: "plan.md"
requirements:
  - REQ-DESKTOP-NATIVE-DOWNLOADS-001
acceptance_criteria:
  - AC-DESKTOP-NATIVE-DOWNLOADS-001.2
  - AC-DESKTOP-NATIVE-DOWNLOADS-001.4
  - AC-DESKTOP-NATIVE-DOWNLOADS-001.5
system_design:
  - ../../specs/desktop/system-design/native-downloads.md
---

# Task 02: Align export feedback and Blob lifetime

## Summary

Keep generated object URLs alive through native completion, and make the
System Logs export report the real desktop save result. Browser and phone
downloads remain usable.

## In scope

- Correct direct Blob export callers with immediate URL revocation.
- Connect desktop save results to the log-bundle status and retry state.
- Add focused browser and phone download coverage for the affected paths.

## Out of scope

- Changing ZIP, SVG, JSON, or task-file contents.
- Changing desktop updater behavior.

## Acceptance

1. System Logs does not claim a download or saved file before desktop
   completion; cancel is neutral and failure is visible and retryable.
2. Every audited first-party object-URL export survives native download
   initiation and retains browser download behavior.
3. Existing phone controls and browser download flows remain accessible.

## ASCII UI preview

`UI-01: Desktop export` and `UI-02: Phone and browser` from the
[plan](plan.md#ascii-ui-preview):

```text
Desktop: Export -> Save dialog -> Saved / visible failure
Phone:   Export -> existing browser-managed download
```

The phone's existing customizer drawer remains the entry point. AC-001.2,
AC-001.4, and AC-001.5 define the outcomes.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/settings/system/log-viewer.test.tsx lib/utils/file-download.test.ts lib/desktop/download-feedback.test.ts)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run tests/system/logs-page.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/system/mobile-logs-bundle.spec.ts)
(cd apps/web && pnpm run typecheck)
```

The desktop startup smoke does not exercise native downloads. Record a
packaged macOS run with Save/Cancel and HTTP/Blob byte hashes before marking
AC-001.2 complete.

## Files likely touched

- `apps/web/components/settings/system/log-viewer.tsx`
- `apps/web/components/settings/system/log-viewer.test.tsx`
- `apps/web/lib/utils/file-download.ts`
- `apps/web/lib/utils/file-download.test.ts`
- `apps/web/app/office/workspace/settings/export/export-preview.tsx`
- `apps/web/app/office/workspace/org/org-chart-canvas.tsx`
- `apps/web/app/office/agents/[id]/components/agent-memory-tab.tsx`
- `apps/web/e2e/tests/system/mobile-logs-bundle.spec.ts`
- `apps/web/e2e/tests/system/logs-page.spec.ts`
- `apps/web/e2e/tests/system/backups-page.spec.ts`
- `apps/web/src/locales/*` if new status copy is needed
- `docs/public/desktop-app.md`

## Dependencies

Task 01: the native result contract must exist before the UI consumes it.

## Risks

Tests must check a real browser download and native transfer, not merely that
an anchor was clicked. Avoid leaking object URLs after failed attempts.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/desktop/requirements/native-downloads.md)
- [System design](../../specs/desktop/system-design/native-downloads.md)
- Existing browser and mobile Playwright download tests

## Results

- `pnpm exec vitest run components/settings/system/log-viewer.test.tsx lib/utils/file-download.test.ts lib/desktop/download-feedback.test.ts`: 17 tests passed. Coverage includes delayed native Blob initiation, terminal-event cleanup, bounded cleanup, missing Logs results, listener failure, and an unrelated export using the diagnostic filename.
- Targeted ESLint and `pnpm run typecheck` passed.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet` passed.
- `pnpm e2e:run tests/system/logs-page.spec.ts`: all 3 browser tests passed.
- `pnpm e2e:run --project mobile-chrome tests/system/mobile-logs-bundle.spec.ts`: all 3 mobile tests passed.
- `node --test scripts/validate-public-docs.test.mjs`: all 62 tests passed; the public-doc validator accepted all 47 pages.
- `python3 scripts/list-docs.py validate` and `python3 scripts/lint-spec-files.py --all` passed.
- The Linux desktop package built and its startup smoke passed. It does not
  automate the native Save dialog or verify downloaded bytes.
- Packaged macOS Save/Cancel and byte-preservation evidence remains required
  before AC-001.2 is verified. This Linux host cannot run that WKWebView check.
- Status remains blocked on the same native transfer gate as Task 01. The
  frontend and browser/mobile checks are complete; AC-001.2 is unverified.
