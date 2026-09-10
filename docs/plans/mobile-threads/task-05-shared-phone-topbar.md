---
id: "05-shared-phone-topbar"
title: "Normalize phone listing headers"
status: done
wave: 5
depends_on:
  - 04-swipe-position-feedback
plan: "plan.md"
requirements:
  - REQ-UI-MOBILE-QUICK-CHAT-TOPBAR-001
  - REQ-UI-THREADS-DECK-003
  - REQ-UI-QUICK-CHAT-IDLE-DOT-001
acceptance_criteria:
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.1
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.2
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.3
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.4
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.5
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.6
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.7
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.8
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.9
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.10
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.11
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.12
  - AC-UI-MOBILE-QUICK-CHAT-TOPBAR-001.13
  - AC-UI-THREADS-DECK-003.10
  - AC-UI-THREADS-DECK-003.12
  - AC-UI-QUICK-CHAT-IDLE-DOT-001.5
system_design:
  - ../../specs/ui/system-design/mobile-quick-chat-topbar.md
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 05: Normalize phone listing headers

## Summary

Apply one compact phone header to Kanban, List, and Threads. Generalize the
existing Threads menu actions so moving controls does not remove functionality
or change workspace, listing, or plugin semantics.

## In scope

- Shared 56-pixel phone topbar, stacked context control, fixed ghost menu, and
  unchanged inline Threads pagination; remove the phone brand/action strip.
- Kanban/List workspace-and-mode context buttons opening the existing menu;
  keep the Threads saved-view control and drawer.
- Generalize menu launchers, preserve search reveal/clear, workspace-aware
  Home, list/board display controls, plugin props, metrics, connection status,
  and Quick Chat activity feedback. Preserve focus from both menu openers.
- Migrate all affected mobile entry-point tests to visible menu controls.
- Update affected public how-to sections in `developer-tools.md`,
  `tasks-and-workflows.md`, and `sessions-and-review.md` where needed. No
  speculative default-Home instructions. Reuse existing localization keys
  where possible; new copy requires every supported catalog.
- Refresh and inspect the existing private evaluation instance without
  reseeding it or altering unrelated Tailscale services.

## Out of scope

Tablet/desktop headers, task-session navigation, new saved views for Kanban or
List, backend/settings changes, explicit Home defaults, plugin API changes,
and agent/session lifecycle changes.

## Acceptance

1. All three phone modes share header geometry and visible context/menu
   hierarchy at 360 and 393 pixels, without horizontal overflow or a permanent
   extra row. Their primary context actions and Threads indicator work.
2. Every relocated action remains touch-reachable and workspace-correct;
   search focuses/clears correctly, Home preserves routing, activity/status
   remain visible, menu/dialog handoff and focus return work, and enabled
   metrics/plugins fit inside the scrolling drawer.
3. Existing listing preferences, tablet/desktop composition, phone content
   scrolling, drafts, and session selection remain unchanged.

## Verification

RED: parameterize `kanban-header-mobile.test.tsx` over Kanban, List, and
Threads, asserting the shared title slot and absence of the strip. Replace the
old fixed-brand browser expectations with a three-mode geometry/navigation
regression before changing production. Add focused menu action tests for
page-specific plugin props, both openers' focus return, close-before-launch,
search clearing, missing workspace, and simultaneous activity/connection cues.

Update the existing mobile search/chat/terminal tests instead of deleting
their outcome assertions. Search for every old launcher selector and migrate
shared helpers as well as direct test calls. Keep the tests' shared-setting
capture/restore isolation.

From the repository root (install dependencies first only in a fresh worktree):

