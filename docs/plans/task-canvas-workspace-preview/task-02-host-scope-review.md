---
id: "02-host-scope-review"
title: "Host scope review and browser proof"
status: complete
wave: 2
depends_on:
  - "01-workspace-data-scope"
plan: "plan.md"
requirements:
  - REQ-CANVASES-WORKSPACE-PREVIEW-001
  - REQ-CANVASES-AGENT-WEB-APPS-003
acceptance_criteria:
  - AC-CANVASES-WORKSPACE-PREVIEW-001.1
  - AC-CANVASES-WORKSPACE-PREVIEW-001.4
  - AC-CANVASES-WORKSPACE-PREVIEW-001.5
  - AC-CANVASES-WORKSPACE-PREVIEW-001.7
  - AC-CANVASES-AGENT-WEB-APPS-003.1
system_design:
  - ../../specs/canvases/system-design/task-canvas-workspace-preview.md
  - ../../specs/canvases/system-design/agent-authored-web-apps.md
---

# Task 02: Host scope review and browser proof

## Summary

Show task placement and workspace data access separately in the host and
promotion review. Give older task-only canvases an owner-reviewed access
action on desktop and phone, update author guidance, and prove live workspace
task refresh before promotion in browser tests.

## In scope

- Canvas API client/projections, existing host actions and review component,
  localized copy in six catalogs, and phone action drawer.
- Versioned bundled authoring reference, public canvas and plugin API docs,
  and concise scoped `AGENTS.md` updates if implementation changes their
  current statements.
- Component and desktop/mobile Playwright tests using multiple tasks in one
  workspace and a foreign-workspace negative case.

## Out of scope

- New canvas-authored UI template, broad UI redesign, installation-wide task
  listing, and silent legacy grant migration.

## Acceptance

1. Desktop and phone hosts label the data scope accurately and offer the
   upgrade review only for eligible task-only canvases; the action does not
   promote or republish.
2. Promotion review distinguishes placement from data access and keeps exact
   declared permission details visible on both viewports.
3. Browser tests show a new task canvas refreshing at least two workspace
   tasks before promotion and an existing one expanding access after review.

## ASCII UI preview

`UI-01: Task canvas host, new release (desktop)` and
`UI-02: Existing task-only canvas (desktop)` use the same header shown in
[the full plan](plan.md#ascii-ui-preview):

```text
New:      Task coordinator [Workspace data] [Releases] [Promote canvas]
Existing: Task coordinator [Task data] [Enable workspace data] [Promote canvas]
```

`UI-03: Existing task-only canvas (phone)` uses the existing focused route
and Actions drawer. Its review is full-height with one scroll region and
fixed safe-area actions:

```text
< Task   Task coordinator             [Actions]
         Task data
Actions > Enable workspace data
Review declared permissions [scrolls]
[Cancel] [Enable workspace data] [fixed, safe area]
```

The scope label and action availability are required by
`AC-CANVASES-WORKSPACE-PREVIEW-001.4-.5` and `.7`; text spacing is
illustrative. Reuse the shipped canvas host and phone drawer.

## Verification

Run from the repository root:

```bash
(cd apps/web && pnpm exec vitest run components/settings/canvas-lifecycle-dialogs.test.tsx components/settings/canvas-host-components.test.tsx lib/api/domains/canvas-api.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --host --project chromium tests/canvas/plugin-canvas.spec.ts)
(cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/canvas/mobile-plugin-canvas.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/web/lib/api/domains/canvas-api.ts`
- `apps/web/components/settings/canvas-host-actions.tsx`
- `apps/web/components/settings/canvas-lifecycle-dialogs.tsx`
- `apps/web/components/settings/canvas-host-route*.tsx`
- `apps/web/src/locales/*/canvases.json`
- `apps/web/e2e/tests/canvas/plugin-canvas.spec.ts`
- `apps/web/e2e/tests/canvas/mobile-plugin-canvas.spec.ts`
- `apps/backend/internal/mcp/canvasskill/files/references/browser-api.md`
- `apps/backend/config/prompts/create-canvas.md`
- `docs/public/canvases.md`
- `docs/public/plugins-authoring.md`
- Adjacent component tests and `AGENTS.md` files when needed

## Dependencies

Task 01 supplies the data-scope projection and conditional access endpoint.

## Risks

The current canvas fixture expects one task. The browser proof must create a
second task in the same workspace and retain a foreign-workspace negative case
without changing unrelated fixture behavior.

## Parallelism

`sequential`

## Inputs

- [Workspace preview requirements](../../specs/canvases/requirements/task-canvas-workspace-preview.md)
- [Workspace preview design](../../specs/canvases/system-design/task-canvas-workspace-preview.md)
- [Full UI preview](plan.md#ascii-ui-preview)

## Results

Passed: focused Vitest suite (41 tests), web typecheck, i18n checks, desktop
canvas Playwright suite, mobile canvas Playwright suite (5 tests), spec index
validation, specification lint, and public documentation validation. The phone
browser test verifies both scope labels and the legacy review flow; it uses a
test-only legacy API response, while backend service/store tests verify the
transactional grant upgrade.
