---
id: "01-own-opening-touch"
title: "Own the opening touch"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-TASK-NAVIGATION-001
acceptance_criteria:
  - AC-UI-MOBILE-TASK-NAVIGATION-001.3
  - AC-UI-MOBILE-TASK-NAVIGATION-001.11
  - AC-UI-MOBILE-TASK-NAVIGATION-001.12
system_design:
  - ../../specs/ui/system-design/mobile-menu-backdrops.md
---

# Task 01: Own the opening touch

## Summary

Keep the native Sessions action menu inspectable after opening by touch. A
separate explicit selection retains the existing captured row action once.

## In scope

Local touch trigger ownership, real-Radix regression tests, native phone
browser RED/GREEN, compatibility and focused checks.

## Out of scope

Backend/API, lifecycle eligibility, shared primitives, rename, bulk removal,
new confirmations, timers and extra workers/tasks.

## Acceptance

1. Actual opening/dismissal touches emit zero lifecycle, primary, deletion or
   handoff requests; selected row/picker stay correct.
2. Separate explicit item selection invokes the right session action once with
   successful real backend readback; cancelled touch does not open.
3. Existing keyboard/mouse/pen, pending/disabled, delete and focus handoff,
   viewport containment and 44px touch targets remain usable.

## ASCII UI preview

UI-01: Native Sessions picker, selected secondary row, action menu.

```text
Phone: Sessions drawer           Touch completes -> inset menu
[Primary session          ...]   [Set as primary             ]
[Selected session         ...]   [Stop                       ]
                                [Restart session            ]
                                [Delete                     ]
                                [Handoff                  > ]
Tablet/desktop: existing session tabs and anchored menus (unchanged)
```

Before: opening touch can also select Restart. After: opening leaves the menu
inspectable; a separate item gesture chooses the action. Order above is
illustrative; eligibility and localized labels remain authoritative. Reuse
MobileSessionsPicker/MobilePickerSheet and shared menu-bottom-sheet styling:
one menu scroller, existing safe area and 70dvh cap, 44px phone/coarse-pointer
hit targets. Drawer selection remains the primary action. No layout changes.
This native picker is phone-only in task-layout; tablet and
desktop retain their existing session-tab composition. Real-Radix tests cover
mouse/pen at a wider viewport, and the phone browser covers hybrid mouse input.
Maps to AC-UI-MOBILE-TASK-NAVIGATION-001.3, .11, .12.

See [combined preview](plan.md#ascii-ui-preview).

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/mobile/mobile-sessions-section.test.tsx)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 pnpm e2e:run --host --shards 1 --project mobile-chrome e2e/tests/session/mobile-session-menu-gesture.spec.ts e2e/tests/session/mobile-session-tab-restart.spec.ts e2e/tests/session/mobile-session-deletion.spec.ts --retries=0)
(cd apps/web && pnpm exec eslint components/task/mobile/mobile-sessions-section.tsx components/task/mobile/mobile-sessions-section.test.tsx components/task/mobile/use-session-menu-opening-gesture.ts e2e/tests/session/mobile-session-menu-gesture.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run repository PR-documentation coverage preflight before completion.

## Files likely touched

- `apps/web/components/task/mobile/mobile-sessions-section.tsx`
- `apps/web/components/task/mobile/use-session-menu-opening-gesture.ts`
- `apps/web/components/task/mobile/mobile-sessions-section.test.tsx`
- `apps/web/e2e/tests/session/mobile-session-menu-gesture.spec.ts`
- `docs/specs/ui/requirements/mobile-task-navigation.md`
- `docs/specs/ui/system-design/mobile-menu-backdrops.md`

## Dependencies

None. Existing installed Radix and disposable managed E2E fixture only.

## Risks

Native touch timing and enclosing Drawer focus ownership; do not substitute
mouse clicks or mock lifecycle browser requests.

## Parallelism

`sequential`

## Inputs

Owning AC-UI-MOBILE-TASK-NAVIGATION-001.3/.11/.12, linked system design, preserved
attempt16-source diagnostics. User-authorized autonomous planning/TDD exception;
parent review checkpoint before push/PR remains mandatory.

## Results

All commands above passed after the keyboard review correction. Real-Radix
consumer tests: 27/27 (3.80s). The original touch RED remains recorded as
4 failures/20 passes; additional Tab/ArrowRight/unrelated-key regressions fail
3/3 before narrowing the propagation guard. Enter/Space/ArrowDown continue to
open the menu without selecting its enclosing row.

Fresh managed production browser package: 5/5 (53.1s), including the native touch
mutation gate and all existing restart/deletion cases. Retries 0, one worker/shard,
strict gateway accounting and bounded Go build settings remain active.

Scoped ESLint has no warnings/errors. Typecheck passes with 8GB heap. Catalog,
full spec lint, PR-documentation coverage, and diff checks pass. The default-heap
OOM and initial setup failures remain historical, separately recorded outcomes.

Full gateway history, positive backend/readback evidence, public docs assessment,
compatibility limits and receipt paths are in the [plan results](plan.md#verification-results).
Parent validation and subsequent delivery authorization completed; final-head
CI/review/screenshot/merge evidence is maintained externally until terminal.
