---
id: "02-preserve-composer-adjacency"
title: "Preserve visible composer adjacency"
status: done
wave: 2
depends_on: ["01-clamp-composer-suggestions"]
plan: "plan.md"
spec: "../../specs/ui/requirements/composer-suggestion-overlays.md"
system_design: "../../specs/ui/system-design/composer-suggestion-overlays.md"
---

# Task 02: Preserve Visible Composer Adjacency

## Intent

Keep suggestion lists directly above a visible mobile composer while retaining
the off-screen-anchor containment fallback added in Task 01.

## Acceptance

- `AC-UI-COMPOSER-OVERLAY-001.1`: Raw anchors outside the visible viewport
  remain covered by deterministic containment tests.
- `AC-UI-COMPOSER-OVERLAY-001.3`: A keyboard-sized phone layout keeps the menu
  contained, touch-safe, and free of horizontal overflow.
- `AC-UI-COMPOSER-OVERLAY-001.4`: Touch selection still inserts the saved
  prompt and preserves focus.
- `AC-UI-COMPOSER-OVERLAY-001.5`: A visible mobile composer and suggestion
  surface have no more than an eight-pixel vertical gap.

## Files likely touched

- `apps/web/e2e/tests/chat/mobile-prompt-mention-composer.spec.ts`
- `docs/specs/ui/requirements/composer-suggestion-overlays.md`
- `docs/specs/ui/system-design/composer-suggestion-overlays.md`
- `docs/plans/mobile-composer-suggestion-viewport/plan.md`
- `docs/plans/mobile-composer-suggestion-viewport/task-02-preserve-composer-adjacency.md`

## Steps

1. Add a menu-to-composer bounding-box assertion to the current mobile prompt
   scenario and record its failure against the occlusion-only viewport mock.
2. Resize the page to the keyboard-sized height so layout, composer, and visual
   viewport reflow together. Keep the existing containment, touch-size,
   overflow, insertion, and focus assertions.
3. Capture and inspect fresh phone evidence, then run the focused and sibling
   shared-menu checks.

## Verification

```bash
cd apps/web && pnpm test -- components/task/chat/popup-menu.test.tsx
cd apps/web && pnpm e2e:run --project mobile-chrome \
  tests/chat/mobile-prompt-mention-composer.spec.ts \
  tests/chat/mobile-entity-reference-composer.spec.ts \
  tests/chat/mobile-slash-command-composer.spec.ts \
  tests/task/mobile-task-create-escape.spec.ts
cd apps/web && pnpm run typecheck
cd apps && pnpm --filter @kandev/web lint
python3 scripts/lint-spec-files.py --all
```

## Dependencies

Task 01 supplies shared visual-viewport containment behavior and unit coverage.
This remediation changes browser modeling and durable adjacency coverage, not
production geometry.

## Parallelism

Sequential in the primary conversation. This work order does not authorize
subagents.

## Inputs

- User screenshot feedback showing the list detached above the composer.
- RED browser measurement: a 136-pixel menu-to-composer gap after replacing
  only `visualViewport.height` while leaving page layout unchanged.
- Existing unit coverage for an anchor below a reduced visual viewport.

## Risks

- Do not remove the raw off-screen-anchor containment regression while fixing
  screenshot evidence.
- Do not claim an operating-system keyboard is present in Playwright. The
  browser runner models its reduced available height by resizing the page.
- Keep capture assets ignored and publish them through PR media, not feature
  branch history.

## Output contract

Report the RED gap, corrected viewport model, permanent adjacency assertion,
fresh inspected screenshot, exact verification results, and PR status. Then
mark this work order done and synchronize the plan.

## Results

- RED: the permanent bounding-box assertion measured a 136-pixel gap between
  the saved-prompt surface and composer under the old occlusion-only viewport
  mock.
- Replaced that invalid visual model with `page.setViewportSize()` at a
  420-pixel height, which reflows the real mobile page, composer, and visual
  viewport together. Production containment geometry did not require change.
- GREEN: the focused scenario passed with direct adjacency, visual-viewport
  containment, a 44-pixel result row, no horizontal overflow, touch insertion,
  and retained composer focus.
- The full targeted mobile shared-consumer run passed five tests covering `@`,
  `#`, `/`, and task-create mention behavior.
- Focused popup unit coverage passed 11 tests. Typecheck, full web lint, and
  specification lint passed.
- Captured and visually inspected
  `apps/web/.pr-assets/mobile-prompt-mention-composer--mobile-composer-prompt-menu.png`;
  the menu is directly attached above the composer.
