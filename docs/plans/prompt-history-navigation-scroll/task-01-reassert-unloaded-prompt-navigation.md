---
id: "01-reassert-unloaded-prompt-navigation"
title: "Reassert unloaded prompt navigation"
status: completed
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-PROMPT-HISTORY-PANEL-001
acceptance_criteria:
  - AC-UI-PROMPT-HISTORY-PANEL-001.11
  - AC-UI-PROMPT-HISTORY-PANEL-001.12
system_design:
  - ../../specs/ui/system-design/prompt-history-panel.md
---

# Task 01: Reassert Unloaded Prompt Navigation

## Summary

Make prompt-history navigation resilient to transcript growth after an around-
window load. An unloaded target receives its normal initial start-aligned scroll,
then one guarded reassertion 250 ms later, within one second of the first
successful scroll, before the target is consumed; already-loaded targets remain
single-pass.

## In scope

- TDD regressions for Dockview target consumption and the phone/non-Dockview
  pending-message path.
- Explicit mobile target identity, stable mobile-chat host ownership, Chat
  visibility propagation, and leave-Chat/return cancellation.
- Target-token and session/host ownership checks across the delayed pass,
  including active visibility and deactivation cancellation.
- Initial-transcript loading deferral, same-token around-request deduplication,
  and request-local around-request settlement accounting.
- A bounded single reassertion using the existing transcript scroll handle.
- Desktop and mobile Playwright coverage for a genuinely unloaded target and
  the user-visible navigation result.

## Out of scope

- Message fetching, transcript pagination, auto-scroll policy, or scroll API
  changes outside target navigation.
- Prompt history row layout, localization, settings, or backend changes.

## Acceptance

- An unloaded prompt is initially navigated after its around-window response,
  remains owned while that first position can be displaced, and is re-navigated
  exactly once 250 ms after the first successful target scroll and no later
  than one second after that boundary before compare-and-clear.
- Loaded prompts are not double-scrolled. A target whose host becomes inactive,
  is superseded, changes session, is torn down, or whose request fails cannot
  run or clear the delayed pass.
- Around navigation requested during initial transcript loading waits for
  readiness; rendered-message revisions cannot duplicate an in-flight request
  for the same session/host/token; every settled request releases loading
  accounting without allowing stale state to overwrite the active target.
- Desktop and phone prompt-history selection leave the target message aligned to
  the transcript viewport's start after the around-window/content-change flow.
## Verification

Focused unit tests:

```bash
cd apps && pnpm --filter @kandev/web test -- --run components/task/task-chat-panel.scroll-target.test.tsx
```

The regression cases must cover: absent target while initial messages are
loading; first placement followed by multiple transcript revisions and exactly
one delayed reassertion at 250 ms; already-loaded single-pass navigation;
same-token rerenders that do not duplicate an in-flight request; stale A/B
requests whose loading accounting settles; superseded, session-swapped,
hidden, or unmounted targets that cannot run or clear the delayed pass; and the
mobile target's leave-Chat/return cancellation.

Focused browser tests after a production build:

```bash
cd apps/web && pnpm e2e:run --project chromium e2e/tests/task/prompt-history-panel.spec.ts
cd apps/web && pnpm e2e:run --project mobile-chrome e2e/tests/task/mobile-prompt-history-panel.spec.ts
```

Each browser regression must attach `watchWs(page)` before `page.goto` when
the controlled revision is delivered over WebSocket, or prepare `waitForHttp`
when the mutation is HTTP-only. It must seed or evict a middle target absent
from the initial bounded transcript window, with prompts before and at least
one viewport of newer transcript content after it so exact start alignment is
reachable. Do not select the oldest or newest boundary prompt. Assert the
active Chat does not contain `#msg-<target-id>` immediately before selection,
intercept and assert exactly one target-specific `around` request, hold and
release that response, and control the browser clock. Record the monotonic
timestamp when the first target scroll operation succeeds. Before mutation,
require two consecutive geometry samples one animation frame apart with target
top, transcript top, and scrollTop unchanged within 0.5px. Give the controlled
mutation a unique message ID/marker and, for the new-row branch, insert that
row before the target; the alternative must change the height or scroll range
of a target-preceding row. Arm the transport wait immediately before the
mutation, assert the marker was absent before mutation, await the exact
transport event and active-Chat DOM marker, assert it is rendered, and assert
the target offset relative to the scrollport changed. Advance 250 ms and assert
exactly one reassertion. Await two consecutive samples one animation frame
apart with target top, transcript top, and scrollTop unchanged within 0.5px.
Bound this stable-scroll poll by the remaining time from the first-scroll
timestamp, then verify target top minus transcript top equals the target's
computed `scrollMarginTop` within two pixels before the one-second deadline.
The mobile test has two scenarios: Scenario A completes the gated reassertion
and asserts target top minus phone transcript top equals its computed
`scrollMarginTop` within two pixels; Scenario B leaves Chat before the delayed
pass and verifies that returning does not consume the cancelled target.

Final checks required by the task:

```bash
make fmt
make typecheck test lint
```

## Files likely touched

- `apps/web/components/task/task-chat-panel.tsx`
- `apps/web/components/task/task-chat-panel.scroll-target.test.tsx`
- `apps/web/components/task/mobile/session-mobile-layout.tsx`
- `apps/web/e2e/tests/task/prompt-history-panel.spec.ts`
- `apps/web/e2e/tests/task/mobile-prompt-history-panel.spec.ts`
- `docs/specs/ui/requirements/prompt-history-panel.md`
- `docs/specs/ui/system-design/prompt-history-panel.md`

## Dependencies

None.

## Risks

- React effect reruns and Dockview activation can race the delayed pass; every
  callback must re-read the current target identity.
- Mobile uses a separate pending-target lifecycle, so unit and browser coverage
  must exercise both hosts.

## Parallelism

`sequential`

## Inputs

- `docs/specs/ui/requirements/prompt-history-panel.md`, especially
  `AC-UI-PROMPT-HISTORY-PANEL-001.11` and `.12`.
- `docs/specs/ui/system-design/prompt-history-panel.md`, Prompt navigation.
- Existing target tests and `useScrollToMessage` behavior.

## Results

Implemented. Dockview and mobile target lifecycles now defer initial loading,
deduplicate same-token around requests, retain unloaded targets through one
guarded delayed reassertion, and cancel stale or inactive work. Focused tests,
desktop E2E, mobile E2E, formatting, typecheck, and lint passed. The repository
test target remains environment-flaky in unrelated backend suites.
