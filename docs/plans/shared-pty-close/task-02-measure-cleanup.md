---
id: "02-measure-cleanup"
title: "Measure cleanup savings with retained coverage"
status: in_progress
wave: 2
depends_on: ["01-interruptible-close"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-004
  - REQ-PLATFORM-CI-PERFORMANCE-005
acceptance_criteria:
  - AC-PLATFORM-CI-PERFORMANCE-004.1
  - AC-PLATFORM-CI-PERFORMANCE-004.2
  - AC-PLATFORM-CI-PERFORMANCE-005.1
  - AC-PLATFORM-CI-PERFORMANCE-005.5
system_design:
  - ../../specs/platform/system-design/ci-performance.md
---

# Task 02: Measure cleanup savings with retained coverage

## Summary

Compare the corrected runtime against the parent snapshot without changing
E2E assertions, reset operations or worker capacity. Keep a local cleanup
improvement distinct from hosted critical-path performance.

## In scope

- Three sequential paired baseline/candidate samples on the same host with the
  same build/cache policy, instrumentation and complete file selection.
- Check exact case identities/outcomes and report all retries or failures.
- Run the files in both orders using explicit sequential invocations against
  the same reusable backend when the runner supports it; do not assume CLI
  argument order controls Playwright execution order. Preserve normal resets.
- Restore temporary timing instrumentation byte-for-byte before committing.
- Update the CI audit and follow-up PR with hosted results after delivery.

## Out of scope

Shorter stop timeouts, missing cleanup, weakened isolation, more workers,
suppressed failures, new runtime timing hooks and any claim that this fix alone
halves E2E execution.

## Acceptance

1. All paired complete-file outcomes agree. Median reset time improves without
   new retries; disclose total suite duration and residual embedded-shell cost.
2. Mobile continuity remains covered. Hosted results distinguish execution,
   queue, total runner-minutes and workflow elapsed time using exact head/run/attempt.
3. Keep the fixed 2159-second longest-normal-E2E reference and 1079.5-second
   target from PR #4440. Frontend savings do not count toward the user's E2E
   target. Do not replace the reference with a slower convenient run.

## Verification

From repository root, run these guarded commands sequentially. The sample
commands name complete files; repeat paired runs three times, rebuilding the
matching source each time. Never overlap full suites.

```bash
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/chat/message-queue.spec.ts tests/chat/last-prompt-scroll.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/session/mobile-transient-turn-runtime-continuity.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

After verified delivery, use `scripts/pr-await <follow-up-PR> --deadline-min 120 --format json`
and capture job steps and test artifacts for its exact new head. Parent PR
#4440 is merged. A pending or cancelled run is not measured success.

## Files likely touched

- `apps/web/e2e/fixtures/test-base.ts` temporarily for operation timings only.
- `docs/audits/ci-half-runtime-2026-10-12.md`
- This plan and its work-order result sections.

## Dependencies

Task 01. Local regression evidence precedes delivery; native macOS CI must
pass before merge. A draft can carry the verified candidate to that CI gate.

## Risks

Shared runner load varies substantially. Parent fixture steps include nested
operations; do not double-count them or add concurrent durations to wall time.

## Parallelism

`sequential`

## Inputs

- [CI requirements](../../specs/platform/requirements/ci-performance.md).
- [CI design](../../specs/platform/system-design/ci-performance.md).
- [Prior fixture investigation](../ci-performance/e2e-cost-follow-up.md).

## Results

The initial Linux implementation passed three alternating pairs with 29 identical
test identities/outcomes,
zero skips and zero retries. Each sample rebuilt through the guarded managed
runner with the same one-worker selection and temporary instrumentation.

| Pair | Baseline suite (s) | Candidate suite (s) | Baseline resets (s) | Candidate resets (s) |
| --- | ---: | ---: | ---: | ---: |
| 1 | 479.519 | 448.100 | 60.592 | 27.934 |
| 2 | 479.358 | 451.224 | 59.900 | 31.224 |
| 3 | 474.578 | 444.891 | 61.168 | 27.270 |

Median complete-suite duration is 479.358s versus 448.100s, **6.52% lower**.
Median per-run reset total is 60.592s versus 27.934s. Median per-run reset
median is 2581.10ms versus 584.05ms. Residual embedded-shell cleanup and other
shutdown waits remain; no whole-suite hosted percentage is established.

An earlier candidate was interrupted before completion and is excluded. Every
complete sample restored the fixture byte-for-byte; its final SHA-256 is
`68060f6654d5afc07edf7333a663187eeb14207ca02f45f163427e58f1ee950c`.
The later pairs use Go source overlays to rebuild the parent and candidate
without swapping the working production file. Make's command-line GOFLAGS
propagation was verified. The managed runner does not expose a supported
reverse-file order against the same reusable backend; CLI argument order was
not treated as execution-order proof. Normal per-test resets remained enabled.

Raw JSON reports, restoration receipts and parity checks remain under ignored
`apps/backend/.build/ci-half/e2e-focus/`; `paired-summary.json` records all six
samples. All four mobile continuity cases passed in 170.830s,
with zero skips or retries, including the shared desktop/phone exhaustion
journey. Web typecheck, full E2E discovery, changed-file ESLint, backend lint,
documentation validation and whitespace checks pass. Hosted execution and
native macOS compatibility remain pending.

The first native macOS run exposed a missing-poller initialization defect.
Task 01 corrects it with a newly registered duplicate descriptor. The numbers
above describe the initial Linux implementation at f5466aafe8, not the portable
revision. Fresh affected-suite verification and exact-head hosted timing are
pending for that correction; do not carry the prototype percentage forward as
a measured portable result.

Portable revision local verification passed: all 42 desktop cases across queue,
transcript and workflow-switch files, plus all four mobile continuity cases,
with zero skips or retries. This combined correctness run overlapped backend
lint and is not a controlled speed comparison. PTY race tests passed ten times;
login/shell suites and all interactive-runner, first-idle and PTY-dimension tests
passed. Full backend lint reports zero issues, Windows compilation passes, and
documentation validation passes. Hosted macOS and final E2E timing remain pending.

Native macOS compatibility run 38425537679 passes at 3314a962, including the
ten-repeat PTY race tests and the affected callers. Complete pre-concurrency E2E
run 38416314881 passed with a 2,178-second longest normal step. The later
three-process trial failed and is not portable-PTY performance evidence. The
50% E2E goal remains open; native compatibility is now verified.
