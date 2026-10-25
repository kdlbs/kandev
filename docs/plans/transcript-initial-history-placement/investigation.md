# Follow-up investigation: send, reader intent, and history loading

Date: 2026-09-26. Source: `dfce4dac058` (28 commits after v0.96.0).
Scope: temporary diagnosis against unchanged production code. The managed E2E
runner built this checkout and launched disposable Chromium runtimes. It did
not use the user's live instance. No native subagents were used.

## User-confirmed behavior

- Sending from the latest message keeps following the new turn.
- Agent output and the running indicator remain visible while following.
- A deliberate upward scroll pauses following, including a small movement.
- Reader pause is temporary intent, not a change to the auto-scroll preference.

These outcomes are recorded as `AC-UI-TRANSCRIPT-AUTO-SCROLL-001.19` and `.20`.
The existing `.17` criterion covers content growth. The package retains explicit
navigation, unread targets, valid layout restoration, and disabled preferences.

## Browser evidence

The seed used a completed task, 35 persisted agent messages, and an enabled
transcript. Each live check sent a delayed mock turn from the actual bottom.
Controlled harness messages then arrived while the running indicator remained
present. A real wheel gesture moved the reader upward by 30px before three
more messages arrived. Geometry was sampled after 24 frames to cover the
existing 180ms motion duration.

| Case | Observed result |
| --- | --- |
| Existing enabled-send E2E | Passed. Latest output reached the bottom. |
| Send from bottom, reduced motion | Bottom gap 0px. Running text fully inside the viewport. |
| Send from bottom, animations on | Motion settled to a gap below 3px. Running text remained inside the viewport. |
| Small upward wheel, reduced motion | Failed reader preservation. Offset changed from 1940px to 2141px, the new bottom. |
| Small upward wheel, animations disabled in saved settings | Failed with the same 1940px to 2141px jump. OS motion remained allowed. |
| Same wheel and output, animations on | Passed. Offset stayed 1940px while the bottom gap increased to 201px. |
| Initial HTTP history available, WS refresh held | Near the latest content with a transient 40px gap. Release restored a 0px gap. No oldest-message landing. |
| HTTP hydration and WS history both held | Offset 0px, non-overflowing content, loading and preparation status. No first user prompt. |
| Both history paths released | Offset 1811px, content height 2295px, viewport height 484px, bottom gap 0px. |

The current checkout did not reproduce a persistent clipped running indicator
or a loaded transcript stuck at its oldest message. The reporter's browser and
exact runtime state remain unknown. These results do not establish that every
reported first-message view is a loading artifact.

The initial temporary paced-output test used an exact selector that did not
match the merged streaming paragraph. That was a fixture failure. Its corrected
version could observe the output too late to prove an in-turn reader pause.
The final controlled-delivery test explicitly kept the turn running and caused
new transcript height after the gesture. Only that test establishes the browser
reader-pause result above.

## Hook evidence and cause

Temporary tests used the existing `AutoScrollHarness` with the actual
`useAutoScroll` hook. Three expected-behavior assertions failed:

1. A 30px upward wheel at 570px followed by an append wrote the shared bottom
   offset, `2147483647`, instead of preserving 570px.
2. A work-start transition at a reader-owned 250px position wrote that same
   bottom offset instead of preserving 250px.
3. Simulated content growth and a native scroll event without user input cleared
   follow intent. The next append stayed at 600px instead of requesting bottom.

The harness records raw scroll writes, so the large value is a bottom command,
not a literal browser position. The third case models an event ordering. It does
not prove that this ordering caused the user's intermittent symptom.

`useFollowIntent` applies a 100px proximity threshold unless `userReading` is
set. `useChatScrollMotion` installs interruption handling only when its motion
driver exists. Reduced motion and the disabled animation preference omit that
driver. The work-start path uses `!hasUnreadDivider` instead of actual reader
intent. These paths need one shared reader-intent policy independent of motion.

## History-load classification

`fetchSessionDataForTask` loads a newest window over HTTP before the native
transcript's WS refresh. Holding only `message.list` leaves hydrated rows visible.
The complete cold test held `/api/v1/task-sessions/<id>/messages` and WS
`message.list`, then opened the task from the board. After the optional HTTP
hydration deadline, the transcript showed a loading state. Releasing both paths
placed the newest content at the bottom.

`shouldShowTaskDescriptionFallback` requires initialized, exhausted history
without a stored user prompt. It does not display the initial prompt merely
because conversation loading is pending. The separate divider/bottom effect can
also correct ordinary entry, so isolated coordinator traces cannot establish a
whole-UI failure for every open.

## Commands and retained evidence

The temporary files were `components/task/chat/issue-3979-repro.test.tsx` and
`e2e/tests/chat/issue-3979-repro.spec.ts`, relative to `apps/web`. They are removed
after diagnosis. The work order names the permanent regression destinations.

```bash
(cd apps/web && pnpm exec vitest run components/task/chat/issue-3979-repro.test.tsx -t 'temporary issue 3979')
(cd apps/web && pnpm e2e:run --host --shards 1 tests/chat/auto-scroll-toggle.spec.ts --grep 'enabled auto-scroll stays at the bottom for live messages')
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 tests/chat/issue-3979-repro.spec.ts --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --shards 1 tests/chat/issue-3979-repro.spec.ts --grep 'cold history' --retries=0)
```

The `--no-build` runs reused the fresh, unchanged production build from the
first managed run. Hook cases ran in two targeted invocations. The final
controlled browser matrix had one expected failure and two passes. The complete
cold-history check passed separately. A separate saved-animation-off run
reproduced the same reader-pause failure. Screenshots and temporary test sources
are retained under `/tmp/kandev3979-browser-evidence/` for this workspace session.
The screenshots include the fully visible running indicator and reader positions.

Mobile and WebKit reproduction remain pending. The work order requires mobile
coverage during implementation. The package does not claim the current browser
results cover the reporter's unknown browser.

## Cleanup and package validation

Temporary test files were removed from the repository after their sources were
copied to the evidence directory. Managed E2E fixtures stopped their owned
backend/browser processes. Production code remains unchanged. Catalog validation,
specification lint, documentation cross-reference coverage, and diff checks passed.