```bash
cd apps/web
pnpm test components/kanban/ lib/task-listing/ components/threads/ app/threads/threads-page-client.test.tsx
pnpm test components/quick-chat/quick-chat-focus.test.ts components/quick-chat/quick-chat-provider-focus.test.tsx hooks/use-quick-chat-launcher.test.ts hooks/use-quick-terminal-launcher.test.ts
pnpm run typecheck
pnpm exec eslint components/kanban/ --max-warnings=0
pnpm run i18n:check
cd ../..
make build-web
cd apps/web
pnpm e2e:raw tests/kanban/mobile-kanban-topbar.spec.ts tests/plugins/mobile-plugin-topbar.spec.ts tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/task/mobile-task-list-search.spec.ts tests/task/mobile-task-listing-display.spec.ts --project=mobile-chrome --retries=0
pnpm e2e:raw tests/chat/mobile-quick-chat-entry.spec.ts tests/chat/mobile-quick-chat-tabs.spec.ts tests/chat/mobile-quick-chat-idle-dot.spec.ts tests/chat/mobile-quick-chat-saved-prompt-delivery.spec.ts tests/terminal/mobile-quick-terminal.spec.ts tests/layout/mobile-app-status-bar.spec.ts --project=mobile-chrome --retries=0
pnpm e2e:raw tests/task/threads-view.spec.ts tests/chat/quick-chat-saved-prompt-delivery.spec.ts --project=chromium --retries=0
pnpm e2e:raw tests/kanban/mobile-kanban.spec.ts tests/mobile-zoom.spec.ts --project=mobile-chrome --retries=0 --grep 'renders focused mobile layout|search toggle reveals|collapsing search clears|opening mobile menu does not focus|mobile search bar filters|form fields render'
cd ../..
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

Run guarded E2E commands sequentially, never pass a literal `--` before their
arguments, and rebuild before browser runs. Include a focused 820-pixel
coarse-pointer regression in the shared header tests. Run scoped formatting
and lint on any additional changed files. Check dark 360/393-pixel screenshots
of all three modes and opened menus, then verify the existing demo HTTPS and
WebSocket route. Do not call the old 22-test result proof of this follow-up.

## Files likely touched

- `apps/web/components/kanban/kanban-header-mobile.tsx` and its tests
- `apps/web/components/kanban/kanban-header.tsx` if a phone-only input is needed
- `apps/web/components/kanban/mobile-threads-menu-actions.tsx` (generalize name)
- `apps/web/components/kanban/mobile-menu-sheet.tsx`
- `apps/web/components/kanban/mobile-search-bar.tsx` only for focus/close wiring
- `apps/web/components/quick-chat/quick-chat-focus.ts` and its regression tests
- `apps/web/hooks/use-quick-chat-launcher.ts` and `use-quick-terminal-launcher.ts`
  for explicit persistent-opener focus through the existing focus owner
- `apps/web/components/threads/threads-view-controls.tsx` for shared appearance
- Existing mobile topbar, plugin, search, chat, terminal, and status E2Es above
- `apps/web/e2e/tests/chat/quick-chat-saved-prompt-delivery-helpers.ts`
- Applicable locale catalogs, scoped guidance, and public how-to docs

## Dependencies

Task 04; preserve its direct, scroll-derived pagination signal while composing
the shared header. No dependency on the queued default-Home subtask.

## Risks

Missing Home after wordmark removal, hidden metrics with status disabled,
incorrect plugin `currentPage`, focus restored into an unmounted menu, duplicated
search inputs, hidden Quick Chat activity, and accidentally changing tablet
menus through shared components. Tests must retain those capabilities.

## Parallelism

`sequential`

## Inputs

- Mobile Workspace Topbar requirement and paired system design.
- Threads responsive design and Task 04's completed implementation.
- Quick Chat activity requirements, existing launcher hooks, PageTopbar, and
  MobileMenuSheet. Use the navigation manifest, not a second destination list.

## Results

Implemented the shared context control and quiet 56px phone header, generalized
menu actions, search reveal/clear, plugin identity, opted-in metrics, Home, and
visible connection/activity cues. Permanent unit and browser checks reproduced
the old composition before implementation. Tablet/desktop composition remains
unchanged.

The terminal close regression found a detached menu-row focus target. Added
an optional persistent-opener ref through the existing Quick Chat focus owner;
both unit and browser assertions failed on the missing focus return before
the fix. Targeted focus, launcher, plugin, and header tests now pass. Combined
Tasks 04-05 verification passed 319 distinct unit and 46 browser tests (33
mobile, 13 desktop), with one browser worker and retries disabled. The final
chat/terminal run also passed with tracing enabled. The layout test submits
once instead of replaying an accepted send; terminal cleanup waits for the
settled menu state.

Typecheck, zero-warning scoped lint, formatting, i18n checks/ratchet, web build,
30 spec-linter tests, specification lint, and public-docs validation passed.

The existing demo was refreshed without reseeding. At 360px and 393px, all
three headers measured 56px high with 44px menu buttons and no document
overflow. The final held touch swipe reported 2/4 at 65.8% scroll before release, with
one active chat after snap. Dark captures are in the runtime directory.
HTTPS health and TLS WebSocket upgrade passed. The evaluation backend and
private route remain running; the owned inspection browser is closed.
