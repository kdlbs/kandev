# PR popover and transcript setup consolidation

## Evidence and scope

Hosted run 38493594964 executes every expected case: 3,930 pass, one phone
startup-retry case fails, and the same 40 identities remain skipped. The
slowest normal test step is 1,136 seconds, a 47.38% reduction from 2,159 seconds,
56.5 seconds short of the target. That shard has 282 first-attempt passes and
no retries. Its slower process spends 127.318 seconds in auto-scroll-toggle
and 89.240 seconds in pr-topbar-popover. The other process has less work;
removing repeated setup from these files addresses measured work directly.

Before editing, all 29 cases in the two selected files pass in 183.174 seconds
on the rebuilt current-main merge, pinned CI image, four CPUs, 8 GiB, one
worker, no retries. This local subset is not a hosted acceptance result.

No production behavior, runner count, timeouts, skip policy, or test-only seam
is removed. Existing unit tests for PR aggregation and transcript progression
remain. Browser assertions move unchanged into named steps; state preconditions
are re-established before each transition. Separate desktop/mobile, provider,
authentication, refresh, geometry and live-stream boundaries remain covered.

## PR popover candidates

Owner: `components/github/pr-ci-popover.tsx`, `use-pr-ci-popover`, and the shared
change-request anatomy. Requirements: current PR check results and existing
PR popover/hover contracts. History includes #4092's current-suite semantics,
retained as its separate real-provider journey. Aggregate assertions protect
pre-feedback rendering; the selected-suite journey protects provider refresh.

Consolidate into **PR aggregate, review and navigation state transitions**:

- C: `counts row uses TaskPR aggregates and hides zero-count buckets`. Keep all assertions in its identically named step.
- C: `shows only Passed when all checks succeed (in_progress + failed hidden)`. Keep all assertions in its identically named step.
- C: `review row shows N / M required + unresolved comments count`. Keep all assertions in its identically named step.
- C: `comments row hidden when unresolved_review_threads = 0`. Keep all assertions in its identically named step.
- C: `hover opens popover; mouse-leave closes it after the close delay`. Keep all assertions in its identically named step.
- C: `header PR link points at the PR on GitHub`. Keep all assertions in its identically named step.
- C: `empty state when PR has no checks at all`. Keep all assertions in its identically named step.
- C: `counts update on a re-associate (live refresh)`. Keep all assertions in its identically named step.

The keeper starts with no checks, updates aggregate counts while open, reaches
all-passing and review/comment states, verifies the external URL and exercises
pointer-only close. It retains exact counts, hidden buckets, header-only
passed groups, review requirements, comment absence, link destination and
close behavior. No loaded provider feedback is substituted for the aggregate
precondition.

Consolidate into **workflow grouping and pointer bridges**:

- C: `workflow rollup groups jobs and shows (N/M passed) badge under Failed`. Keep all assertions in its identically named step.
- C: `popover survives the cursor crossing from the button onto it`. Keep all assertions in its identically named step.
- C: `chip popover survives the cursor crossing from the chip onto it`. Keep all assertions in its identically named step.
- C: `failed workflow row exposes open + add-to-context buttons`. Keep all assertions in its identically named step.

The keeper seeds the existing six concrete check runs, preserves both workflow
ratios and all Lint actions, then crosses the actual topbar and chip gaps.
Each bridge still waits past the 150 ms product timer and verifies controls
remain usable. Closing one popover precedes opening the other.

Retain the conflict badge/details, overflowing viewport, auth loss, held
refresh indicator and current-suite/cancellation journeys independently.

## Transcript candidates

Owner: the native transcript and `transcript-auto-scroll.ts`. Requirements:
REQ-UI-TRANSCRIPT-AUTO-SCROLL-001, including bottom anchoring, progression-based
catch-up and persisted reader position. Existing message revisions and native
geometry remain the independent oracles. The real mock-agent session and
follow-up WebSocket delivery remain; no transcript outcome is stubbed.

