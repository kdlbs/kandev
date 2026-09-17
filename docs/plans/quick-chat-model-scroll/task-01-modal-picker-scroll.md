---
id: "01-modal-picker-scroll"
title: "Restore modal picker scrolling"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001
acceptance_criteria:
  - AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.7
  - AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.8
system_design:
  - ../../specs/ui/system-design/quick-chat-viewport-layout.md
---

## Summary

Keep model picker content inside its containing dialog's scroll/focus boundary.

## Scope and exclusions

Own the selector trigger reference and portal resolution plus focused component
and desktop/mobile E2E tests. Do not alter model APIs, persistence, shared dialog
locking, or unrelated selectors.

## Acceptance

1. Demonstrate behavioral RED: an overflowing Quick Chat model list fails to
   advance under real wheel input before the fix; then passes after containment.
2. Wheel and touch reveal a supported offscreen model and selection updates the
   trigger; the background and composer remain stable on desktop and phone.
3. Search, keyboard selection, Escape focus return, viewport containment, and
   non-modal task selector behavior pass their targeted regression checks.

## ASCII UI preview

See the [full plan](plan.md#ascii-ui-preview).

### UI-01: Composer model picker, open

```text
Quick Chat dialog
  [Search models                 ]
  [Model A                       ]
  [Model B                       ]  <- model list scrolls
  [More models below             ]
  [Provider options              ]
  [Composer text                 ]
  [Model v]                 [Send]
```

Desktop and phone retain this control hierarchy. Desktop is anchored above the
composer; phone uses the existing viewport-contained picker in the full-height
dialog with coarse-pointer targets. Search and provider options are outside the
model scroll region. Structure and scroll ownership are required; spacing and
labels are illustrative. Existing localized copy remains authoritative.
Maps to AC-UI-QUICK-CHAT-VIEWPORT-LAYOUT-001.7 and .8.

## Verification

Run from repository root. Install dependencies once in this fresh worktree.
Build the local runtime and frontend explicitly, then use the guarded E2E runner
with those fresh artifacts. The default runtime build also cross-compiles remote
helpers unrelated to these host-only tests; omit that target. Use a writable Go
cache when the default cache is read-only. Run browser RED before production edits.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/model-config-selector.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/model-config-selector.tsx components/model-config-selector.test.tsx e2e/tests/chat/quick-chat-model-scroll-helpers.ts e2e/tests/chat/quick-chat-model-scroll.spec.ts e2e/tests/chat/mobile-quick-chat-model-scroll.spec.ts)
GOCACHE=/tmp/kandev-model-scroll-go-cache make -C apps/backend -o build-agentctl-remote build e2e-plugin-package
(cd apps/web && pnpm build:e2e)
(cd apps/web && pnpm e2e:run --host --no-build --project chromium -- tests/chat/quick-chat-model-scroll.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome -- tests/chat/mobile-quick-chat-model-scroll.spec.ts tests/chat/mobile-model-selector.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/components/model-config-selector.tsx`
- `apps/web/components/model-config-selector.test.tsx`
- `apps/web/e2e/tests/chat/quick-chat-model-scroll.spec.ts` (new)
- `apps/web/e2e/tests/chat/quick-chat-model-scroll-helpers.ts` (new shared test helpers)
- `apps/web/e2e/tests/chat/mobile-quick-chat-model-scroll.spec.ts` (new)
- This work order and `plan.md` for results/status.

## Dependencies

None. Use existing `quick-chat-helpers.ts`, test-base fixtures, and the long-menu
E2E store seeding pattern in `mobile-model-selector.spec.ts`.

## Risks

Dialog positioning, focus restoration, and late model hydration. Do not use
forced clicks or direct scrollTop assignment to bypass failing behavior.

## Parallelism

Sequential.

## Inputs

- [Requirements](../../specs/ui/requirements/quick-chat-viewport-layout.md)
- [Design](../../specs/ui/system-design/quick-chat-viewport-layout.md#composer-model-popup-containment)
- Source evidence and test matrix in [plan](plan.md).

## Results

- RED: component containment test failed (17 passed, 1 failed); Chromium wheel
  regression failed with expected scrollTop > 0, actual 0 on an overflowing list.
- GREEN: component suite passed all 18 tests. Typecheck and targeted ESLint passed.
- Local runtime/plugin package and production E2E frontend builds passed.
  A writable Go cache and explicit `-o build-agentctl-remote` avoided the
  default read-only cache and unnecessary cross-platform compilation.
- Mobile E2E passed both tests, including real touch scrolling to Mock Smart,
  selection, stable composer/background, and viewport containment. Bounds allow
  one CSS pixel for fractional device-pixel rounding; document overflow remains
  checked against the viewport width without that tolerance.
- Desktop E2E passed 3 tests: wheel/selection, search/keyboard/Escape, and the
  non-modal task selector. Final shared-helper verification passed all 3 tests with retries disabled.
- E2E used one worker and isolated fixture data. Elevated execution was required
  to bind the local backend port; fixtures cleaned up their owned runtimes.
- Specification catalog, specification lint, and diff whitespace checks passed.
- Public docs need no change: existing interaction restored with no copy,
  navigation, configuration, or API changes.
