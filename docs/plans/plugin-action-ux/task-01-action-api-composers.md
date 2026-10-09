---
id: "01-action-api-composers"
title: "Shared action API and composers"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-ACTION-UX-001
  - REQ-PLUGINS-ACTION-UX-002
  - REQ-PLUGINS-ACTION-UX-003
acceptance_criteria:
  - AC-PLUGINS-ACTION-UX-001.1
  - AC-PLUGINS-ACTION-UX-001.2
  - AC-PLUGINS-ACTION-UX-001.4
  - AC-PLUGINS-ACTION-UX-002.1
  - AC-PLUGINS-ACTION-UX-002.2
  - AC-PLUGINS-ACTION-UX-002.3
  - AC-PLUGINS-ACTION-UX-002.4
  - AC-PLUGINS-ACTION-UX-002.5
  - AC-PLUGINS-ACTION-UX-002.7
  - AC-PLUGINS-ACTION-UX-003.1
  - AC-PLUGINS-ACTION-UX-003.2
  - AC-PLUGINS-ACTION-UX-003.3
  - AC-PLUGINS-ACTION-UX-003.5
system_design:
  - ../../specs/plugins/system-design/plugin-action-ux.md
---

# Task 01: Shared action API and composers

## Summary

Deliver the typed Action and ActionGroup exports and one working composer slice.
Retain legacy rendering and composer capability ownership.

## In scope

- Implement surface styles, prop filtering, structural event/ref types, and nullable surface context.
- Extend internal PluginSlot props without changing public registration signatures or component keys.
- Integrate task chat, Quick Chat, task creation, and new-session creation.
- Share composer styling with AttachFilesButton and ordinary neighboring ghost controls.
- Add standard-action fixture controls and desktop/mobile composer geometry tests.
- Extend SDK consumer tests for cloneable triggers, refs, pointer capture, and fallback selection.

## Out of scope

Topbar/sidebar/status integration, official plugin changes, new overlay state machines, and generalized action registration.

## Acceptance

1. A typed plugin renders Action inside each composer slot without imports from private host modules.
2. Native and plugin composer controls share geometry. Hold, disabled, busy-stop, and focus behavior pass targeted tests.
3. An unchanged legacy composer contribution and an old-host fallback remain functional without duplicate registration.

## ASCII UI preview

UI-01 / UI-02 / UI-03: Composer controls. See the [combined previews](plan.md#ascii-ui-preview).
The frontmatter criteria apply to this excerpt. Labels and glyphs are illustrative.

```text
Desktop: [attach] [plugin mic] [plugin cost] ... [send]
Phone:   [attach] [plugin mic] [send]
States:  [mic] -> [stop] -> [busy]   [disabled]
```

Desktop uses 28px controls. Phone/coarse pointers use 44px targets. Keep the active draft and composer capability local.

## Verification

Run from the repository root. Use TDD for new behavior and record the initial
behavioral failure. New test paths in this order are files to create.
For a fresh worktree, first run `(cd apps && pnpm install --frozen-lockfile)`.

```bash
(cd apps/packages/plugin-sdk && pnpm test && pnpm typecheck)
(cd apps/web && pnpm exec vitest run lib/plugins/sdk-contract.test.ts lib/plugins/host-api.test.ts components/plugins/plugin-action.test.tsx components/plugins/plugin-slot.test.tsx components/task/chat/chat-input-plugin-actions.test.tsx components/task-create-dialog-selectors.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium e2e/tests/plugins/plugin-action-ux.spec.ts --grep composer)
(cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/plugins/mobile-plugin-action-ux.spec.ts --grep composer)
```

Run any other test file changed by this work order with the same targeted runner.
For rendered changes, inspect the focused desktop and phone screenshots against
the assigned previews. Do not infer CSS geometry from unit tests.

## Files likely touched

- `apps/packages/plugin-sdk/src/index.ts` and `test/standalone-contract.node.mjs`
- `apps/web/lib/plugins/{types.ts,host-api.ts,sdk-contract.test.ts,host-api.test.ts}`
- New `apps/web/components/actions/{surface-action.tsx,surface-action-styles.ts}`
- New `apps/web/components/plugins/{plugin-action.tsx,plugin-action.test.tsx,plugin-action-surface.tsx}`
- `apps/web/components/plugins/{plugin-slot.tsx,plugin-slot.test.tsx}`
- `apps/web/components/task/chat/{chat-input-plugin-actions.tsx,chat-input-plugin-actions.test.tsx,chat-input-toolbar-primitives.tsx}`
- `apps/web/components/task-create-dialog-selectors.tsx` and its test
- `apps/backend/cmd/plugin-fixture/fixture-package/ui/bundle.js` (test fixture only)
- New `apps/web/e2e/tests/plugins/{plugin-action-ux.spec.ts,mobile-plugin-action-ux.spec.ts}`

## Dependencies

None.

## Risks

Do not put composer capabilities into surface context. SDK event types must preserve real button methods without introducing a React dependency.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/plugins/requirements/plugin-action-ux.md)
- [System design](../../specs/plugins/system-design/plugin-action-ux.md)
- [Plan and source audit](plan.md)
- Existing plugin-slot, SDK consumer, and packaged plugin E2E patterns.