Consolidate these five cases into **auto-scroll state transitions**:

- C: `enabled auto-scroll stays at the bottom for live messages`. Preserve the original assertions as a named step.
- C: `disabling while genuinely at the bottom still freezes the view when new content arrives`. Preserve the original assertions as a named step.
- C: `disabling freezes the position and suppresses auto-scroll for new messages`. Preserve the original assertions as a named step.
- C: `preserves the frozen scroll position across navigating away and back`. Preserve the original assertions as a named step.
- C: `catches up on re-enable when content arrives after remounting while already disabled`. Preserve the original assertions as a named step.

Retain `re-enabling does not jump to the bottom when nothing progressed while disabled` with its own persisted-history setup. Consolidate `re-enabling does not jump when the user scrolls further away themselves while disabled` into a named step in that clean-history case. Neither scenario sends live follow-ups before its no-progression assertion.

Each step restores enabled state or an explicit midpoint as needed. After live
content while disabled, the test waits for the real turn to settle, re-enables
and observes bottom catch-up before establishing the next baseline. The two
navigation paths still actually leave and reopen the task. Negative no-jump
windows, exact/bounded scroll offsets and overflow-anchor assertions remain.

Retain independently: environment-changing task switches, inactive session
activation and hidden updates, hidden preference, the compact default toggle
layout, and the held live-stream upward-wheel case. Their different lifecycle,
layout and transport preconditions cannot be established by the keeper.

## Validation

Baseline: 29 passed in 183.174 seconds, zero retries/skips. The initial consolidated
run passes all 13 cases in 115.903 seconds under the same resource limits,
saving 67.271 seconds (36.73%) in this local subset. Blob evidence accounts for
all 29 original scenarios: 10 retain standalone identities and 19 execute as
successful named steps. The first attempt exposed a missing action URL in the
combined workflow fixture; the final seed preserves the original URL and
failure output. No assertions were loosened. Whole-file zero-warning ESLint,
web typecheck and 54 focused owner/fixture unit tests pass. That local subset did not establish hosted acceptance; the following section records the hosted result.
The separate phone startup-retry failure is handled as a fixture race, not a
consolidation or deleted assertion.

## Hosted result and retained isolation

Run 38500882345 completes all 14 normal and six container shards successfully.
The longest normal test step is 1,049 seconds (17m29s), 51.41% below the fixed
2,159-second reference. The 34 blobs contain exactly the 3,955 expected
identities: 3,915 pass and the unchanged 40 skip. No cases are omitted or
duplicated. Five cases recover on their first retry; both phone startup-retry
cases pass their first attempts. The two-file consolidation retains all 29
original scenarios. The full E2E workflow passes; elapsed time including queue, build and report is 50m46s.

One retry occurs in the new scrolling sequence: its no-progression assertion
observes scrollTop 2,117 after recording a 1,059 midpoint, following two live
turns. Passing local runs did not prove that this combined setup preserved a
quiet-history precondition. Restore the original standalone no-progression
setup and place the manual-scroll/no-progression scenario beside it. Keep
all scroll offsets, negative-assertion windows and live progression checks;
do not add sleeps, increase timeouts or relax assertions. The revised two-file
catalog is 14 cases covering 11 original standalone scenarios and 18 named
steps. New-head validation and hosted timing remain required.

The restored scrolling file passes all 21 cases over three full-file repetitions with retries disabled on current main. Transcript owner tests (35), current-main rich-output owner tests (28), typecheck, formatting/lint and documentation checks pass. The original shard passes 280 cases with the same three skipped identities, zero retries, omissions or duplicates. Next-head CI remains pending.

The old-head frontend test job was cancelled after its test step exceeded 45 minutes (prior run: 16m42s). Normal cancellation completed after five minutes; no force-cancel was issued. GitHub did not publish that job log, including in the full workflow archive, so the cause is unconfirmed. The next head requires a fresh passing frontend run.
