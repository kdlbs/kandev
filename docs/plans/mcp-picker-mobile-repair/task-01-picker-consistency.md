---
id: "01-picker-consistency"
title: "Unify MCP picker details and repository disclosure"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-WORKSPACE-MCP-CONFIGURATION-006
acceptance_criteria:
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.7
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.8
  - AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.21
system_design:
  - ../../specs/agents/system-design/workspace-mcp-configuration.md
---

# Task 01: Unify MCP picker details and repository disclosure

## Summary

One expansion reveals repository MCP choices. Every shared picker choice shows
the same truthful description and definition metadata already used in Customize.
Preserve each caller's form, selection and save ownership.

## In scope

Repository inline selection content, shared row metadata/accessibility, desktop
and phone regressions for repository/task creation, and the manual guide update.

## Out of scope

Session origin grouping changes, inherited disabling, autosave changes, global
profile routing, runtime attachment, catalog configuration or new strings unless
existing localized copy cannot express the agreed UI.

## Acceptance

1. Repository MCP is collapsed initially with one selected count. Expanding
   directly reveals its existing description, search and choices. No second MCP
   trigger or nested drawer; shared Save changes/discard/reload still works.
2. Every rendered shared choice shows alias, supplied nonblank description and
   passive mode/transport/known-source badges; checkbox aligns with name and
   describes support text. Disabled and inherited restrictions remain intact.
3. Desktop, phone and coarse-pointer lists wrap and reach their final choice;
   task Advanced settings stays compact and collapsed initially. Session
   pending actions and origin grouping retain their existing presentation.

## ASCII UI preview

UI-01/02 excerpt from [combined preview](plan.md#ascii-ui-preview):

```text
Repository: MCP servers [0] v      Task: MCP servers (i) [0 selected v]
            [Search MCP servers]  Picker: [Search MCP servers]
            [ ] Atlas docs                [ ] Atlas docs
                atlas-docs                    atlas-docs
                Shared guides.                Shared guides.
                [Remote] [SSE] [Custom]       [Remote] [SSE] [Custom]
```

Repository has one inline expansion; task retains its desktop popover/phone
sheet. Phone wraps metadata and keeps one existing scroller with 44px targets.
Map to AC-006.7/8/21; example badge abbreviations are illustrative, not new copy.
Loading, search-empty, disabled and error meanings remain unchanged.

## Verification

From the repository root, after dependencies are installed:

```bash
(cd apps/web && corepack pnpm@9.15.9 exec vitest run components/mcp/mcp-definition-badges.test.ts components/mcp/mcp-origin-groups.test.ts)
(cd apps/web && corepack pnpm@9.15.9 run typecheck)
(cd apps && corepack pnpm@9.15.9 --filter @kandev/web lint)
make build-web PNPM="corepack pnpm@9.15.9"
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project chromium e2e/tests/settings/workspace-mcp-configuration.spec.ts e2e/tests/task/task-mcp-selection.spec.ts)
(cd apps/web && corepack pnpm@9.15.9 e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-workspace-mcp-configuration.spec.ts e2e/tests/task/mobile-task-mcp-selection.spec.ts)
git diff --check
```

Write and run repository single-disclosure and repository/task metadata
regressions before implementation; verify the expected failure. Include blank
description and disabled choices, keyboard/tap selection, save/discard/reload,
name/alias filtering, final choice reachability and no horizontal overflow.
Retain all runner session handles. Do not overlap E2E suites or bypass build
freshness guards. If new copy is unavoidable, add all shipped locales and run
`(cd apps/web && corepack pnpm@9.15.9 run i18n:check)` as an additional gate.

## Files likely touched

- `apps/web/components/settings/repository-mcp-selection.tsx`
- `apps/web/components/mcp/mcp-selection-picker.tsx`
- `apps/web/components/mcp/mcp-definition-details.tsx` only if shared markup needs adjustment
- The four settings/task E2E files named above and bounded shared test helpers
- `docs/plans/workspace-mcp-configuration/testing-guide.md`
- `docs/plans/workspace-mcp-configuration/ux-consistency-review.md`

Do not duplicate metadata in `task-create-dialog-mcp-select.tsx` or other
callers: they already render shared selection content.

## Dependencies

None.

## Risks

Longer choices can conceal the last row or disabled badge. Preserve settings
baseline/reset paths and do not introduce an independent selection controller.

## Parallelism

`sequential`

## Inputs

The owning design's Frontend surfaces and Responsive behavior sections; current
`ServerRow`, `McpDefinitionDetails`, repository save contributor and task-create
popover/drawer. Existing Customize origin package remains a completed record.

## Results

Implemented. RED: the two focused desktop regressions failed on the intended
missing search-after-one-expansion and missing task description. GREEN:

- Badge/group Vitest suites: 2 files, 25 tests passed.
- Typecheck: passed.
- Initial lint identified the repository component at 104 lines. Extracted its
  disclosure header; targeted lint passed. Full final lint passed after Task 02.
- Production web build: passed, rebuilt after the header extraction.
- Required desktop settings/task E2E: 14 passed.
- Required mobile settings/task E2E: 10 passed.
- `git diff --check`: passed.

All new rows reuse existing localized keys; no new copy or catalog generation.
Final affected-regression reruns passed: two desktop and two phone picker
scenarios after the last production edit. Asset-only preview deployment and
saved-selection/process guards passed in Task 02.
