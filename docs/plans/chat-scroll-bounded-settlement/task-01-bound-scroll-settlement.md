---
id: "01-bound-scroll-settlement"
title: "Bound scroll settlement"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-CHAT-MOTION-003
acceptance_criteria:
  - AC-UI-CHAT-MOTION-003.1
  - AC-UI-CHAT-MOTION-003.2
  - AC-UI-CHAT-MOTION-003.4
system_design:
  - ../../specs/ui/system-design/chat-motion.md
---

# Task 01: Bound Scroll Settlement

## Summary

Stop the follow driver when browser rounding prevents an exact target match.
Prove bounded work without changing existing scroll ownership or later content following.

## In scope

- Deterministic RED regression with fractional `scrollTop` readback on either side of the integer target.
- A 1 px settlement tolerance and termination at the current 180 ms interpolation endpoint.
- A clamped-write regression that proves the deadline stops retries.
- Coverage for later growth, interruption, cancellation, and idle geometry reads.
- Desktop and phone checks through the existing chat-motion browser scenarios.

## Out of scope

MacOS memory-growth closure, stylesheet changes, inspector enablement, store
retention, desktop process supervision, and new settings or telemetry.

## Acceptance

1. Fractional readback settles within 300 ms and 2 px, with no pending follow
   callbacks or further geometry reads during idle. Clamped writes terminate by
   the interpolation endpoint even when the requested target is unreachable.
2. Later content restarts following. Continuous growth advances, requests coalesce,
   and reader input, hidden panels, unmounts, and disabled motion retain their behavior.
3. Targeted unit tests and fresh-build desktop/phone scenarios pass. Results
   describe the fixed defect without claiming that macOS memory growth is resolved.

## Implementation sequence

1. Mark this work order `in_progress` after a later explicit implementation request.
2. Extend the existing driver fixture with quantized/clamped scroll setters.
3. Add `settles when the browser cannot represent the exact bottom target`.
4. Run that test against the original source and record the expected failure.
5. Correct `createChatScrollMotion` according to the design's settlement section.
6. Add deadline, later-growth, and idle-read assertions. Preserve existing tests.
7. Extend the shared browser scenarios with fractional geometry and bounded idle observation.
8. Run the commands below and record every result. Mark the work order and plan complete only after required checks pass.

The [temporary probe](evidence.md#repeatable-browser-probe) demonstrates the source
defect without changing production files. It does not replace permanent regression tests.

## ASCII UI preview

UI-01: Settled transcript. See the [combined preview](plan.md#ascii-ui-preview).

```text
Desktop chat panel          Phone full-height chat
+--------------------+      +--------------------+
| Latest message     |      | Latest message     |
| Bottom within 2 px |      | Bottom within 2 px |
+--------------------+      +--------------------+
| Composer           |      | Composer/safe area |
+--------------------+      +--------------------+
Follow work stops after settlement in both views.
```

The transcript owns scrolling. Existing controls, focus, touch targets, and safe
areas remain intact. This view maps to AC-UI-CHAT-MOTION-003.1, .2, and .4.

## Verification

Run from the repository root. The managed browser runner builds current assets.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/task/chat/chat-scroll-motion.test.ts components/task/chat/use-chat-scroll-motion.test.tsx components/task/chat/transcript-auto-scroll.test.ts)
(cd apps/web && pnpm exec eslint components/task/chat/chat-scroll-motion.ts components/task/chat/chat-scroll-motion.test.ts components/task/chat/use-chat-scroll-motion.test.tsx e2e/tests/chat/chat-motion-helpers.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/chat-motion.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-chat-motion.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Also repeat the isolated Chromium/WebKit probe in `evidence.md`. If macOS becomes
available, verify a noninteger desktop zoom and capture a settled idle sample.
That optional platform evidence does not replace the required unit and browser checks.
The full day-long memory incident remains a separate investigation.

## Files likely touched

- `apps/web/components/task/chat/chat-scroll-motion.ts`
- `apps/web/components/task/chat/chat-scroll-motion.test.ts`
- `apps/web/components/task/chat/use-chat-scroll-motion.test.tsx`
- `apps/web/e2e/tests/chat/chat-motion-helpers.ts`
- Existing E2E entry points: `chat-motion.spec.ts` and `mobile-chat-motion.spec.ts` in the same directory.
- This work order and `plan.md` for status and results.

## Dependencies

None.

## Risks

Fractional values require behavioral assertions. A DOM fixture that accepts
every write exactly cannot detect this defect. Global animation counts can
misattribute unrelated status motion to the follow driver.

## Parallelism

`sequential`

## Inputs

- [Chat motion requirements](../../specs/ui/requirements/chat-motion.md), requirement 003.
- [Chat motion design](../../specs/ui/system-design/chat-motion.md), scroll integration and bounded settlement.
- [Investigation evidence](evidence.md).
- Existing driver, hook, and shared desktop/phone browser tests.

## Results

- The regression suite failed against the original exact-equality driver for
  fractional readback, clamped writes, and later-growth resumption.
- The corrected driver passes all 21 focused tests. The work-order unit command
  passes 59 tests. Scoped ESLint and `pnpm run typecheck` pass.
- Fresh managed Chromium and mobile-chrome runs each pass both chat-motion
  scenarios. The zoomed transcript reaches the bottom, later growth resumes
  following, and no transcript geometry reads occur during the idle windows.
- The unmodified Chromium driver added 21 geometry reads during the stream idle
  window. The corrected driver added none.
- Catalog validation, specification lint, public-doc validation, and diff checks pass.
- macOS long-uptime memory validation remains pending. This repair does not
  establish the cause of the issue 4100 memory report.
