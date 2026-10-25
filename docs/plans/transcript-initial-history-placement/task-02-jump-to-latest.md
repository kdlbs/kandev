---
id: "02-jump-to-latest"
title: "Add explicit latest-message navigation"
status: complete
wave: 2
depends_on: ["01-preserve-initial-placement"]
plan: "plan.md"
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
acceptance_criteria:
  - AC-UI-TRANSCRIPT-AUTO-SCROLL-001.18
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
---

# Task 02: Add explicit latest-message navigation

## Summary

Add a visible action that reveals the newest transcript message. Preserve the
session's auto-scroll preference and existing prompt navigation.

## In scope

- A native latest-position handle, below-viewport visibility state, and localized Button.
- Shared desktop/phone placement above the composer, keyboard operation, and focus handling.
- Six-language copy and user documentation in `docs/public/sessions-and-review.md`.

## Out of scope

New settings, backend endpoints, and changes to existing navigation preferences.

## Acceptance

- The action reaches an assistant reply after the last user prompt and preserves enabled or disabled auto-scroll.
- The action appears only with content below the viewport and has localized text and an accessible name.
- Phone/coarse-pointer targets measure at least 44px. The action clears the composer and safe area without horizontal overflow.

## TDD evidence

Add `jump-to-latest-button.test.tsx` for visibility and accessible activation.
Extend the native-list tests with `explicit latest navigation reaches the
transcript end without changing auto-scroll`. Verify old initial callbacks
cannot overwrite the explicit action. Cover disabled preference, grouped output,
streaming growth, empty history, and focus after the button disappears.

## ASCII UI preview

UI-01: Task Chat entry, desktop and phone. The shared transcript is the only
vertical scroll owner. Desktop keeps its Dockview surroundings. Phone keeps the
full-height Chat tab, fixed header, composer, and existing safe-area clearance.

```text
Current failure            Corrected entry         Reader above latest
+--------------------+     +--------------------+  +--------------------+
| Oldest message     |     | Recent conversation|  | Older conversation |
| Earlier history   |     | Latest assistant   |  | Reader's position  |
| ...                |     | reply              |  | ...                |
+--------------------+     +--------------------+  +--------------------+
| Composer           |     | Composer           |  | [Jump to latest]   |
+--------------------+     +--------------------+  | Composer           |
                                                  +--------------------+
```

The action sits outside the scroller above the composer. It disappears at the
bottom and for empty history. During refresh, cached history keeps its intended
position. Cold empty history retains the existing loading state. Phone uses a
44px minimum hit target. Desktop uses the existing compact density. No additional
sheet or navigation step is needed. Exact spacing is illustrative.

Criteria: `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.14`, `.16`, `.17`, and `.18`.

See [the full plan](plan.md#ascii-ui-preview). This work order owns the action in
the reader-above-latest column. The existing last-prompt control remains distinct.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/jump-to-latest-button.test.tsx components/task/chat/message-list-native.test.tsx components/task/chat/scroll-to-last-prompt-button.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/jump-to-latest.spec.ts tests/chat/last-prompt-scroll.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-jump-to-latest.spec.ts tests/chat/mobile-last-prompt-scroll.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run targeted ESLint on changed TS/TSX files. Use phone geometry assertions at
390px, 767px, and 768px, including narrow fine-pointer and coarse-pointer input.
Record a rendered phone screenshot during the same E2E run.

## Files likely touched

- `apps/web/components/task/chat/message-list-shared.tsx`
- `apps/web/components/task/chat/message-list-native.tsx`
- `apps/web/components/task/chat/message-list-native-scroll.ts`
- `apps/web/components/task/task-chat-panel.tsx`
- New `apps/web/components/task/chat/jump-to-latest-button.tsx` and its test.
- `apps/web/components/task/chat/message-list-native.test.tsx`
- New `apps/web/e2e/tests/chat/jump-to-latest.spec.ts`
- New `apps/web/e2e/tests/chat/mobile-jump-to-latest.spec.ts`
- Existing chat namespace catalogs under `apps/web/src/locales/` for all six languages.
- `docs/public/sessions-and-review.md`

## Dependencies

Task 01. Use its cancellation and placement contract.

## Risks

Scroll-to-last-prompt is not a substitute for transcript-end navigation. An
unconditional bottom action must still leave disabled auto-scroll disabled.

## Parallelism

`sequential`

## Inputs

- [Requirement](../../specs/ui/requirements/transcript-auto-scroll.md), criterion `.18`.
- [Design](../../specs/ui/system-design/transcript-auto-scroll.md), explicit latest navigation and responsive behavior.
- Existing `MessageListHandle`, `TranscriptNavGroup`, and full-height mobile Chat composition.

## Results

Added a native Jump to latest handle and visibility tracking that reacts to
scroll and content-size changes. The action sits above the composer, uses a
compact fine-pointer target and a 44px coarse-pointer target, and transfers
focus to the transcript after activation. It cancels stale placement work and
keeps the auto-scroll preference unchanged. Added six-language and pseudo
catalog copy plus the public navigation note.

The focused unit run passed 182 tests and web typecheck passed. The managed Vite
build passed. Desktop Jump to latest passed, as did all 11 existing desktop
last-prompt cases. Phone Jump to latest passed at 390px, 767px, and 768px, with
target size, composer clearance, focus, and horizontal-overflow assertions. The
mobile last-prompt case passed. `i18n:check`, `i18n:ratchet`, targeted ESLint,
public-doc validators, specification validation, and `git diff --check` passed.

Review follow-up: Jump to latest now invalidates any in-flight prompt verifier
and releases its programmatic guard before taking bottom ownership. A
controlled-RAF regression drains the stale verifier and confirms later output
continues to follow the latest position.
