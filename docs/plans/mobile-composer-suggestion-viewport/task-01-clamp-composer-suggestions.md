---
id: "01-clamp-composer-suggestions"
title: "Clamp composer suggestions to the visual viewport"
status: done
wave: 1
depends_on: []
plan: "plan.md"
spec: "../../specs/ui/requirements/composer-suggestion-overlays.md"
system_design: "../../specs/ui/system-design/composer-suggestion-overlays.md"
---

# Task 01: Clamp Composer Suggestions to the Visual Viewport

## Intent

Keep mobile composer suggestion menus visible and selectable when a software
keyboard shrinks the visual viewport but the editor anchor remains in layout
coordinates.

## Acceptance

- `AC-UI-COMPOSER-OVERLAY-001.1`: An above-placement menu whose raw anchor is
  below the visual viewport renders its full surface inside the padded visible
  bounds when the header and a result row fit.
- `AC-UI-COMPOSER-OVERLAY-001.2`: An already-open menu recomputes that contained
  geometry after visual-viewport resize or scroll without retriggering.
- `AC-UI-COMPOSER-OVERLAY-001.3`: The phone browser keeps the popup horizontally
  contained, the document free of horizontal overflow, and each selectable row
  at least 44 CSS pixels tall.
- `AC-UI-COMPOSER-OVERLAY-001.4`: Touching the custom-prompt result inserts it,
  closes the menu, preserves composer focus, and does not send the draft.
- Ordinary desktop and offset-phone geometry remains unchanged. Below placement
  retains its explicit top-edge semantics.
- The unit and browser regressions fail before the production change and pass
  after the smallest geometry correction.

## Files likely touched

- `apps/web/components/task/chat/popup-menu.tsx`
- `apps/web/components/task/chat/popup-menu.test.tsx`
- `apps/web/e2e/tests/chat/mobile-prompt-mention-composer.spec.ts`
- `docs/plans/mobile-composer-suggestion-viewport/plan.md`
- `docs/plans/mobile-composer-suggestion-viewport/task-01-clamp-composer-suggestions.md`

## Steps

1. Add the shrunken-visual-viewport unit case to
   `popup-menu.test.tsx`, annotate its acceptance-criterion coverage, and run it
   to record the expected failure.
2. Add ordinary above- and below-placement assertions that pin existing
   geometry.
3. Change `computePopupMenuStyle` so the rendered vertical edge and available
   height use the same visual-viewport-normalized coordinate. Keep the current
   width, margin, height cap, transform, and event subscriptions.
4. Run the focused unit suite to green and refactor only if the geometry stays
   simpler and deterministic.
5. Add the managed mobile E2E from the diagnostic scenario. Record its red
   geometry failure before relying on the production correction, then run it to
   green.
6. Run the sibling mobile menu scenarios, typecheck, and lint. Record exact
   results in this work order and the plan.

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
```

## Dependencies

None.

## Parallelism

Sequential in the primary conversation. Production geometry and its unit and
browser regressions form one TDD loop. This work order does not authorize
subagents.

## Inputs

- `REQ-UI-COMPOSER-OVERLAY-001` and its four acceptance criteria.
- `docs/specs/ui/system-design/composer-suggestion-overlays.md`.
- Root-cause evidence: popup bottom 551 pixels with a 420-pixel visual viewport
  after typing a matching custom-prompt query.
- Passing focused baseline: 23 tests across popup geometry, TipTap suggestion,
  and custom-prompt hooks.
- Existing mobile browser coverage in
  `mobile-entity-reference-composer.spec.ts`,
  `mobile-slash-command-composer.spec.ts`, and
  `mobile-task-create-escape.spec.ts`.

## Risks

- Use visual-viewport offsets as well as height; an offset viewport can occur
  during zoom or browser chrome movement.
- Calculate maximum height from the normalized rendered edge, not the stale
  caret anchor.
- Preserve short-list anchoring and the plan editor's below placement.
- Keep the E2E cleanup in `finally`; custom prompts are shared fixture data.
- Do not add a hardcoded user-facing string. The production change needs no new
  copy or locale key.

## Output contract

Report the red failure, geometry formula, permanent regression coverage, exact
verification results, affected shared consumers, blockers, and remaining risks.
Then mark this work order done and update the plan checkbox and results.

## Results

- Added acceptance-criterion-linked unit coverage for an above-composer anchor
  left below a software-keyboard visual viewport, live viewport resize, and the
  unchanged below-caret placement path.
- RED: the focused unit suite failed both new containment checks with `552px`
  instead of `412px` (2 failed, 9 passed).
- RED: the permanent mobile saved-prompt E2E measured popup bottom `551px`
  against a `421px` limit and failed the containment assertion.
- Updated `computePopupMenuStyle` to clamp the requested rendered vertical edge
  to the padded visual viewport, then calculate available height from that same
  normalized edge. Existing width, height cap, transform, portal, and viewport
  event subscriptions remain unchanged.
- GREEN: the focused unit suite passed (11 tests).
- GREEN: the rebuilt mobile saved-prompt scenario passed (1 test).
- Shared regression: the rebuilt mobile `@`, `#`, `/`, and task-create mention
  command passed (5 tests).
- `pnpm run typecheck` passed.
- Final full web lint passed with zero warnings after splitting the test groups
  to satisfy the callback line limit.
- PR review remediation qualified the legacy entity-reference anchoring text to
  cover the below-viewport clamp; specification lint passed afterward.
- Managed browser runs removed their artifacts and task-owned instances during
  teardown. No blocker or known remaining risk remains in the work-order scope.
