---
id: TASK-CHAT-MOTION-01
title: Text motion and appearance preference
status: done
wave: 1
depends_on: []
plan: plan.md
requirements:
  - REQ-UI-CHAT-MOTION-001
  - REQ-UI-CHAT-MOTION-002
acceptance_criteria:
  - AC-UI-CHAT-MOTION-001.1
  - AC-UI-CHAT-MOTION-001.2
  - AC-UI-CHAT-MOTION-001.3
  - AC-UI-CHAT-MOTION-001.4
  - AC-UI-CHAT-MOTION-002.1
  - AC-UI-CHAT-MOTION-002.2
  - AC-UI-CHAT-MOTION-002.3
  - AC-UI-CHAT-MOTION-002.4
system_design:
  - ../../specs/ui/system-design/chat-motion.md
---

# Text motion and appearance preference

## Summary and scope

Deliver live text/row motion and the persisted Appearance control end to end.
Read the paired requirements/design and root/web AGENTS.md before TDD.
Exclude scrolling mechanics (work order 02), backend settings, and non-chat
Markdown behavior. The final scrolling promise is completed by dependent 02.

## Likely files

- Proposed `apps/web/lib/settings/chat-motion.ts`, its test, and
  `lib/state/slices/ui/chat-motion-actions.ts` with tests; existing settings
  constants, `ui-slice.ts`, UI types, and `app-state-types.ts` for wiring.
- Proposed `apps/web/hooks/use-chat-motion.ts` and `.test.tsx`; existing
  `components/settings/appearance-settings-state.ts`, its test,
  `general-settings.tsx`, its test, and settings-discovery preferences catalog.
- Existing `components/task/chat/message-list-native.tsx`,
  `message-list-shared.tsx`, and message components; proposed chat arrival helper
  and `chat-motion.test.tsx`. Existing `components/shared/memoized-markdown.tsx`
  and tests; proposed `lib/markdown/chat-text-motion.ts` and `.test.ts`.
- `apps/web/app/globals.css`, supported `src/locales/*/settings.json`, and
  proposed `e2e/tests/chat/chat-motion.spec.ts` and `mobile-chat-motion.spec.ts`.

All abbreviated paths above are relative to apps/web. Keep new helpers narrow,
use existing Markdown dependencies, and do not enlarge already oversized files
with inline algorithms. Run the Traditional Chinese converter, not hand edits.

## Implementation acceptance

1. Default/storage, Appearance preview/save/cancel/rebase, OS precedence, and
   localized/discoverable switch pass unit and desktop/phone tests.
2. New inline suffixes and newly delivered rows animate once; old text, grouped
   updates, historical/refetched/remounted content, and rewritten Markdown do
   not replay. Unicode, copy, links, and comment selection remain correct.
3. Finite effects cancel on disable/hide/unmount; bursts do not create unbounded
   spans or effect queues. Record desktop/phone visual evidence and a stream trace.

## ASCII UI preview

### UI-01: Appearance, enabled and disabled (desktop and phone)

Entry: Settings > Preferences > Appearance. Existing navigation is retained.

```text
Appearance
  Rich output animations                [on]
  Chat animations                       [on]
  Animate incoming text, new chat items,
  and scrolling on this device.
  Respects reduced motion.
                              [Reset] [Save]

Saved off: Chat animations               [off]
OS reduced motion: saved switch stays on;
                   effective motion is off.
```

Both viewports retain this control order. Phone help text wraps below the
label; the switch has a 44 px hit area. Existing settings page owns scrolling
and Save/Reset placement. Spacing/copy are illustrative; control semantics,
localization, hierarchy, and touch reachability are required.

### UI-02: Live chat (shared content order, existing phone full-height layout)

```text
[Session header / navigation]                 fixed
[Existing transcript text]                    scrolls
[Assistant: existing text + incoming suffix]   suffix fades
[New tool row]                                enters once
[Composer and existing navigation controls]    outside scroller
```

Phone shows one full-height Chat surface; desktop retains its Dockview panel.
Only transcript content scrolls; composer and safe-area geometry are unchanged.
History/loading restoration and motion-off states use the same layout with
immediate content. No new empty/error state is introduced. UI-01 maps to
AC-UI-CHAT-MOTION-002.1 through .4; UI-02 maps to the 001 and 003 criteria.
The work-order E2E suites prove the rendered controls and motion states.

## Targeted verification

From repository root; initial install is needed only if dependencies are absent:

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run lib/settings/chat-motion.test.ts lib/state/slices/ui/chat-motion-actions.test.ts hooks/use-chat-motion.test.tsx lib/markdown/chat-text-motion.test.ts components/task/chat/chat-motion.test.tsx components/shared/memoized-markdown.test.tsx components/shared/chat-markdown-motion.test.tsx components/settings/appearance-settings-state.test.ts components/settings/general-settings.test.tsx lib/settings-discovery/catalog.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/chat-motion.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-chat-motion.spec.ts)
git diff --check
```

Add behavioral RED tests before production code. E2E covers live append versus
history/remount, grouped new tools, preference reload/cancel, runtime reduced
motion, real finite opacity, text selection, and mobile control hit testing.
Use causal stream waits and finite animation completion, never fixed sleeps.
Run affected existing comment-selection tests if changing the selection boundary.

## Dependencies and risks

No work-order dependency. Depends on existing Appearance transaction and native
message list. Risk: source-offset mismatch after Markdown normalization; use
static fallback and explicit rewrite cases. Settings save must preserve new
edits made while saving. Structural previews above match the combined plan.

## Results

Implemented the per-device default-on preference, Appearance preview/Save/Reset,
localized discovery/copy, live reduced-motion policy, prose suffix fades,
new-row entrances, and selection-safe finite text runs.

Validation completed on 2026-09-15:

- The combined targeted run passed 198 tests across 18 files.
- Final lint cleanup and selection/comment regression rerun passed 94 tests
  across 5 files; changed TypeScript files lint cleanly.
- TypeScript, i18n checks (all five supported languages plus pseudo), public
  documentation validation (46 pages), and specification validation passed.
- Desktop and phone settings/live-motion scenarios passed on final assets.
  Phone suite: all 9 tests passed. Screenshots of both layouts were inspected.
- Browser tests measure real intermediate opacity, bounded scroll settling,
  wheel/touch interruption, and disabled/reduced-motion behavior. History readiness
  and finite entrance completion are causal waits; no fixed sleeps were added.
- No commit or push performed.
