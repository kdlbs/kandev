---
id: "02-direct-canvas-dialog"
title: "Open the canvas task dialog directly"
status: done
wave: 2
depends_on:
  - "01-canvas-saved-prompt"
plan: "plan.md"
requirements:
  - REQ-CANVASES-AGENT-WEB-APPS-009
acceptance_criteria:
  - AC-CANVASES-AGENT-WEB-APPS-009.1
  - AC-CANVASES-AGENT-WEB-APPS-009.2
  - AC-CANVASES-AGENT-WEB-APPS-009.3
  - AC-CANVASES-AGENT-WEB-APPS-009.4
  - AC-CANVASES-AGENT-WEB-APPS-009.5
  - AC-CANVASES-AGENT-WEB-APPS-009.6
  - AC-CANVASES-AGENT-WEB-APPS-009.7
  - AC-CANVASES-AGENT-WEB-APPS-009.9
  - AC-CANVASES-AGENT-WEB-APPS-009.11
system_design:
  - ../../specs/canvases/system-design/guided-canvas-task-launch.md
---

# Task 02: Open the canvas task dialog directly

## Summary

Reuse the canvas task launcher from the empty sidebar row. Present the short
goal plus saved reference on desktop and phone, with the existing task options.

## In scope

- Shared launcher presentation, direct semantic button, stable dialog lifetime,
  workspace reset, cancel/focus, failure/retry, and success navigation.
- Localized goal plus literal reference; update actual preset contract tests.
- All tests in the plan's UI and E2E matrix; retain settings shortcut behavior.
- Generate Traditional Chinese and pseudo catalogs. Use `/mobile-parity`,
  `/tdd`, and `/e2e` for implementation and rendered checks.
- Update canvas creation and saved-prompt public docs through `/docs-maintainer`.

## Out of scope

- Saved-prompt backend implementation, generic autocomplete behavior,
  additional mobile navigation, and canvas permission or runtime changes.

## Acceptance

1. Sidebar setup opens the same task dialog without route change, retaining
   task defaults, management navigation, cancellation, and workspace isolation.
2. All locales show a short editable goal followed by exact `@create-canvas`;
   desktop and phone submit user edits and preserve them after failure.
3. Targeted component, catalog, desktop/mobile E2E, and documentation checks
   pass; rendered phone geometry matches the preview.

## ASCII UI preview

