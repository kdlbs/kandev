---
created: 2026-09-26
status: complete
requirements:
  - REQ-UI-TRANSCRIPT-AUTO-SCROLL-001
system_design:
  - ../../specs/ui/system-design/transcript-auto-scroll.md
legacy_specs: []
---

# Implementation Plan: Initial Transcript History Placement

## Overview

Repair [issue #3979](https://github.com/kdlbs/kandev/issues/3979).
First preserve initial placement and live-turn follow intent.
Then provide explicit latest-message navigation. Both work orders run sequentially.

The UI system owns this reusable transcript interaction contract. Existing
criteria `.11` through `.15` cover task-switch behavior. Criteria `.16` through
`.18` define ordinary entry, content growth, and explicit latest navigation.
The existing design remains authoritative for unread targets and saved positions.
This package refines that design without changing storage or backend ordering.

## Evidence and root cause

Investigation used commit `dfce4dac058` and the canonical issue, which contains
no image attachments or comments. GitHub assignment is `carlosflorencio`.

`runInitialScrollPositionEffect` selects provisional placement only when an
environment-switch token exists. Ordinary entry with cached rows and a pending
refresh therefore consumes final placement too early. Refresh completion cannot
reconcile the viewport once `didInitialScroll` is true.

`applyInitialScrollPosition` also consumes the final latch for every competing
owner. It does not distinguish a temporary blocker from a completed handoff.
`pendingChatScrollTop` has no session identity, and its release is not an explicit
initial-placement effect dependency. A blocked visit can thus lose its retry.
Valid same-transcript restores must retain their existing priority.

A temporary Node harness extracted the actual coordinator function bodies with
the TypeScript parser. It supplied a clamped DOM offset and minimal store stubs:

| Scenario                                                           | Observed offset | Required offset |
| ------------------------------------------------------------------ | --------------: | --------------: |
| Initial placement blocked at zero, then blocker released           |               0 |            2500 |
| Ordinary cached entry at 2500, then history grows and refresh ends |            2500 |            4500 |

Both paths consumed `didInitialScroll`. The harness was `/tmp/kandev-3979-repro.cjs`.
To repeat the trace, extract `runInitialScrollPositionEffect` and its local
placement helpers from `message-list-native-scroll.ts`. Set viewport height to
500 and content height to 3000. Invoke with 30 items, enabled auto-scroll, no
unread target, and no environment token. For the first case, set the pending
layout offset to zero, invoke, clear it, and invoke again. For the second case,
start with refresh pending, invoke, increase content height to 5000, clear
refresh pending, and invoke again. The second invocation does not place either
viewport. This is coordinator evidence, not a browser reproduction of the
reporter's exact environment.

The issue's browser-anchoring claim remains unproven. Message wrappers already
exclude themselves from anchoring, and the bottom anchor participates. Avoid a
blanket CSS change without rendered evidence. The helper's null result alone is
also not the complete cause: the competing-owner branch precedes that helper.

## Follow-up: live-turn reader intent

The user clarified two outcomes: sending from the latest message keeps following
the agent, and any deliberate upward scroll pauses following while the reader
inspects history. The running indicator must not remain clipped while following.
This is a temporary reader pause, separate from the saved auto-scroll preference.

Temporary tests using the actual `useAutoScroll` hook reproduced two violations:

- With animations off, a 30px upward wheel followed by an appended message
  wrote the bottom offset instead of preserving the reader's 570px position.
- A working-state transition wrote the bottom offset despite a reader-owned
  250px position.

`useFollowIntent` uses a 100px proximity threshold unless `userReading` is set.
The motion driver sets that flag on input, but exists only when motion is enabled.
The work-start path also substitutes `!hasUnreadDivider` for actual follow intent.
A third hook test modeled content growth followed by a native scroll event,
without a user gesture. The next append stayed at 600px instead of requesting
the bottom. `resyncIsNearBottom` treated the resulting 200px gap as reader intent.
This is a controlled event-order reproduction, not a browser-proven cause.

These mechanisms explain the failing hook tests. A controlled Chromium run also
reproduced the 30px reader-pause violation under reduced motion. The animations-on
control preserved the reader's position. The reported intermittent clipping
path remains unconfirmed. See [the follow-up evidence](investigation.md).

The legacy initial-prompt fallback in
`use-processed-messages.ts::buildTaskDescriptionMessage` requires initialized,
exhausted history without a stored user prompt. It does not explain cold-load
prompt display by itself. The separate
`useScrollToDividerOrBottom` effect can place the bottom after refresh, so the
isolated coordinator trace does not prove whole-UI failure on every ordinary open.
The browser reproduction distinguished held history from fully loaded history.
With HTTP hydration and the WS refresh both delayed, the empty transcript showed
loading and preparation status. It showed no first user prompt. After release,
it reached the latest message. The claim that first-message focus is only a
loading artifact remains unconfirmed on this checkout.

## Scope

### In scope

- Ordinary, cached, cold, desktop, and phone transcript entry.
- Send-from-bottom continuity, complete running-indicator visibility, and small-scroll reader pauses.
- Explicit completion versus deferral of scroll ownership.
- Content-size reconciliation while bottom-follow intent remains active.
- A localized Jump to latest action that targets assistant output too.
- Regression tests, mobile parity, and public navigation documentation.

### Out of scope

- Backend message ordering, pagination sizes, or transcript virtualization.
- Changes to unread-divider preferences, saved offsets, or auto-scroll defaults.
- Replacing last-prompt navigation or adding a new persisted preference.

## Technical approach

Task 01 updates the existing coordinator and its restore integration. Use a
session-scoped visit identity even without an environment-switch token. Separate
applied, deferred, and transferred outcomes. Observe blocker release and content
size without stealing explicit reader ownership. Preserve cancellation and stale
token guards. Keep pagination blocked through real placement completion.

Task 02 extends `MessageListHandle` and the task Chat composition. Use one shared
native bottom action and visibility policy. Keep the action outside the
scroller, with responsive hit targets and six-language localization.

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

## Tests

| Criteria                   | Required regression evidence                                                                                                                          |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------- |
| `.11`, `.14`, `.16`        | `message-list-native.test.tsx`: ordinary cached refresh with no environment token, cold entry, temporary blocker release                              |
| `.10`, `.12`, `.15`, `.16` | Same harness: disabled saved offset, reader cancellation, selected unfocused tab, valid restore and unread ownership                                  |
| `.13`, `.16`               | Same harness: no older-page request before actual placement, current sentinel geometry after release                                                  |
| `.5`, `.7`, `.17`          | Content growth without a new message identity, motion on/off, write-only append, observer cleanup                                                     |
| `.19`, `.20`               | Send-from-bottom, delayed status/footer growth, 30px upward wheel, touch/key/scrollbar input, and work-start while reading with motion on/off/reduced |
| `.18`                      | Native handle and new `jump-to-latest-button.test.tsx`: assistant end, visibility, keyboard, preference preservation                                  |

## E2E tests

Extend `auto-scroll-toggle.spec.ts` and `mobile-auto-scroll-toggle.spec.ts` for
ordinary history entry without an unread divider. Hold latest-window responses
with the existing WS helper. Assert cached placement before release and final
placement afterward. Seed completed history so live output cannot hide failure.
Cover cold entry, delayed content growth, reader scroll during refresh, disabled
saved positions, and no premature pagination. Retain existing maximize, unread,
and last-prompt suites.

Add `jump-to-latest.spec.ts` and `mobile-jump-to-latest.spec.ts`. Include an
assistant response below the last prompt. Prove actual bottom geometry after
activation, no preference mutation, keyboard usability, and phone containment.
Use Chromium and mobile-chrome projects, respectively.

The auto-scroll suites also need a paced live turn. Send from the actual bottom,
measure the running indicator against the viewport, and inspect each output phase.
Scroll upward by 30px during a pause before the next output. Assert that later
output does not move the reader. Repeat with chat motion enabled and disabled,
and with OS reduced motion. Use real wheel/touch input for the rendered checks.
Do not treat Playwright `toBeVisible` as proof that a row lies inside its scroll
viewport: measure both rectangles and the bottom gap.

## Work orders

- [x] [Task 01: Preserve initial placement until completion](task-01-preserve-initial-placement.md) (complete)
- [x] [Task 02: Add explicit latest-message navigation](task-02-jump-to-latest.md) (complete)

Task 02 depends on Task 01 because both use the native scroll coordinator.
No subagents were used. Existing completed scroll packages remain unchanged.

## Verification results

Temporary coordinator reproduction: both failure paths reproduced. Task 01
implementation is complete. Its focused unit suite (74 tests) and web typecheck
passed. The required Chromium auto-scroll, unread-divider, and session-layout
run had 22 passes and one terminal-marker failure; the terminal case passed on
targeted rerun. All seven required mobile auto-scroll tests passed, including a
real 30px touch gesture under reduced motion. Temporary hook and browser
investigation results are recorded in [the follow-up evidence](investigation.md).
Task 02 implementation is complete. The final focused unit run passed 182 tests,
including explicit latest navigation, stale activation cancellation, empty
history visibility, and content growth. Web typecheck passed. The managed Vite
E2E build passed. Chromium Jump to latest passed, and all 11 desktop
last-prompt cases passed. Phone Jump to latest passed at 390px, 767px, and
768px; the 44px target, composer clearance, focus transfer, and horizontal
overflow checks passed. The mobile last-prompt case passed. The phone run saved
a screenshot through Playwright's test output path.

`pnpm run i18n:check`, `pnpm run i18n:ratchet`, and targeted ESLint passed with
no errors. Public docs validation passed (62 validator tests and 47 published
pages). Specification validation:

- `python3 scripts/list-docs.py validate`: passed (311 decisions, 1187 specifications).
- `python3 scripts/lint-spec-files.py --all`: passed.
- `.github/scripts/pr-docs.cjs::validateCoverage`: passed for both work orders,
  using the planned coordinator path as a simulated runtime change. All REQ/AC,
  design, and plan references resolved. No runtime file was changed.
- `git diff --check`: passed.

Review follow-up: deterministic regressions exposed and fixed two remaining
ownership races. Jump to latest supersedes a queued prompt verifier and releases
the programmatic guard before bottom placement. Layout restore ownership is
captured for the current visit before visibility or restore deferral, so a
completed same-session restore survives hidden activation. The native scroll
suite passed all 72 tests. The focused coordinator, navigation, status-bar, and
composer suite passed 111 tests. Typecheck and the production Vite build passed.
Targeted ESLint passed with zero warnings after extracting the placement,
pagination, composer, and status-row helpers. Desktop and phone Jump to latest
E2E passed after the final refactor.

PR fixup on 2026-09-27 addressed five review threads and two failed E2E leaves.
Reader navigation now claims the pending placement during refresh. Jump to
latest cancels session-owned Dockview, pending-message, and scroll-to-start
navigation before taking bottom ownership. Directional input pauses follow only
when moving toward older content; tests cover downward/no-op input and upward
scrollbar-thumb drags. Archived transcripts keep the latest action. Quick Chat
focus is restored after the browser's click default focuses its scroll container.

The phone Jump test sets an off-bottom position before tapping the control.
Chromium did not synthesize a click after the test's raw CDP swipe, while the
separate phone live-follow test still verifies the real 30px upward touch.
The stale composer-disclosure assertion now verifies that the reader stays more
than 300px from the bottom after upward movement, instead of expecting an
unchanged pixel anchor while clarification rows are inserted.

Post-fixup verification:

- `pnpm exec vitest run components/task/chat/chat-scroll-motion.test.ts components/task/chat/use-chat-scroll-motion.test.tsx components/task/chat/message-list-native.test.tsx components/task/task-chat-panel.scroll-target.test.tsx`: 146 tests passed.
- `pnpm run typecheck`: passed.
- Targeted ESLint on changed TS/TSX files: passed with zero warnings after extracting helpers that exceeded the native function limits.
- `pnpm run i18n:ratchet`: passed.
- `(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/chat/quick-chat.spec.ts e2e/tests/chat/jump-to-latest.spec.ts e2e/tests/task/threads-composer-disclosure.spec.ts --grep "clarification shortcuts work after clicking the message surface|Jump to latest returns to the newest grouped reply and preserves auto-scroll|archived transcript keeps Jump to latest available without a composer|scrolls long required questions through the Grid footer" --retries=0)`: 7 passed; the managed Vite build passed.
- `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-jump-to-latest.spec.ts e2e/tests/chat/mobile-auto-scroll-toggle.spec.ts --grep "phone Jump to latest fits coarse targets|archived phone transcript keeps a reachable Jump to latest control|is reachable and toggles by touch|follows a live turn from the bottom, then pauses after a small upward touch" --retries=0)`: 4 passed.
- `(cd apps/web && pnpm e2e:run --host --no-build --project chromium e2e/tests/chat/last-prompt-scroll.spec.ts --retries=0)`: 11 passed.
- `(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/chat/mobile-last-prompt-scroll.spec.ts --retries=0)`: 1 passed.
- `git diff --check`: passed after recording these results.

The first post-push CI run for `dc0f14e8971` found two E2E failures: the desktop
and mobile visit-start unread-divider tests left the divider outside the
viewport. Both aggregates failed as a result. The reader-intent check was
sharing the initial-placement completion latch, so delegating placement to an
unread divider could be mistaken for explicit reader ownership. A separate
per-visit reader-position latch now records user input and latest navigation;
initial-placement delegation no longer claims it. A coordinator regression
covers unread-divider placement after that delegation. The focused scroll suite
passed 147 tests, typecheck and targeted ESLint passed, and the desktop and
mobile unread-divider E2E tests both passed locally after rebuilding the web
assets. The follow-up commit and its CI results are pending.

## Risks

- A bottom retry can overwrite a valid restore, unread target, or user gesture.
- DOM growth can look like user departure unless intent is tracked separately.
- A stale callback can move a newly selected session or release its pagination.
- Browser anchoring varies. Rendered checks must prove the correction.
- The reporter's exact browser and runtime state remain unknown.
