---
id: TASK-CHAT-MOTION-02
title: Smooth scrolling and final integration
status: done
wave: 2
depends_on: [TASK-CHAT-MOTION-01]
plan: plan.md
requirements:
  - REQ-UI-CHAT-MOTION-002
  - REQ-UI-CHAT-MOTION-003
acceptance_criteria:
  - AC-UI-CHAT-MOTION-002.3
  - AC-UI-CHAT-MOTION-002.4
  - AC-UI-CHAT-MOTION-003.1
  - AC-UI-CHAT-MOTION-003.2
  - AC-UI-CHAT-MOTION-003.3
  - AC-UI-CHAT-MOTION-003.4
system_design:
  - ../../specs/ui/system-design/chat-motion.md
---

# Smooth scrolling and final integration

## Summary and scope

Integrate effective chat motion with live following and explicit navigation.
Read work order 01 results and the transcript auto-scroll requirement/design.
Exclude new scroll ownership, history placement redesign, and global CSS smooth
scroll. Complete user docs and package validation after integration.

## Likely files

Existing `apps/web/components/task/chat/message-list-native-scroll.ts`, its tests,
`message-list-native.tsx`, `message-list-native.test.tsx`,
`simple/task-chat.tsx`, and `hooks/domains/session/use-session-search.ts`.
Proposed `components/task/chat/chat-scroll-motion.ts` and `.test.ts` isolate
frame scheduling and cancellation. Extend work order 01 desktop/mobile E2E files.
Update `docs/public/sessions-and-review.md` with a short Appearance control section
and reconcile `docs/specs/ui/requirements/transcript-auto-scroll.md` and its
paired design's immediate-pinning language with the new bounded-motion contract.

## Implementation acceptance

1. One retargetable driver follows live growth within the specified settling
   bound, preserving manual input, auto-scroll off, anchors, and programmatic
   locks. Restoration is instant and commit-time geometry reads stay absent.
2. Motion off/OS reduced motion cancels all new effects and uses immediate chat
   navigation; session toggles stay independent. Desktop and phone pass the
   same behavior, including touch interruption and keyboard viewport resizing.
3. Public docs describe the shipped default, per-device save/cancel behavior,
   reduced-motion precedence, and distinction from auto-scroll/rich-output
   settings. All package criteria have recorded results and matching designs.

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

From repository root (dependencies installed by work order 01):

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/chat-scroll-motion.test.ts components/task/chat/use-chat-scroll-motion.test.tsx components/task/chat/message-list-native-scroll.test.ts components/task/chat/message-list-native.test.tsx components/task/chat/transcript-auto-scroll.test.ts hooks/domains/session/use-session-search.test.ts components/task/simple/components/topbar-working-indicator.test.tsx components/task/simple/task-chat.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/chat-motion.spec.ts tests/chat/auto-scroll-toggle.spec.ts tests/chat/last-prompt-scroll.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-chat-motion.spec.ts tests/chat/mobile-auto-scroll-toggle.spec.ts tests/chat/mobile-last-prompt-scroll.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Unit RED uses a deterministic frame clock: retarget bursts, logical following
while between positions, input cancellation, late resizes, hidden/session
cleanup, and disable without moving an owned reader position. Browser tests
observe intermediate scroll positions and exact final landing, wheel/touch
interruption, manual navigation during streaming, pagination anchors, and
hidden-panel restoration. Seed overflow so assertions cannot pass trivially.
Keep existing instant placement tests explicit about their motion state.

Record sustained-stream evidence with the new driver: bounded animation count,
no idle RAF loop and no geometry reads in the synchronous message commit path.
Run the work order 01 unit suites again only where integration changes their
owned logic. Synchronize Results and statuses after all required checks pass.

## Dependencies and risks

Depends on TASK-CHAT-MOTION-01's effective motion hook and tests. Existing
scroll-write sites have different ownership: do not replace every bottom write.
Scrollbar/manual events and driver-generated events must be distinguishable;
false near-bottom changes would strand a following transcript. Existing
work-start policy is retained. No new global preference coupling is permitted.

## Results

Implemented a single retargetable frame driver, ResizeObserver growth handling,
manual-input cancellation, navigation ownership and motion-off behavior. Initial
placement, history pagination, and layout restoration retain immediate writes.

Validation completed on 2026-09-15:

- Combined targeted unit run: 198 tests passed across 18 files.
- Final native scroll/settings/selection regression run: 94 tests passed across
  5 files. Tests prove one queued frame, no synchronous request geometry reads,
  exact settling, input cancellation, disabled navigation, and cleanup.
- TypeScript and changed-file lint passed; documentation gates passed.
- Fresh browser bundle built successfully. All 24 desktop cases have passing
  coverage (initial suite plus targeted final-build reruns); all 9 phone cases
  passed together on final assets. Desktop and phone screenshots were inspected.
- Phone cached-history regression was reproduced and fixed by retaining browser
  anchoring during history loading/refresh; 63 scroll/lifecycle tests and the
  full phone suite passed after the fix.
- The existing pinned-prompt hover test now waits for its finite entrance
  animation before hovering. It passed three consecutive runs on final assets.
- Local backend sockets required approved sandbox escalation; managed runners
  used one worker and fresh frontend assets with previously built backend binaries.
- No commit or push performed.

## PR review remediation

Continuous content growth now advances on every frame rather than restarting at
zero progress. Ordinary clicks and phone taps retain follow intent; a touch drag
beyond 6 px or a native scrollbar press yields control. Wheel and keyboard
interruption remain unchanged. The shared desktop/phone browser case now clicks
or taps transcript prose before asserting continued smooth following.

Validation: the new continuous-growth and click unit tests failed before the fix;
the phone-tap browser regression also failed against the earlier build. The
focused scroll, lifecycle, and search unit run passed all 77 tests.

Final rebuilt browser suites: desktop chat-motion 2/2 and phone chat-motion 2/2
passed with click/tap follow preservation and wheel/touch interruption.

## CI pagination remediation

A delayed upward scroll from prepend anchoring could retry a stale sentinel
intersection after the sentinel had already left preload. Gesture retries now
use the same current-geometry eligibility check as observer/lifecycle retries.
This preserves the existing pagination contract; no user setting changes.

Validation: the new stale-gesture regression failed before the fix; all 36
shared-sentinel tests, all 8 mobile pagination cases, and all 8 desktop
pagination cases passed after it. Typecheck, changed-file lint, and the spec
linter also passed.
Commands: `pnpm exec vitest run hooks/use-lazy-load-sentinel.test.ts` and
`pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-message-pagination.spec.ts -- --retries=0` from `apps/web`.
Desktop command: `pnpm e2e:run --host --no-build e2e/tests/chat/message-pagination.spec.ts -- --retries=0`.