Excerpts from [the full preview](plan.md#ascii-ui-preview):

```text
UI-01 Desktop, empty sidebar
[Set up a canvas] -> [Task dialog on current route]
  [Editable coordinator goal]
  [@create-canvas]
  [Agent / Model / Executor] [Workflow / Step]
  [Cancel] [Start task v]

UI-02 Phone, workspace settings
[Create canvas] -> [Full-screen task form]
  [Editable goal and @create-canvas]
  [Task options, scrolling body]
  [Cancel] [Start task v] (reachable footer)

UI-03 Submission failure
[Edited goal and reference preserved]
[Existing error] [Cancel] [Start task v]
```

UI-01 covers `.1`–`.4`, `.6`, `.7`, `.9`, `.11`; UI-02 covers `.5`–`.7`,
`.9`, `.11`; UI-03 covers `.11`. All refer to the canvas launch requirement.
Use the existing full-screen task form and one body scroll owner, dynamic
viewport containment, safe areas, and 44px phone targets. Desktop retains its
normal density. Grouping and behavior are required; ASCII spacing is not.

## Verification

Run from the repository root. Install workspace dependencies once if missing:

```bash
(cd apps && rtk pnpm install --frozen-lockfile)
```

Then run these commands sequentially:

```bash
(cd apps/web && rtk pnpm run i18n:zh-hant)
(cd apps/web && rtk pnpm run i18n:pseudo)
(cd apps/web && rtk pnpm exec vitest run components/canvas/canvas-task-create-launcher.test.tsx components/canvas/canvas-task-prompt.test.ts components/app-sidebar/sections/canvases-section.test.tsx components/settings/workspace-canvases-page.test.tsx)
(cd apps/web && rtk pnpm run typecheck)
(cd apps/web && rtk pnpm run i18n:check)
(cd apps/web && rtk pnpm run i18n:ratchet)
(cd apps/web && rtk pnpm exec eslint components/canvas/canvas-task-create-launcher.tsx components/app-sidebar/sections/canvases-section.tsx)
(cd apps/web && rtk pnpm e2e:run --project chromium tests/canvas/plugin-canvas.spec.ts -- --grep 'canvas setup|canvas creation prompt' --retries=0)
(cd apps/web && rtk pnpm e2e:run --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts -- --grep 'creates a scratch canvas task' --retries=0)
rtk node --test scripts/validate-public-docs.test.mjs
rtk node scripts/validate-public-docs.mjs
rtk python3 scripts/lint-spec-files.py --all
rtk git diff --check
```

If extracting a helper adds another test file, add that exact path to this block
before marking the task complete. Managed E2E builds production artifacts;
inspect its phone screenshot for structure as well as geometry assertions.

## Files likely touched

- `apps/web/components/canvas/canvas-task-create-launcher.tsx` and its test
- `apps/web/components/app-sidebar/sections/canvases-section.tsx` and its test
- `apps/web/components/canvas/canvas-task-prompt.test.ts`
- `apps/web/components/settings/workspace-canvases-page.test.tsx`
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,pseudo}/canvases.json`
- `apps/web/e2e/tests/canvas/{plugin-canvas,mobile-plugin-canvas}.spec.ts`
- `docs/public/canvases.md`, `docs/public/developer-tools.md`

## Dependencies

Task 01 must seed the saved prompt before the UI references it.

## Risks

Empty-row unmount or workspace switching can lose or misroute a draft. Mocked
translation tests can miss catalog regressions. Browser tests that replace the
entire preset cannot prove saved-prompt delivery.

## Parallelism

`sequential`

## Inputs

- Canvas requirements `.009` and design sections for guided launch and prompts.
- Existing launcher, sidebar tests, task dialog, and mobile creation scenario.
- Existing completed prompt and UX packages linked from the plan.

## Results

- Added the shared `CanvasTaskCreateLauncher` sidebar presentation. The empty
  canvas row is now a semantic button that opens the existing task dialog on
  the current route; the settings shortcut retains its existing path.
- Kept the task description editable and localized as a short goal followed by
  the literal `@create-canvas` reference. The dialog resets on workspace or
  feature changes, restores focus on cancellation, and preserves the editor on
  failed submission.
- Updated desktop and phone canvas scenarios, including direct sidebar launch,
  settings launch, edited prompt retention, cancellation, and controlled
  creation failure/retry. Generated Traditional Chinese and pseudo catalogs.
- Added public guidance to `docs/public/canvases.md` and
  `docs/public/developer-tools.md`.
- Focused component/catalog/settings tests: 19 passed in 4 files.
- `cd apps/web && rtk pnpm run typecheck`: passed.
- `cd apps/web && rtk pnpm run i18n:check`: passed.
- `cd apps/web && rtk pnpm run i18n:ratchet`: passed.
- `cd apps/web && rtk pnpm exec eslint components/canvas/canvas-task-create-launcher.tsx components/app-sidebar/sections/canvases-section.tsx`: passed.
- Chromium canvas E2E: 2 passed.
- Mobile canvas E2E: 1 passed.
- Fresh desktop and Pixel 5 dialog captures passed fixture assertions, were
  inspected for layout and content, and were compressed for PR publication.
- `rtk node --test scripts/validate-public-docs.test.mjs`: 62 passed.
- `rtk node scripts/validate-public-docs.mjs`: 46 published docs validated.
- `rtk python3 scripts/lint-spec-files.py --all`: passed.
- `rtk git diff --check`: passed.
