---
id: "13-browser-fidelity-docs-and-e2e"
title: "Document and prove browser-fidelity preview"
status: done
wave: 4
depends_on:
  - "12-browser-panel-preview-ui"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.5
  - AC-UI-NATIVE-HTML-PREVIEW-001.6
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
  - AC-UI-NATIVE-HTML-PREVIEW-001.9
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
  - AC-UI-NATIVE-HTML-PREVIEW-001.11
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---
# Task 13: Document and prove browser-fidelity preview

## Summary

Update public guidance for the trusted static preview. Replace isolation E2E
assertions with browser-fidelity, relative-asset, lifecycle, and responsive
evidence.

## In scope

- Public documentation for the one-click action, current-buffer behavior,
  relative assets, browser APIs, and trusted-code warning.
- Public documentation for static-server limits, recovery, and the explicit
  development-server alternative.
- Desktop Chromium E2E for native scripts, a browser API absent from the old
  virtual runtime, relative CSS/JavaScript/image assets, unsaved republish,
  Browser-panel reuse, errors, and source preservation.
- Mobile Chrome E2E for the shared proxied page, touch size, containment, retry,
  `Show code`, and unchanged source.
- Desktop build/WebView smoke and final targeted regression verification.

## Out of scope

- Claiming hostile-content isolation.
- Broad executor-matrix E2E beyond existing port-proxy coverage.

## Acceptance

- Documentation accurately distinguishes trusted static preview from configured
  application development servers.
- Desktop and mobile tests causally prove native browser execution, relative
  asset loading, current-buffer refresh, responsive behavior, and recovery.
- Specification, documentation, dependency, backend, frontend, and desktop
  validation commands pass.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
make -C apps/backend test
cd apps/web
pnpm e2e:raw --project=chromium tests/chat/html-preview.spec.ts
pnpm e2e:raw --project=mobile-chrome tests/task/mobile-html-preview.spec.ts
cd ../desktop
pnpm e2e
```

## Files likely touched

- `docs/public/developer-tools.md`
- `apps/web/e2e/tests/chat/html-preview.spec.ts`
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts`
- Focused E2E fixtures and page objects.

## Dependencies

Task 12 supplies the complete user flow and removes the old runtime.

## Risks

- E2E fixtures can accidentally prove saved disk content instead of the current
  unsaved overlay.
- Relative asset assertions can pass through cached data unless request paths and
  version changes are observed.
- Documentation can overstate safety unless the trusted-code language stays
  explicit.

## Parallelism

`sequential`

## Inputs

- All acceptance criteria and the final verification strategy.
- The accepted trusted Browser HTML preview ADR.

## Results

Updated the public developer-tools guidance to document current-buffer static
preview, relative assets, native browser behavior, trusted workspace code,
bounded server behavior, recovery, and the explicit development-server
alternative. Replaced the old isolation-oriented E2E assertions with native
browser fidelity coverage on desktop and mobile.

Verification:

- `python3 scripts/lint-spec-files.py --all` passed.
- `node --test scripts/validate-public-docs.test.mjs` passed: 61 tests.
- `node scripts/validate-public-docs.mjs` validated 43 published pages.
- `make -C apps/backend test` passed with task-host internal config overrides
  cleared.
- Desktop Chromium preview E2E passed: 2 tests.
- Mobile Chrome preview E2E passed: 2 tests.
- Desktop shell E2E passed.
- Fresh desktop and mobile PR screenshots were captured and compressed.
- Reproduced and fixed the frontend CI failure caused by a synchronous
  assertion racing Radix repository-menu dismissal, and removed unhandled
  Happy DOM loopback requests caused by Playwright runtime imports in Vitest
  helper tests.
- Full frontend Vitest verification passed after the fix: 1,870 test files,
  15,851 passed, and 4 skipped, with no unhandled network errors. Direct web
  typecheck, lint, i18n, and formatting checks passed as well.