## Results

Complete. `host.ui.Action` and `ActionGroup` are additive typed exports backed
by the shared composer action surface. Existing slots and composer capability
props remain unchanged. Chat, Quick Chat, task-create, and new-session composers
receive explicit context; the attachment control uses the same sizing contract.

Validation passed:

- `(cd apps/packages/plugin-sdk && pnpm test && pnpm typecheck)`
- `(cd apps/web && pnpm exec vitest run lib/plugins/sdk-contract.test.ts lib/plugins/host-api.test.ts components/plugins/plugin-action.test.tsx components/plugins/plugin-slot.test.tsx components/task/chat/chat-input-plugin-actions.test.tsx components/task-create-dialog-selectors.test.tsx)` (75 tests)
- `(cd apps/web && pnpm run typecheck)`
- Desktop composer E2E and phone composer E2E through the managed runner (1 each).
- Desktop and phone composer screenshots inspected against UI-01/UI-02; controls remain beside the active composer and phone hit areas meet 44px.

The initial desktop E2E assertion counted the native attachment action with the
plugin actions. The selector now scopes fixture actions by test ID; both focused
browser checks pass.

Review remediation (2026-09-25): desktop and phone composer browser checks now
measure the SVG itself at the intended 16px, so a correctly sized wrapper can
no longer hide a 14px glyph. The updated mobile composer/action suite passed
as part of the 5/5 mobile action and Status drawer run.

PR review remediation (2026-09-25): Action now displays its label when no icon
or non-empty text is present. A disabled Action with a tooltip exposes a
focusable host trigger, and ActionGroup reads its surface context before its
empty-child return to keep hook ordering unconditional. The combined focused
renderer suite passed (3 files, 19 tests):
`(cd apps/web && pnpm exec vitest run components/plugins/plugin-action.test.tsx components/kanban/main-top-bar-plugin-actions.test.tsx components/actions/surface-action-styles.test.ts)`.
Also passed: `(cd apps/web && pnpm run typecheck)` and focused ESLint on the
changed renderer, topbar, style, test, and action UX E2E files.

The original PR-head frontend job still expected the pre-migration `h-7` and
`min-h-11` class names on every composer action. Updated the responsive toolbar
test to accept the shared square `size-7`/`size-11` contract or existing
minimum-size controls. The focused toolbar suite passed all 27 tests:
`(cd apps/web && pnpm exec vitest run components/task/chat/chat-input-toolbar.test.tsx)`.

Original PR-head CI also showed the mobile HTML preview's `Files` locator
matching both the navigation button and the newly labeled `Attach files`
composer action. The locator now uses an exact accessible name. Its focused
managed mobile E2E passed 1/1. The unchanged file-tree download test had a
worktree-materialization timeout in the original CI shard; its targeted managed
Chromium rerun passed 1/1.
