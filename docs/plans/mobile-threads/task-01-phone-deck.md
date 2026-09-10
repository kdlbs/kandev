---
id: "01-phone-deck"
title: "Improve the phone conversation deck"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-003
acceptance_criteria:
  - AC-UI-THREADS-DECK-003.8
  - AC-UI-THREADS-DECK-003.9
  - AC-UI-THREADS-DECK-003.10
  - AC-UI-THREADS-DECK-003.11
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 01: Improve the phone conversation deck

## Summary

Give phone conversations their available width and a discoverable thread picker.
Reduce header competition while preserving session and utility actions.

## In scope

- Phone Threads topbar, column composition, and shared menu action slot.
- Translations and focused mobile interaction/geometry regressions.
- A fresh mock-data instance for user evaluation after verification.

## Out of scope

- Desktop redesign, new APIs, session lifecycle, publication, and real agents.

## Acceptance

- Each phone chat fills the deck; prose and composer fit at narrow widths.
- Title/count picker changes the visible chat; swipe and session selection work.
- Header controls remain reachable and a seeded branch instance is provided.

## Verification

```bash
make build-web
make -C apps/backend build e2e-plugin-package
cd apps/web
pnpm test components/threads/ lib/threads/
pnpm test components/kanban/kanban-header-mobile.test.tsx components/kanban/mobile-menu-sheet.test.tsx components/kanban/mobile-menu-utility-actions.test.tsx components/kanban/main-top-bar-plugin-actions.test.tsx components/kanban/mobile-topbar-action-strip.test.tsx
pnpm run typecheck
pnpm run i18n:check
pnpm e2e:raw tests/task/mobile-threads-view.spec.ts --project=mobile-chrome --retries=0
pnpm e2e:raw tests/task/threads-view.spec.ts --project=chromium --retries=0
pnpm e2e:raw tests/kanban/mobile-kanban-topbar.spec.ts tests/plugins/mobile-plugin-topbar.spec.ts --project=mobile-chrome --retries=0
```

## Files likely touched

- `apps/web/components/threads/`
- `apps/web/components/kanban/kanban-header-mobile.tsx`
- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/components/kanban/mobile-threads-menu-actions.tsx`
- `apps/web/app/threads/threads-page-client.tsx`
- `apps/web/src/locales/*/threads.json`
- `apps/web/e2e/tests/task/mobile-threads-view.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-topbar.spec.ts`

## Dependencies

None. Install workspace dependencies in this fresh worktree before checking.

## Risks

Keep picker navigation separate from session selection and read-cursor state.

## Parallelism

`sequential`

## Inputs

Threads requirements/design, existing mobile column navigator and picker sheet.

## Results

- RED: the baseline phone column measured about 334 pixels in a 393-pixel
  viewport and failed the full-width assertion. A separate menu regression
  exposed missing Home/tool access after compacting the topbar.
- GREEN: 187 unit tests and 22 browser tests passed. Browser tests cover picker
  selection and dismissal focus, real touch paging, one active chat, 360-pixel
  containment, saved views, session switching, Quick Chat/terminal launchers,
  plugin reachability, and unchanged desktop behavior.
- Web build, typecheck, scoped lint, i18n checks, specification checks, and
  public documentation validation passed.
- A disposable mock instance was evaluated on loopback port 48490, with
  four fictional tasks and five sessions, then stopped at the user's request.
  The manual send check received a normal mock reply. The demo backend and
  private route are stopped; its database and captures were retained locally.
- No production credentials, personal instance, real provider calls, commit,
  push, or publication were used. Real-device keyboard/Safari checks remain
  outside this emulated-browser verification.
