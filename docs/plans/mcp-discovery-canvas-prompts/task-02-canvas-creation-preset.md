---
id: "02-canvas-creation-preset"
title: "Canvas creation preset"
status: done
wave: 2
depends_on:
  - "01-mcp-discovery-guidance"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-009
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-009.6
  - AC-CANVASES-AGENT-WEB-APPS-009.7
  - AC-CANVASES-AGENT-WEB-APPS-009.8
system_design:
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 02: Canvas creation preset

Subsequent design: [direct canvas creation](../canvas-direct-creation/plan.md)
moves the detailed preset into a saved prompt. This completed work order
records the earlier implementation; the new package owns replacement tests.

## Summary

Strengthen the editable Create canvas prompt with explicit MCP discovery and
publication steps. Preserve the normal task dialog on desktop and mobile.

## In scope

- Use the proposed wording in the canvas design's discovery section.
- Update English, Portuguese, and Simplified Chinese catalogs. Generate the
  Traditional Chinese pair and pseudo-locale through repository scripts.
- Preserve exact tool identifiers in each locale.
- Cover real catalog content instead of only mocked translation values.
- Extend the mobile guided task case to inspect the default prompt before
  replacing it. Add a desktop case with the same user outcome.
- Assert the submitted description preserves a user's edit. Retain the
  existing scratch/local defaults and editable workflow/profile controls.

## Out of scope

- Canvas service changes, approval automation, or a new dialog layout.
- Live models, persistent demo tasks, or repair of the original failed task.

## Acceptance

1. Every supported locale explains discovery, create, one core skill read,
   assigned-directory edits, live data, publication, and accurate status.
2. Desktop and mobile show an editable preset and submit the edited value.
   The longer text leaves the task dialog usable without horizontal overflow.
3. Catalog, component, and targeted E2E checks pass. Evidence distinguishes
   prompt delivery from external model compliance.

## Verification

From `apps`, install dependencies once if this worktree has no installation:

```bash
rtk pnpm install --frozen-lockfile
```

From `apps/web`:

```bash
rtk pnpm run i18n:zh-hant
rtk pnpm run i18n:pseudo
rtk pnpm exec vitest run components/canvas/canvas-task-create-launcher.test.tsx components/canvas/canvas-task-prompt.test.ts
rtk pnpm run i18n:check
rtk pnpm e2e:run --project chromium tests/canvas/plugin-canvas.spec.ts -- --grep 'canvas creation prompt'
rtk pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --grep 'creates a scratch canvas task'
```

Use `canvas creation prompt` in the new desktop test title. The managed E2E
runner builds production artifacts and owns isolated instance cleanup. Run
desktop and mobile sequentially. Inspect the rendered phone dialog screenshot.
Use TDD for the new prompt contract and submission assertions.

## Files likely touched

- `apps/web/src/locales/en/canvases.json`
- `apps/web/src/locales/pt-pt/canvases.json`
- `apps/web/src/locales/zh-cn/canvases.json`
- Generated `zh-hk`, `zh-tw`, and pseudo canvas catalogs.
- `apps/web/components/canvas/canvas-task-create-launcher.test.tsx`
- `apps/web/components/canvas/canvas-task-prompt.test.ts` (new)
- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`

## Dependencies

Task 01 supplies the common discovery and capability-aware guidance.

## Risks

Mocking the locale value can pass while production wording is wrong. Inspect
the real catalog text. Existing MCP fixture commands test the lifecycle, not
whether a model discovers the tools from the default prompt.

## Parallelism

`sequential`

## Inputs

- Canvas requirements and design, especially guided launch and discovery.
- `CanvasTaskCreateLauncher` and its current preset test.
- Existing mobile guided task case and the standard full-screen task dialog.
- Repository i18n, mobile-parity, and E2E guidance.

## Results

- Strengthened the English, Portuguese, Simplified Chinese, Traditional
  Chinese, and pseudo-locale canvas task presets with conditional discovery,
  draft creation, one skill read, returned-directory editing, authorized live
  data, publication, permission review, and accurate release reporting.
- Updated pseudo-locale generation to preserve Markdown code spans so the
  exact callable canvas tool names remain unchanged in development catalogs.
- Added real-catalog unit coverage: 2 files and 4 tests passed. i18n key,
  punctuation, non-JSX-copy, and new-code ratchet checks passed; TypeScript,
  ESLint, and Prettier checks passed.
- Desktop E2E passed 1 test. Mobile E2E passed 1 test. Both flows showed the
  editable preset, retained the submitted description, and had no horizontal
  viewport overflow.
