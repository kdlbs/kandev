---
id: "15-in-editor-preview-docs-e2e"
title: "Revise preview guidance and browser E2E"
status: done
wave: 6
depends_on:
  - "14-in-editor-html-preview"
plan: "plan.md"
requirements:
  - REQ-UI-NATIVE-HTML-PREVIEW-001
acceptance_criteria:
  - AC-UI-NATIVE-HTML-PREVIEW-001.1
  - AC-UI-NATIVE-HTML-PREVIEW-001.2
  - AC-UI-NATIVE-HTML-PREVIEW-001.3
  - AC-UI-NATIVE-HTML-PREVIEW-001.4
  - AC-UI-NATIVE-HTML-PREVIEW-001.7
  - AC-UI-NATIVE-HTML-PREVIEW-001.8
  - AC-UI-NATIVE-HTML-PREVIEW-001.10
system_design:
  - ../../specs/ui/system-design/native-html-preview.md
---

# Task 15: Revise preview guidance and browser E2E

## Summary

Update public guidance and browser tests so they describe and prove the
in-editor source/preview workflow while retaining native browser fidelity and
the focused mobile composition.

## In scope

- Public documentation for in-editor preview, source recovery, refresh, and the
  optional Browser-panel action.
- Desktop Chromium E2E proving no automatic Browser panel, iframe replacement,
  unsaved republish, native scripts and assets, `Show code`, and dirty state.
- Mobile Chrome regression coverage for the shared focused iframe behavior.
- Final specification, documentation, frontend, E2E, and desktop verification.

## Out of scope

- Changes to the trusted-code boundary or server lifecycle.
- New executor-matrix E2E beyond existing port-proxy coverage.

## Acceptance

- Documentation describes the in-editor workflow without claiming automatic
  Browser-panel behavior.
- Desktop and mobile tests prove the same current-buffer browser value with
  viewport-appropriate controls and source recovery.
- Task-defined validation passes without retries or flakes.

## Verification

```bash
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
cd apps/web
pnpm e2e:run tests/chat/html-preview.spec.ts
pnpm e2e:run --project mobile-chrome tests/task/mobile-html-preview.spec.ts
cd ../desktop
pnpm e2e
```

## Files likely touched

- `docs/public/developer-tools.md`
- `apps/web/e2e/tests/chat/html-preview.spec.ts`
- `apps/web/e2e/tests/task/mobile-html-preview.spec.ts`
- Focused E2E page objects or helpers.

## Dependencies

Task 14 supplies the revised in-editor implementation.

## Risks

- An E2E test can accidentally assert the old Browser-panel iframe.
- Production-build E2E can exercise stale assets unless the managed runner
  rebuilds after frontend changes.

## Parallelism

`sequential`

## Inputs

- All applicable acceptance criteria and the revised responsive contract.
- The amended trusted native-browser HTML preview ADR.

## Results

- Public guidance now documents the in-editor desktop and focused mobile
  workflow, explicit Browser-panel access, and the session-scoped server
  lifecycle. Closing any preview view unmounts that view; agentctl teardown
  stops the bounded shared server.
- Desktop Chromium E2E passed 2 of 2 tests on the first attempt. It proves no
  automatic Browser panel, native scripts and browser APIs, relative assets,
  current-buffer refresh, explicit Browser-panel opening, source recovery, and
  retry after publication failure.
- Mobile Chrome E2E passed 2 of 2 tests on the first attempt, preserving the
  focused-viewer interaction and source recovery at phone width.
- Specification validation passed all 30 tests. Public-documentation validation
  passed 61 tests across 46 published pages. Desktop shell E2E passed.
- The full frontend suite passed 1,872 test files and 15,902 tests, with four
  intentional skips and no failures.
