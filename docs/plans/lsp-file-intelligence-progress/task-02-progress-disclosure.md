---
id: "02-progress-disclosure"
title: "Responsive progress disclosure"
status: completed
wave: 2
depends_on: ["01-progress-protocol"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 02: Responsive Progress Disclosure

## Acceptance

- The toolbar control opens details instead of directly stopping a live server and presents exactly one appropriate Start, Stop, or Retry action.
- Active server work shows title, optional message and percentage, locale-aware elapsed time, concurrent count, and incomplete-cross-file guidance; no-report and completed states make no project-wide claim.
- Fine-pointer layouts use a popover while coarse-pointer Monaco layouts use a viewport-contained drawer with touch-sized controls and shared state.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run lib/lsp components/editors/monaco hooks/use-lsp.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/editors hooks/use-lsp.ts lib/lsp
```

## Files Likely Touched

- `apps/web/hooks/use-lsp.ts`
- `apps/web/components/editors/lsp-status-button.tsx`
- `apps/web/components/editors/lsp-progress-details.tsx`
- `apps/web/components/editors/monaco/monaco-code-editor.tsx`
- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx`
- `apps/web/components/editors/monaco/use-monaco-editor-lsp.ts`

## Dependencies

Task 01.

## Parallelism

Sequential; Task 03 targets this rendered contract.

## Inputs

- Spec toolbar and readiness scenarios.
- `PRStatusChipDrawer` responsive exemplar.
- Mobile design contract in `plan.md`.

## Output Contract

Record rendered behavior, accessibility and geometry decisions, files changed, exact checks, and update this task plus `plan.md`.

## Result

- RED: presentation-helper tests proved elapsed, lifecycle, active, no-report, and completion copy were absent; hook tests also reproduced the mounted-editor Retry failure.
- GREEN: one snapshot now drives a fine-pointer popover or coarse-pointer drawer, with separate connection/project state, determinate-only percentages, tabular elapsed time, honest completion copy, and explicit lifecycle actions.
- Review hardening: hour, minute, and second units plus multi-unit composition now come from the active locale catalog, with pseudo-locale coverage.
- Review hardening: the boot runtime's task-host-independent preference list controls auto-install checkboxes, agentctl retains platform authority, and the LSP/settings catalogs now retain full Simplified Chinese parity.
- Review hardening: an explicit Stop now records a runtime override for the session/language, preventing global auto-start from reacquiring the lease after a matching editor or configuration rerender; explicit Start clears it. Hook and desktop E2E coverage exercise both halves.
- Verified:
  - focused LSP/editor Vitest suite (75 tests)
  - `pnpm exec vitest run hooks/use-lsp.test.tsx` (6 passed, including auto-start Stop/configuration/explicit-Start policy)
  - `pnpm e2e:run tests/lsp/lsp-file-intelligence.spec.ts -- --grep "auto-starts one shared server"` (1 production-build Chromium scenario passed)
  - `pnpm run typecheck`
  - full web ESLint with zero warnings
