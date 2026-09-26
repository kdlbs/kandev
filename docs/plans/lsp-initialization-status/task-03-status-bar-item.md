---
id: "03-status-bar-item"
title: "Active-editor status-bar item"
status: completed
wave: 3
depends_on: ["01-initialization-stage", "02-placement-setting"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 03: Active-Editor Status-Bar Item

## Acceptance

- On a fine-pointer layout with the application bar enabled and `status_bar` saved, `builtin:lsp` replaces the toolbar trigger and summarizes only the active supported Monaco file's LSP.
- Clicking the item opens the same details and lifecycle actions; it participates in existing opaque status-item ordering.
- Loading, static/binary, diff, non-file, and unsupported active panels hide the item, while feature-disabled and coarse-pointer layouts retain the toolbar without rewriting the preference or starting a phone LSP lease.

## TDD sequence

1. Add failing focused tests for built-in identity/order and active-file summary visibility.
2. Add the status item and reusable status trigger/details with the smallest active-editor subscription.
3. Add toolbar fallback tests and refactor shared presentation without duplicating lifecycle logic.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/app-status-bar components/editors lib/lsp/lsp-status-placement.test.ts hooks/use-lsp.test.tsx
cd apps/web && pnpm run typecheck
cd apps/web && pnpm exec eslint components/app-status-bar components/editors hooks/use-lsp.ts lib/lsp
```

## Files likely touched

- `apps/web/components/app-status-bar/app-status-bar-order.ts`
- `apps/web/components/app-status-bar/app-status-items.tsx`
- `apps/web/components/app-status-bar/lsp-status-item.tsx`
- `apps/web/components/app-status-bar/app-status-bar.test.tsx`
- `apps/web/components/app-status-bar/app-status-drawer.test.tsx`
- `apps/web/components/editors/lsp-status-button.tsx`
- `apps/web/components/editors/monaco/monaco-editor-toolbar.tsx`
- `apps/web/lib/lsp/lsp-progress-view.ts`
- `apps/web/lib/lsp/lsp-status-placement.ts`
- `apps/web/hooks/use-lsp.ts`
- `apps/web/lib/state/dockview-store.ts` only if the existing active file identity is insufficient

## Dependencies

Tasks 01 and 02.

## Parallelism

Sequential. The item consumes both the presentation and persistent placement contracts.

## Inputs

- Spec active-file and responsive fallback scenarios.
- `ConnectionStatusItem` and `StatusSurfaceMetrics` as built-in item patterns.
- Existing `activeFilePath`/`activeFileRepo` Dockview tracking.

## Output contract

Record RED/GREEN evidence, lease/active-editor ownership, responsive behavior, files changed, exact commands, and update this task plus `plan.md`.

## Result

- RED: focused tests failed on the missing `builtin:lsp` identity, active-file resolver, compact summary, status item, toolbar suppression, and status-only lifecycle subscriber.
- GREEN: 21 focused app-status/editor test files pass 87 tests; the full web typecheck and focused ESLint pass without warnings.
- `builtin:lsp` is an opaque, reorderable right-side item derived only from the active session and supported active Monaco file. Unsupported and non-file panels remove it.
- The item subscribes to the existing `(session, language)` connection without acquiring another lease. Start, Stop, and Retry use a shared request generation consumed by mounted editor leases.
- Fine-pointer `status_bar` placement suppresses the Monaco toolbar trigger. Feature-disabled and coarse-pointer conditions resolve back to the toolbar without changing the saved preference.
- The phone application-status drawer does not construct the LSP item or call the status hook, so it cannot start an invisible lease.
- Review follow-up: the item now also requires an active `file-editor` panel with a loaded text buffer. A supported extension rendered by `StaticFilePanel` can no longer expose Start/Retry without a mounted Monaco `useLsp` lease.
- RED/GREEN follow-up: the resolver failed the new mounted-editor contract; 34 focused status/store tests now cover loading, Monaco text, binary/static, diff, unsupported, CodeMirror, and responsive drawer boundaries.
- Codex restore follow-up: a session-restored tab placeholder now remains LSP-ineligible until its fetched content receives an explicit text classification. The integration regression failed while the placeholder's missing `isBinary` value was treated as text, then passed when status ownership required `isBinary === false`.
- CI follow-up: text-file responses omit the false-valued `is_binary` JSON field. The WebSocket file-content boundary now normalizes loaded text to `false` while restored placeholders remain unclassified; focused unit/integration tests pass 23 tests, web typecheck and focused ESLint pass, and the production-build Playwright status-placement scenario passes.
