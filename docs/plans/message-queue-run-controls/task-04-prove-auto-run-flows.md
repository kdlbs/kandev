---
id: "04-prove-auto-run-flows"
title: "Prove Auto-run browser flows"
status: done
wave: 4
depends_on: ["03-add-auto-run-switch"]
plan: "plan.md"
spec: "../../specs/ui/requirements/message-queue-run.md"
---

# Task 04: Prove Auto-run browser flows

## Intent

Prove real desktop and phone behavior for finish-current pause, FIFO resume,
targeted Send Now resume, reload persistence, Cancel consistency, and
clarification safeguards.

## Acceptance

1. Desktop E2E toggles OFF while a slow response has A, B, C behind it, proves
   that response finishes, and proves no queued turn starts. Toggling ON then
   runs A, B, C as separate FIFO turns.
2. From OFF, desktop E2E uses row Send Now on B and proves Auto-run becomes ON,
   B replaces the captured turn without ordinary Cancel effects, and A then C
   run as separate turns without a second queue-control click.
3. Reload E2E proves OFF and the held backlog survive browser state loss.
   Explicit Cancel with a backlog projects OFF and ON resumes it.
4. Clarification E2E proves the switch may report ON without bypassing the
   pending clarification.
5. Pixel 5 E2E repeats OFF, reload, ON, and targeted resume through touch;
   verifies a 44-pixel effective target and zero document horizontal overflow.
6. No E2E uses removed header `queue-drain-next` or `queue-send-now` selectors;
   per-row `queue-entry-send-now` remains covered.

## Verification

```bash
cd apps
pnpm install --frozen-lockfile
pnpm --dir web e2e:run --project chromium tests/session/pause-resume-recovery.spec.ts tests/chat/message-queue.spec.ts tests/chat/clarification.spec.ts -- --grep 'Auto-run|Send Now resumes' --retries=0
pnpm --dir web e2e:run --project mobile-chrome tests/session/mobile-pause-resume-recovery.spec.ts tests/chat/mobile-message-queue-management.spec.ts tests/chat/mobile-clarification.spec.ts -- --grep 'Auto-run|Send Now resumes' --retries=0
if rg -n 'queue-drain-next|data-testid="queue-send-now"' web/e2e; then exit 1; fi
```

Confirm each Playwright command discovers the intended test count before
recording it as evidence. The managed runner owns production builds and
teardown.

## Files likely touched

- `apps/web/e2e/tests/session/pause-resume-recovery.spec.ts`
- `apps/web/e2e/tests/session/mobile-pause-resume-recovery.spec.ts`
- `apps/web/e2e/tests/chat/message-queue.spec.ts`
- `apps/web/e2e/tests/chat/mobile-message-queue-management.spec.ts`
- `apps/web/e2e/tests/chat/clarification.spec.ts`
- `apps/web/e2e/tests/chat/mobile-clarification.spec.ts`
- queue-specific E2E helpers if causal setup needs a reusable slow-turn or
  transcript-order assertion

## Dependencies

Task 03. Final copy, selectors, status projection, and responsive layout must
exist before browser proof.

## Parallelism

Sequential. Tests consume Task 03 and produce the evidence required by public
documentation.

## Inputs

- Spec scenarios for OFF, ON, Send Now, persistence, Cancel, clarification,
  transfer safety, and mobile parity.
- Existing queue and cancellation recovery E2E files listed above.
- `/e2e` managed-runner, causal-wait, mobile-discovery, and cleanup rules.

## Risks

- Fast mock turns can reserve the next row before OFF. Use a deliberately slow
  active response and wait for the acknowledged switch result, not sleeps.
- Prove separate transcript turns and B, A, C order, not just final queue
  emptiness.
- Reload persistence must refetch backend state rather than retain an in-memory
  store value.
- `mobile-chrome` discovers only `mobile-*.spec.ts`; keep paths aligned.

## Output contract

Report exact discovered/passed counts, production-build and teardown evidence,
files changed, artifacts on failure, blockers, and residual risks. Set this
task to `done`, record results below, and synchronize `plan.md` before Task 05.

## Results

- Desktop managed E2E discovered and passed 4/4 Chromium scenarios: pending
  clarification, targeted Send Now with B/A/C separate-turn order, OFF across
  current-turn completion and reload with FIFO resume, and explicit Cancel
  parking a backlog OFF.
- Mobile managed E2E discovered and passed 3/3 `mobile-chrome` scenarios:
  clarification deferral, targeted Send Now order with touch geometry and no
  horizontal overflow, and Cancel/OFF persistence across reload with resume.
- The first desktop run exposed that queue-status broadcasts omitted the
  persisted `auto_run` value. Backend publisher contract tests failed first;
  both queue publisher paths now emit complete `auto_run` and
  `merge_enabled` projections, and the focused Go tests pass.
- The production backend and Vite assets were rebuilt by the managed runner.
  Final no-build reruns used those artifacts; every managed run completed its
  owned teardown.
- `rg` found no remaining E2E references to removed `queue-drain-next` or
  header `queue-send-now` selectors. Per-row `queue-entry-send-now` remains
  covered on desktop and mobile.
