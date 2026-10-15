---
id: "09-cancel-recovery-visibility"
title: "Expose recovery after uncertain cancellation"
status: in_progress
wave: 9
depends_on:
  - 08-native-async-turn-ownership
plan: plan.md
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-008
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-008.5
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
---

# Task 09: Cancellation recovery visibility

## Evidence and correction

A timed-out cancellation leaves WAITING_FOR_INPUT, an empty error message and
an open agent_delivery unknown_prompt_outcome block bound to a submission.
Composer props disabled input but excluded this block from uncertain-delivery
presentation. No independent error card existed. Resume eligibility also only
recognized empty generic legacy blocks when recovery metadata was absent.

Use the current delivery block as presentation authority. Expose explicit Resume
for bound unknown outcomes without active recovery metadata, retaining guarded
backend recovery and existing reconnect behavior. A historical restored or
settled snapshot must not hide a current block. No prompt replay, live database
mutation, recovery authorization change, or new UI copy.

## Desktop and phone composition

Reuse SessionStoppedBanner and RecoveryActions, with the existing shared state,
inline desktop actions and stacked phone touch targets. No new scroll owner,
navigation, overlay, or viewport sizing.

```text
Before: Turn cancelled | disabled composer
Desktop: Outcome uncertain | Resume | Retry connection | Stop
Phone:   Outcome uncertain
         [ Resume ]
         [ Retry connection ]
         [ Stop ]
```

## Validation

Five focused regressions failed before correction: missing/historical metadata
presentation, bound-block Resume eligibility, and the real banner action.
All 46 focused Vitest tests, scoped ESLint, documentation catalog and spec lint
pass. Full TypeScript checking exhausted the default Node heap after dependency
sync; do not increase local memory. CI owns typechecking.
Extend native-resume-notice fixtures for the exact bound block with an empty
stream and no error metadata; desktop/mobile CI must verify Resume, retirement,
same native identity, no old prompt replay, reload and a new prompt.
No local browser or E2E execution, per user instruction. CI and review pending.
