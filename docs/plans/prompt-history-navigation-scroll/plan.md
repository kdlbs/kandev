---
created: 2026-09-03
status: completed
requirements:
  - REQ-UI-PROMPT-HISTORY-PANEL-001
system_design:
  - ../../specs/ui/system-design/prompt-history-panel.md
legacy_specs: []
---

# Implementation Plan: Stabilize Prompt History Navigation Scroll

## Overview

Repair prompt-history navigation when the selected prompt is outside the loaded
transcript window. The around-window merge currently permits the first target
scroll to be displaced by transcript pagination or live content; the repair
retains the same navigation intent and performs one bounded reassertion after
the initial placement. The implementation is a single frontend vertical slice,
covering Dockview and phone hosts without changing the message API or transcript
pagination contract.

## Confirmed root cause

`useScrollTargetConsumption` in
`apps/web/components/task/task-chat-panel.tsx` requests an around window when the
selected row is absent, then clears the target after the first successful
rendered-row scroll. `usePendingMessageScroll` owns the equivalent phone/non-
Dockview path. Once the target is cleared, later transcript pagination or live
content can move the viewport without another navigation pass. The existing
`useScrollToMessage` verifier protects one scroll operation but cannot restore a
position changed after that operation has completed.

The same Dockview hook also has two adjacent lifecycle hazards that the repair
must close: it can start an around request while the initial transcript request
is still loading, and its shared `jumpRequestCount` only decrements when the
settling request is still the active target identity. A superseded request can
therefore leave the loading indicator permanently asserted. These are part of
the target lifecycle because both affect whether the first and reasserted scroll
can settle deterministically.

## Scope

### In scope

- Retain an unloaded prompt's target through one post-load reassertion.
- Re-run the existing start-aligned transcript scroll once, after the initial
  target scroll succeeds, within a defined one-second settling window.
- Defer an around-window request while the initial transcript request is still
  loading; do not create competing initial and around fetches.
- Ensure every settled around request releases its loading accounting, while
  stale requests cannot update visible loading state or clear a newer target.
- Preserve session, host-owner, token, stale-request, teardown, and failure
  guards.
- Keep already-loaded prompt navigation single-pass and bound retries to one
  reassertion.
- Add focused unit regressions and extend the existing desktop and mobile
  prompt-history browser flows.

### Out of scope

- Changes to message API payloads, around-window pagination, or transcript row
  ordering.
- Changes to generic auto-scroll behavior, persisted scroll offsets, or prompt
  history presentation.
- New navigation controls, settings, mobile surfaces, or public documentation.

## Technical approach

Update the shared target-consumption lifecycle in
`apps/web/components/task/task-chat-panel.tsx`. Distinguish the initial scroll
that follows an around-window merge from ordinary loaded-target consumption;
keep the matching target token live until a single delayed reassertion has
attempted the same start alignment. The first `scrollToMessage` return is the
observable completion boundary because the existing handle does not expose its
internal verifier. Start a fixed 250 ms delay at that boundary, and require
the reassertion to execute no later than the one-second navigation window.
Schedule one reassertion after that initial pass, without restarting it for
every rendered-message revision. Re-check the live session, host, token, active
visibility, and row guards before the reassertion, cancel it on supersession,
deactivation, or teardown, and compare-and-clear only after the final
successful pass. Apply the equivalent bounded lifecycle to
`usePendingMessageScroll` so phone navigation has the same contract. Replace
the phone message-only state with an explicit mobile target identity containing
session ID, message ID, and a monotonic token. Use a stable mobile-chat host key,
pass Chat visibility into the hook, and clear or cancel the target when Chat is
deactivated, the selected mobile panel changes, or the Chat host unmounts.
Returning to Chat must not consume an abandoned target.

While the initial transcript is loading, retain the target and wait for the
existing readiness transition before starting `loadMessageWindowAround`.
Deduplicate around requests with an in-flight key containing the session, host,
and target token; a rendered-message revision must not start a second request
for the same intent. Track each request with request-local settlement cleanup:
every settled request releases its loading contribution, while stale
completions may not publish stale loading/error state or clear a newer target.
Use existing `useScrollToMessage` and `loadMessageWindowAround` behavior rather
than adding another scroll implementation. The reassertion must be a single
bounded pass, not an unbounded animation-frame or timer loop.

The desktop browser regression must make a middle target genuinely absent from
the initial bounded transcript window. Seed prompts both before and after that
target, with enough newer transcript content after it for start alignment to be
reachable; never select the oldest or newest boundary prompt. Before
`page.goto`, attach `watchWs(page)` if the revision is delivered over
WebSocket; if using an HTTP mutation, prepare `waitForHttp` instead. Assert the
active Chat does not contain `#msg-<target-id>` immediately before selection,
intercept and assert exactly one target-specific `around` request, hold and
release that response, and use a controlled browser clock to observe the first
target placement. Record the monotonic timestamp when that first scroll
operation succeeds. Before mutating content, require two consecutive geometry
samples one animation frame apart with target top, transcript top, and scrollTop
unchanged within 0.5px. Immediately before the controlled mutation, arm the
prepared transport wait. Give the mutation a unique message ID/marker and,
for the new-row branch, insert that row before the target; alternatively,
change the height or scroll range of a target-preceding row. Assert the marker
was absent before mutation, await the exact transport event, assert the marker
is rendered in active Chat, and assert the target's offset relative to the
scrollport changed before advancing 250 ms. Assert the second scroll call and
await the same two-frame stable final geometry before the one-second deadline.
Polling is bounded by the remaining time from the first-scroll timestamp. The
mobile regression uses the same middle-target, reachable-start precondition
through the `Panels` picker.

