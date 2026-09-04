---
id: "01-render-compact-mcp-strategy-options"
title: "Render compact MCP strategy options"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-DESCRIPTIVE-SELECT-OPTIONS-001
acceptance_criteria:
  - AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.1
  - AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.2
  - AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.3
  - AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.4
  - AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.5
system_design:
  - ../../specs/ui/system-design/descriptive-select-options.md
---

# Task 01: Render Compact MCP Strategy Options

## Summary

Render backend-supplied MCP strategies with a concise first-row name and a
second-row mechanism description. Keep the selected trigger compact, preserve
value semantics, and prove containment and touch behavior at desktop and phone
widths.

## In scope

- Add a failing focused browser regression before changing the selector.
- Reuse the shared `SelectItem.description` property in `MCPStrategySelect`.
- Add local shrink/full-width and coarse-pointer hit-area classes.
- Add focused desktop and mobile Playwright coverage for the real dialog.

## Out of scope

- Shared Select primitive changes.
- Locale copy changes.
- Backend, API, store, persistence, or runtime MCP changes.

## Acceptance

- Every backend strategy option exposes a first-row name and an associated
  second-row description, while the closed trigger contains only the name.
- The existing sentinel mapping and submitted strategy key remain unchanged.
- Desktop and mobile browser checks prove dialog/list containment, no document
  horizontal overflow, and coarse-pointer hit areas of at least 44 pixels.

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
cd apps/web
pnpm test -- components/settings/mcp-strategy-select.test.tsx
pnpm run typecheck
pnpm exec eslint components/settings/mcp-strategy-select.tsx components/settings/mcp-strategy-select.test.tsx e2e/tests/settings/custom-tui-mcp-strategy-selector.spec.ts e2e/tests/settings/mobile-custom-tui-mcp-strategy-selector.spec.ts
pnpm run i18n:check
pnpm e2e:run --project chromium tests/settings/custom-tui-mcp-strategy-selector.spec.ts
pnpm e2e:run --project mobile-chrome tests/settings/mobile-custom-tui-mcp-strategy-selector.spec.ts
```

The desktop and mobile browser regressions must fail for the expected
concatenated trigger, missing option structure, containment, or hit-area reason
before the implementation change. The managed E2E runner rebuilds the
production frontend before each final browser run.

## Files likely touched

- `apps/web/components/settings/mcp-strategy-select.tsx`
- `apps/web/components/settings/mcp-strategy-select.test.tsx`
- `apps/web/e2e/tests/settings/custom-tui-mcp-strategy-selector.spec.ts`
- `apps/web/e2e/tests/settings/mobile-custom-tui-mcp-strategy-selector.spec.ts`

## Dependencies

None.

## Risks

- Description text placed inside `SelectPrimitive.ItemText` would recreate the
  overflow by restoring it to the selected trigger.
- A desktop-only assertion would miss coarse-pointer row sizing and mobile
  portal containment.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/descriptive-select-options.md`
- `docs/specs/ui/system-design/descriptive-select-options.md`
- `apps/web/components/task/sidebar-filter/group-picker.tsx`
- `apps/packages/ui/src/select.tsx`

## Results

Completed on 2026-09-04.

- RED: the desktop regression failed because the flattened option had no
  `aria-describedby`; the mobile regression then failed because the trigger
  rendered at 27.4 pixels instead of the required 44 pixels.
- GREEN: the desktop Chromium and mobile-Chrome regressions each passed after
  the two-row anatomy, containment sizing, and coarse-pointer hit areas were
  applied.
- `mcp-strategy-select.test.tsx`: 5 tests passed.
- TypeScript, focused ESLint, Prettier, i18n, specification lints, and
  `git diff --check` passed.
- The open selector was inspected at 1372 x 900 and 393 x 851. Strategy names
  and descriptions render on separate rows, and the dialog and menu remain
  within the viewport.
