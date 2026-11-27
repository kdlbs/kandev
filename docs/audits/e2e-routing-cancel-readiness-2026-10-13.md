# Executor routing and cancellation readiness

Full hosted run 38448127809 fails normal shards 6 and 12. The longest normal
step is 1,520 seconds, 29.60% below the fixed 2,159-second reference; summed
normal execution is 18,128 seconds. Twelve normal shards and all six container
shards pass. Final failures are the mobile 50,000-file viewport case and both
cancellation policies; executor routing passes only on its second retry. The
full artifact audit records 3,885 passed, 40 skipped, three failed and twelve
retry attempts. This is a failed run, not an accepted speed improvement.

## Executor settings

The first attempt checks the child expansion button with an immediate count
before it renders, skips the click, then waits for a profile beneath the still
collapsed branch. The failure snapshot contains that collapsed executor.
The retry reaches the legacy executor page but selects profile text globally;
both its navigation link and profile card match. The latter failure reproduces
locally with zero retries.

Wait for each actual expansion control, inspect its explicit expanded state,
and click only when collapsed. Select the existing unique profile-card test ID
on the legacy executor page. Keep sidebar navigation, card navigation, the
legacy bookmark redirect, saved image configuration, build action and unsaved
changes dialog assertions.

## Cancellation

The hosted disabled-policy retry passes its two UI assertions individually,
then fails the additional Node wall-clock assertion at 2,095 milliseconds.
The trace shows the first assertion consuming 1,994 milliseconds and the
second 97 milliseconds. Start both assertions together with their original
2,000-millisecond limits, instead of adding sequential assertion and transport
costs. Both the disappeared Cancel button and visible idle composer remain
required within the same concurrently started assertion window. The shared
helper still covers desktop and mobile; product cancellation behavior and
its documented two-second responsiveness requirement are unchanged.

The enabled-policy retry misses the exact start marker because both mock
messages have already arrived and merged into one paragraph. Its eight-second
mock turn can expire during hosted page setup. Keep this desktop mock turn
alive for 60 seconds so the real Cancel action owns completion. Cancellation
interrupts that delay; tests do not wait for it. Preserve enabled/disabled
workflow destination assertions and the single-session invariant. No retry
increase or production timeout change is made.

## Verification

The pre-fix complete desktop selection passes both cancellation cases and
fails the executor routing case at its ambiguous locator. In the subsequent
three-repeat desktop/mobile run, eleven cases pass and one first backend
fixture setup times out before entering its test. All six desktop cancellation
and three mobile cancellation iterations pass; both executed settings
iterations pass. Keep that failed setup result as failed evidence.

A fresh complete four-case desktop/mobile run passes with one worker and
zero retries in the same pinned CI image. Changed-file ESLint, Prettier and
web typecheck pass. Earlier host-restart interruption has no passing verdict. Logs and blobs remain under ignored `e2e-focus/latest-*`.