## Tests

| Acceptance criterion | Test evidence |
| --- | --- |
| `AC-UI-PROMPT-HISTORY-PANEL-001.12` | Add deterministic fake-timer/RAF regressions proving an unloaded target is scrolled initially, remains owned while multiple transcript revisions occur, is scrolled exactly once more 250 ms after the first successful scroll and no later than one second, and is then cleared; prove loaded targets are not double-scrolled, initial-loading defers the request, same-token revisions do not duplicate an in-flight request, stale/superseded requests release loading state without clearing the active target, and hidden hosts cancel the delayed pass. Cover the mobile target identity and leave-Chat/return lifecycle separately. |

## E2E tests

- Desktop: extend `apps/web/e2e/tests/task/prompt-history-panel.spec.ts` to
  attach `watchWs(page)` before `page.goto` (or prepare an HTTP wait when the
  mutation is HTTP-only), seed or evict a middle prompt absent from the initial
  bounded transcript window, and retain prompts before plus at least one
  viewport of newer transcript content after the target. Do not select the
  oldest or newest boundary prompt. Assert the active Chat does not contain the
  target row immediately before selection, intercept and assert exactly one
  target-specific `around` request, hold and release its response, install a
  controlled browser clock, and record the monotonic timestamp when the first
  target scroll operation succeeds. Before the controlled mutation, require two
  consecutive samples one animation frame apart with target top, transcript top,
  and scrollTop unchanged within 0.5px. Give that mutation a unique message
  ID/marker and, for the new-row branch, insert the row before the target; the
  alternative must change the height or scroll range of a target-preceding row.
  Arm the transport wait immediately before the mutation, assert the marker was
  absent before it, await the exact event, assert the marker is rendered in
  active Chat, and assert the target offset relative to the scrollport changed.
  Advance 250 ms, assert exactly one reassertion, then await two consecutive
  samples one animation frame apart with target top, transcript top, and
  scrollTop unchanged within 0.5px. Bound that stable-scroll poll by the
  remaining time from the first-scroll timestamp, and assert the final target
  top relative to the active chat scrollport equals its computed
  `scrollMarginTop` within two pixels before the one-second deadline.
- Mobile: extend
  `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts` with the same
  genuinely unloaded, middle-target precondition and exact around-request
  assertion through the `Panels` picker and selected Chat surface. Ensure
  enough newer content remains after the target for exact start alignment.
  Scenario A uses the prepared transport wait, unique mutation marker, and
  target-path displacement before the target to await the rendered revision and
  changed target offset, advance 250 ms, assert exactly one reassertion, await
  the same bounded stable-scroll condition, and assert target top relative to
  the phone transcript scrollport equals the target's computed
  `scrollMarginTop` within two pixels. Scenario B leaves Chat before the delayed
  pass and verifies that returning does not consume the cancelled target.
- Run each through the matching `chromium` and `mobile-chrome` projects with
  the managed E2E runner so the production Vite build is rebuilt.

## Mobile design contract

- **Desktop outcome:** selecting an unloaded prompt from Prompt history returns
  to Chat and leaves that prompt start-aligned after the around-window merge and
  one controlled transcript revision.
- **Mobile entry point:** Prompt history is opened from the existing grouped
  `Panels` bottom-navigation picker; selecting its arrow returns to the
  existing selected Chat surface.
- **Nearest shipped exemplar:** `SessionMobileLayout` and
  `mobile-prompt-history-panel.spec.ts` provide the phone panel and transcript
  composition.
- **Hierarchy and presentation:** Chat remains a full-height surface with one
  vertical transcript scroll owner; no drawer or new navigation surface is
  added. Existing dynamic viewport and safe-area handling remain unchanged.
- **Shared state and logic:** target identity, around-window loading, and the
  one-pass reassertion are shared. The phone-specific `usePendingMessageScroll`
  lifecycle owns only the non-Dockview handoff and consumption callback.
- **Mobile evidence:** the focused mobile Playwright scenario proves the same
  final start alignment and bounded reassertion outcome as desktop.

## Work orders

- [x] [Task 01: Reassert unloaded prompt navigation](task-01-reassert-unloaded-prompt-navigation.md)

## Verification results

Implemented and verified:

- Focused navigation and mobile layout tests: 50 passed.
- Desktop prompt-history E2E: 2 passed.
- Mobile prompt-history E2E: 4 passed.
- `make fmt`, `make typecheck`, and `make lint` passed.
- `make test` was attempted twice; unrelated backend environment-sensitive tests
  failed in process lifecycle, configuration discovery, service metadata, and
  update-handler suites.

## Risks

- A delayed reassertion could override a newer user navigation unless it
  compare-checks the target token at execution time.
- Clearing the target after the first pass would preserve the current defect;
  the regression must assert both scroll calls and final target ownership.
- A stale around request can leak the loading indicator unless request-local
  settlement accounting is tested.
- The phone path has a separate pending-target hook and must not rely only on
  Dockview coverage.

## Open questions

None.
