---
id: "02-render-grid"
title: "Render the conversation grid"
status: done
wave: 2
depends_on:
  - "01-persist-presentation"
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-004
acceptance_criteria:
  - AC-UI-THREADS-DECK-004.1
  - AC-UI-THREADS-DECK-004.2
  - AC-UI-THREADS-DECK-004.3
  - AC-UI-THREADS-DECK-004.4
  - AC-UI-THREADS-DECK-004.5
  - AC-UI-THREADS-DECK-004.6
  - AC-UI-THREADS-DECK-004.7
  - AC-UI-THREADS-DECK-004.8
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 02: Render the conversation grid

## Summary

Render the saved layout using stable task tiles, with two live desktop/tablet
rows and the existing one-chat phone composition. Preserve selection, draft
identity, scrolling, and viewport-owned session delivery.

## In scope

- Pass effective layout from the page and add the proposed pure
  `thread-layout.ts` helper plus board content-size measurement.
- Preserve direct task children and keys; add Grid tracks, odd/single-task
  handling, height fallback, lower-row navigation, and layout-aware recovery.
- Refresh activation on layout changes, reject stale observer callbacks, and
  prove both visible rows without increasing offscreen membership/streams.
- Localize the fallback explanation and preserve existing task actions.

## Out of scope

Composer disclosure and the Display editor/shortcut, owned by Tasks 03 and 04.
Do not change platform traffic contracts, task ordering, or admission limits.

## Acceptance

1. Columns/Grid match UI-01/UI-02, including odd/single/empty cases and the
   300px row-height fallback; saved preferences survive responsive changes.
2. Switching layouts or removing a lower-row task retains the deterministic
   reader/session/draft and existing task navigation behavior.
3. Both visible rows load selected conversations; 30-shell tests stay bounded,
   offscreen chats release, and phone transitions retain one active detail.

## ASCII UI preview

