---
id: "02-header-and-swipe-cue"
title: "Calm the header and reveal swiping"
status: done
wave: 2
depends_on:
  - 01-phone-deck
plan: "plan.md"
requirements:
  - REQ-UI-THREADS-DECK-003
acceptance_criteria:
  - AC-UI-THREADS-DECK-003.9
  - AC-UI-THREADS-DECK-003.10
  - AC-UI-THREADS-DECK-003.11
  - AC-UI-THREADS-DECK-003.12
system_design:
  - ../../specs/ui/system-design/threads-conversation-deck.md
---

# Task 02: Calm the header and reveal swiping

## Summary

Apply the user's evaluation feedback to the implemented mobile deck. Group the
page and view labels, give task titles space, and explain horizontal paging.

## Scope and boundaries

- Phone-only topbar composition and task header presentation.
- A persistent localized swipe hint, numeric position, and at most seven
  decorative page dots; omit the cue for a single conversation.
- Preserve tablet/desktop controls, full-width chats, and existing pickers.
- No new session state, persisted preference, gesture handler, or backend API.
- Keep the same isolated demo and private Tailscale route; preserve its data.

## Implementation

Reuse the `PageTopbar` interactive title slot and the native `MobileColumnTabs`
orientation pattern. The board remains the only horizontal scroll owner;
transcripts and picker sheets retain their own vertical scroll regions. Keep
all actions at least 44 pixels and numeric indicators noninteractive.

## Validation

Add mobile E2E assertions before changing production markup: a two-line long
title at 360 pixels, stacked page/view labels, a separate swipe cue reflecting
picker and touch navigation, and no cue for one thread. Run the task-01 focused
unit, mobile/desktop E2E, build, typecheck, lint, and translation commands.
Visually inspect dark phone layouts and a multi-agent thread in the demo.

## Parallelism

`sequential`

## Results

- RED: the new mobile regression failed because the original header had no
  `thread-swipe-cue` element.
- GREEN: all 187 focused unit tests and 22 browser tests passed with one E2E
  worker and no retries. Mobile assertions cover stacked page/view labels,
  two-line long titles, no cue for a single thread, separate count/dots, and
  the correct active dot after both picker selection and a real touch swipe.
  Phone/tablet transitions, utility actions, plugins, agent selection, saved
  views, and all 12 desktop Threads regressions passed.
- Web build, backend fixture build, typecheck, scoped zero-warning lint,
  formatting, translation checks, specification lint, and public docs
  validation passed. The 138 existing locale orphan warnings remain unchanged.
- Dark phone emulation measured a 360-pixel chat and document in a 360-pixel
  viewport. Balanced titles and multi-agent controls were visually inspected.
  Screenshots are in `/tmp/kandev-mobile-threads-PuEDK5/`.
- The original evaluation database and private Tailscale route remain intact.
  HTTPS health and the TLS-verified WebSocket upgrade (101) passed after the
  UI refresh. No personal instance, credentials, commit, or publication used.
- Physical-device keyboard and Safari behavior remain unverified.

CI follow-up corrects the phone picker/swipe regression's creation-order
assumption. Threads rank by attention and recency, then retain their rendered
slots. The failing CI screenshot placed the selected thread at position 3/3;
the test attempted a left swipe beyond the final column. It now reads the
rendered order, performs one native touch swipe toward an existing neighbor,
and requires that exact neighbor's chat to activate. The shared gesture helper
also supports right swipes without changing held-touch or cleanup semantics.

The directional unit assertion failed with the original helper (start x=306
instead of 54), then passed after the change. The initial local browser run was
discarded after a confirmed test-server port collision with another worktree.
Its picker failure is not evidence of a product defect. The isolated rerun
uses an explicit free port offset, with no retries or increased timeouts.

Local follow-up checks passed from `apps/web`:

- `pnpm exec vitest run e2e/helpers/mobile-threads-swipe-helpers.test.ts`:
  three tests passed, including both failure-cleanup cases.
- `E2E_PORT_OFFSET=0 pnpm e2e:run --host --no-build --project mobile-chrome
  -- tests/task/mobile-threads-view.spec.ts
  tests/task/mobile-threads-swipe.spec.ts --retries=0 --repeat-each=3`:
  all 30 browser runs passed. This includes exact-neighbor activation, held
  swipe pagination, session picker, phone/tablet transitions, and saved views.
- `pnpm run typecheck`, scoped zero-warning ESLint, and Prettier checks passed.

Repository catalog validation, full specification lint, and whitespace checks
passed. Fresh hosted CI and current-base compatibility remain external delivery
gates; these local results do not mark them complete. Physical-device keyboard,
Safari, and the feature's previously documented manual gates remain unverified.


Final no-retry phone integration check:
`KANDEV_RUN_QUIET_DIR=/root/.cache/kandev-pr3598-quiet-owned E2E_PORT_OFFSET=0 GOMAXPROCS=4 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome -- tests/task/mobile-threads-view.spec.ts tests/task/mobile-threads-swipe.spec.ts tests/task/mobile-create-task-workflow-agent-overrides.spec.ts tests/session/mobile-port-forwarding.spec.ts tests/pr/mobile-pr-watcher-missing-branch.spec.ts tests/task/mobile-launch-failure-recovery.spec.ts tests/session/mobile-transient-turn-runtime-continuity.spec.ts --retries=0 --trace=retain-on-failure`
passed all 21 cases in 10.7 minutes. This covers the shared seed-origin fixture,
phone cancellation before any retry dispatch, continuation across viewers,
retry exhaustion, launch recovery, port actions and native thread navigation.
Web typecheck, full web ESLint, all twelve changed TypeScript files' Prettier
checks, and the five seed-origin/swipe unit regressions passed on this source.
These results do not close manual release gates or substitute for pushed-head CI.
