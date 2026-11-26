---
id: "03-theme-rendering"
title: "Generic application and renderer pipeline"
status: done
wave: 3
depends_on:
  - "02-theme-lifecycle"
plan: "plan.md"
requirements:
  - REQ-UI-APP-THEMES-001
  - REQ-UI-APP-THEMES-002
acceptance_criteria:
  - AC-UI-APP-THEMES-001.1
  - AC-UI-APP-THEMES-001.2
  - AC-UI-APP-THEMES-001.3
  - AC-UI-APP-THEMES-001.4
  - AC-UI-APP-THEMES-001.5
  - AC-UI-APP-THEMES-002.2
  - AC-UI-APP-THEMES-002.3
system_design:
  - ../../specs/ui/system-design/application-themes.md
---

# Task 03: Generic application and renderer pipeline

## Summary

Resolve any eligible theme reference into a complete app palette and generic
renderer adapters. Preserve existing light/dark consumers and editing/runtime
state; add no theme-specific product constants or selector option yet.

## In scope

Neutral schema/types/resolver and cross-language fixture checks; state/API/hook
inventory/content loading and stale-response fencing; effectiveTheme provider;
CSS role application/cleanup; authenticated pre-paint fallback; PWA metadata; all Monaco,
CodeMirror, Shiki, Pierre, Tiptap and xterm adapters in the design matrix.
Use actual Pierre registration/worker updates, not only mocked theme props.
Keep existing binary plugin API and intentional fixed-dark terminal path.

## Out of scope

Theme-specific stylesheets, grammar-engine replacement, palette-specific host
branches, selector/import UI, marketplace service and broad unrelated CSS cleanup.

## Acceptance

1. Two arbitrarily named fixture variants render across shell and each renderer
   through the same pipeline, with fresh defaults and no old overrides leaking.
   Returning to built-ins restores the existing binary API and base palette.
2. Digest/inventory/auth owner changes fence late results; unavailable selection,
   temporary refresh failure, corrupt data and highlighter rejection follow
   the specified distinct recovery paths without unsafe CSS or source loss.
3. Same-scheme transitions and version changes update worker/canvas colors while
   retaining editor models/undo/selection, terminal buffer/socket and diff state.

## ASCII UI preview

UI-04 from [the plan](plan.md#ascii-ui-preview):

```text
Desktop: Sidebar | Task tabs | File editor | Changes / Review | Terminal / Chat
Phone:   Task header -> Current file, Changes or Chat -> Existing bottom nav
```

Generic palette colors apply within existing geometry; no new task controls or
phone workbench are mounted. Maps to AC-UI-APP-THEMES-001.2/.3/.5 and 002.3/.5.
UI-01 selector is exposed in Task 05. Programmatic test selection is sufficient here.

## Verification

Run from repository root, installing dependencies once for this fresh worktree.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/theme hooks/domains/plugins/use-theme-inventory.test.ts lib/api/domains/themes-api.test.ts lib/state/slices/plugins components/theme components/theme-provider.test.tsx components/editors components/diff components/task/use-terminal-theme.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps && pnpm --filter @kandev/web build:vite)
git diff --check
```

Add `theme-resolver.test.ts`, `theme-schema.test.ts` (conformance against Task 01
fixtures), `theme-bootstrap.test.ts`, `theme-runtime.test.ts`, theme API/hook/store
regressions, and actual custom-theme worker test coverage. Prove bootstrap HTML
ordering and safe generated-script behavior, not only resolver output.
Focused rendered checks are completed by Tasks 04/05 after user controls exist.

## Files likely touched

- `apps/web/lib/theme/` types, resolver, schema validation, bootstrap and renderer
  mappings plus tests; `apps/web/lib/settings/types.ts` re-export.
- `apps/web/lib/api/domains/themes-api.ts`, hooks/domains/plugins/use-theme-inventory.ts,
  plugin store slice and WS handler, boot hydration/types and tests (new/extended).
- `apps/web/components/theme/app-theme.tsx`, theme-provider.tsx and tests.
- `apps/web/app/globals.css`, scoped Tiptap/Dockview theme-dependent colors;
  `apps/web/vite.config.ts` bootstrap insertion.
- `apps/web/components/editors/{monaco,codemirror,shiki}/`,
  `components/diff/use-diff-options.tsx`, diff-worker-pool-provider.tsx and tests.
- `apps/web/lib/theme/terminal-theme.ts`, task/use-terminal-theme.ts and callers/tests.

## Dependencies

Task 02 supplies eligible inventory/content/digest protocol. Task 01 schema and
fixtures are the contract; no frontend-originated installed-theme authority.

## Risks

Monaco global theme slots, racey worker setRenderOptions, bool-only memo keys,
unsafe startup data, language-dependent token approximations and engine registry retention.

## Parallelism

`sequential`

## Inputs

UI design in full, package inventory/schema sections, existing terminal/Monaco/
Shiki/Pierre integration and state hydration conventions.

## Results

Implemented the generic resolver, validated authenticated inventory/content,
browser-local persistence, preview/save/recovery, and adapters for Monaco,
CodeMirror, Shiki, Pierre and xterm. Optional shell-color caching is omitted;
the built-in base paints until authenticated content resolves.

Relevant frontend suites passed (64 files / 343 tests), with a subsequent focused
run covering final edits (28 files / 123 tests; overlapping coverage). Typecheck
and changed-file ESLint passed. The running-instance browser check verified
Monaco colors, actual Pierre worker token colors, terminal ANSI colors, and
editor/undo/terminal-buffer continuity across disable/re-enable.

See [delivery evidence](delivery.md) for executed checks and limitations.