UI-02: Grid at a sufficient desktop height, excerpt from
[full previews](plan.md#ui-02-grid-auto-hide-enabled-no-active-draft).

```text
+--------------+--------------+--------------+
| A     [Open] | C     [Open] | E     [Open] |
| transcript  | transcript   | transcript   |
+--------------+--------------+--------------+
| B     [Open] | D     [Open] | F     [Open] |
| transcript   | transcript   | transcript   |
+--------------+--------------+--------------+
```

Task 02 retains normal composers; Task 03 supplies the collapsed footer shown
in the combined preview. The task order is A, B, C, D, E, F.
UI-05 phone excerpt:

```text
[Threads / View] [2/6] [Menu]
[Task B v]            [Open]
[one live conversation     ]
[normal composer           ]
```

The phone uses swipe/picker navigation. These structures cover all assigned
layout criteria. The board scrolls horizontally; each transcript owns its
vertical content scroll. Grid tracks must not stretch when content grows.

## Verification

From the repository root, after Task 01 bootstrap. Add behavior-specific RED
unit/browser assertions before implementation. New files below are planned.

```bash
(cd apps/web && pnpm exec vitest run components/threads/thread-layout.test.ts components/threads/threads-board.test.tsx components/threads/thread-column-activation.test.tsx components/threads/use-thread-selection-recovery.test.tsx components/threads/use-mobile-thread-position.test.tsx lib/threads/thread-selection-fallback.test.ts app/threads/threads-page-client.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm exec eslint components/threads/threads-board.tsx components/threads/thread-column.tsx components/threads/thread-layout.ts components/threads/use-thread-column-activation.ts components/threads/use-thread-selection-recovery.ts app/threads/threads-page-client.tsx --max-warnings 0)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/task/threads-layouts.spec.ts tests/task/threads-view.spec.ts tests/task/threads-task-actions.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts -- --retries=0)
git diff --check
```

Use the managed runner's builds and memory limits. Confirm discovery and
capture rendered UI-01/UI-02 at wide desktop, short height, 767/768px, and a
coarse-pointer tablet. API-seeded preferences are fixture setup until Task 04
exposes controls; assertions must read and interact with the rendered board.

## Files likely touched

- `apps/web/app/threads/threads-page-client.tsx`
- `apps/web/components/threads/threads-board.tsx`, `thread-column.tsx`,
  `use-thread-column-activation.ts`, and `use-thread-selection-recovery.ts`.
- New `apps/web/components/threads/thread-layout.ts` and its test.
- New `use-thread-selection-recovery.test.tsx` beside the hook; existing
  board/activation/page tests named above.
- New `apps/web/e2e/tests/task/threads-layouts.spec.ts`; reused presentation
  setup may live in new `threads-presentation-helpers.ts`.
- `apps/web/src/locales/*/threads.json` for the height fallback.
- Existing desktop/task-action/mobile specs only when fixture expectations
  need the explicit default layout; retain their scenarios.
- `apps/web/components/threads/AGENTS.md` for layout and reflow ownership.

## Dependencies

Task 01. Layout consumes its normalized effective presentation.

## Risks

Pair wrappers or index-based React keys remount editors. An observer can
deliver old-layout geometry after resize. Both rows share horizontal offsets,
so recovery must retain task identity instead of inferring a unique task from
an x-coordinate. Full editors need bounded row sizing.

## Parallelism

sequential

## Inputs

- [Layout requirements](../../specs/ui/requirements/threads-conversation-deck.md#req-ui-threads-deck-004-conversation-layouts)
- [Layout design](../../specs/ui/system-design/threads-conversation-deck.md#layout-composition)
- Existing board, viewport activation, stable order, primary-session E2E,
  thread task actions, and mobile swipe test patterns.

## Results

PR #3626 follow-up: observer rebuilds measure current visibility, height
fallback text no longer changes board allocation, wheel interaction records
the reader anchor, and deep-link scrolling honors reduced motion. Final
commands and results are in the [review remediation record](plan.md#pr-review-remediation-2026-09-12).
The original implementation evidence below remains historical.

Done, 2026-09-11.

- Behavioral RED: layout helper returned Columns for Grid, stale observer
  delivery changed active details, and browser geometry remained single-row.
  The real-page test additionally caught the native boot field omission
  recorded in Task 01, resize-generated scroll/live snapshots replacing the
  reader before responsive commit, and layout/width reflow interrupting
  deep-link scroll.
- GREEN: the seven required Vitest files pass 62 tests. Task shells retain
  identity, both rows activate, 30-shell preload/stream windows stay bounded,
  lower-row horizontal anchors survive reflow, and user wheel input retires
  the initial deep-link mark. Typecheck and the exact ESLint command pass.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, and `git diff --check` pass.
  Traditional Chinese and pseudo catalogs were generated; unrelated generator
  changes were removed. No new dependencies or non-test runtime instances were
  added. Updated the scoped engineering guide's grid and reflow ownership.
- First rebuilt desktop run: 19 passed, one deep-link failure. Corrected the
  reflow handoff. Final desktop command below passes all 20 cases (3.7 minutes)
  against the final same-membership snapshot guard, with retries disabled.
  The guard's 62 unit cases, typecheck, lint, and localization ratchet pass too.
- Final mobile command: 10 tests passed with retries disabled (1.6 minutes),
  after `pnpm run build:e2e`. Both phone-to-tablet selection and an immediate
  resize after a deep link are covered. Inspected phone and coarse-tablet
  screenshots: one phone chat, two tablet rows, retained lower-row selection,
  and 44px Open task touch targets. This is emulator evidence, not a physical
  keyboard/device check.

Final browser commands ran sequentially from `apps/web` against the final
production change, with one managed worker and no retries. The backend and
plugin fixtures were built by the earlier managed run; the final change was
frontend-only.

```bash
pnpm run build:e2e
pnpm e2e:run --no-build --project chromium tests/task/threads-layouts.spec.ts tests/task/threads-view.spec.ts tests/task/threads-task-actions.spec.ts -- --retries=0
pnpm e2e:run --no-build --project mobile-chrome tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts -- --retries=0
```
