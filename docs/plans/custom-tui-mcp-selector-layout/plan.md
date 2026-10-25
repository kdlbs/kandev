---
created: 2026-09-04
status: done
requirements:
  - REQ-UI-DESCRIPTIVE-SELECT-OPTIONS-001
system_design:
  - ../../specs/ui/system-design/descriptive-select-options.md
legacy_specs: []
---

# Implementation Plan: Compact Custom TUI MCP Selector

## Overview

Replace the custom TUI agent MCP selector's concatenated option labels with the
shared name-plus-description anatomy already used by sidebar view selectors.
One focused work order adds the regression coverage, applies the presentation
fix, and proves dialog containment on desktop and mobile.

## Scope

### In scope

- Render each backend-supplied MCP strategy name above its description in the
  open selector.
- Keep the selected trigger compact by showing only the strategy name.
- Keep the selector and Add TUI Agent dialog contained on desktop and phone.
- Preserve accessible descriptions, selected values, and submission behavior.

### Out of scope

- Backend strategy registration, ordering, descriptions, or API payloads.
- Custom TUI agent persistence and runtime MCP injection.
- Copy changes or broad changes to the shared Select primitive.

## Technical approach

- Update `MCPStrategySelect` to pass the backend mechanism through the shared
  `SelectItem.description` property and keep `option.key` as `ItemText`.
- Give the local trigger a full-width, zero-minimum layout and give its trigger
  and option rows coarse-pointer-only 44-pixel minimum hit areas.
- Keep the focused mapping tests for stable strategy identities and use
  Playwright to inspect the rendered Radix option structure, accessibility
  association, and compact selected value.
- Add focused desktop and mobile settings E2E coverage for the real dialog and
  strategy endpoint. Use geometry assertions for viewport containment rather
  than fixed visual widths.

## Tests

- Existing value-mapping tests in
  `apps/web/components/settings/mcp-strategy-select.test.tsx` continue to prove
  that the sentinel and backend strategy keys are unchanged.

## E2E tests

- `AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.1` through `.4`:
  `apps/web/e2e/tests/settings/custom-tui-mcp-strategy-selector.spec.ts` opens
  the Add TUI Agent dialog, verifies the two-row strategy entry and compact
  selected trigger, and checks dialog/list containment.
- `AC-UI-DESCRIPTIVE-SELECT-OPTIONS-001.1` through `.5`:
  `apps/web/e2e/tests/settings/mobile-custom-tui-mcp-strategy-selector.spec.ts`
  proves the same selection through touch, checks 44-pixel hit areas, and
  asserts no document horizontal overflow in the mobile-Chrome project.

## Mobile design contract

- Desktop outcome: compare MCP strategies in the Add TUI Agent dialog without
  long descriptions widening the control or dialog.
- Mobile entry point: Add TUI Agent on `/settings/agents`; it keeps the existing
  centered dialog and selector because this is a short temporary choice.
- Nearest shipped exemplar: the sidebar view `GroupPicker` and
  `TypedSortPicker`, which use `SelectItem.description` for the same two-level
  option hierarchy.
- Hierarchy and primary action: strategy name first, mechanism second; choosing
  a row completes the temporary selection and returns to the form.
- Scroll and safe area: the Radix option list owns overflow; the document and
  dialog do not scroll horizontally. Existing dialog dismissal and footer
  behavior remain unchanged.
- Shared logic: all values, API data, selection, and form submission stay
  shared. Only responsive hit-area classes differ for coarse pointers.

## Work orders

- [x] [Task 01: Render compact MCP strategy options](task-01-render-compact-mcp-strategy-options.md)

## Verification results

Completed on 2026-09-04. The MCP strategy selector now uses the shared
name-plus-description option anatomy, keeps only the short strategy key in the
closed trigger, and provides coarse-pointer-only 44-pixel hit areas. Focused
desktop and mobile Playwright regressions passed against a rebuilt production
frontend, and rendered 1372 x 900 and 393 x 851 views were inspected without
horizontal overflow.

## Risks

- Radix derives the closed value from `ItemText`; a regression test must guard
  against accidentally placing the description back inside that node.
- Portal geometry differs between desktop and phone, so both Playwright
  projects must assert the rendered containment contract.
