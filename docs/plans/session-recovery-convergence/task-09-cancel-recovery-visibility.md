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

## CI remediation

CI caught the fallback banner displacing an existing crash recovery card and
hiding Start fresh session. Preserve the current card for a bound submission;
use the fallback when no card exists. Persisted active delivery recovery retains
priority. Three regression cases reproduced this failure locally.

The phone toolbar tests also exposed a stale workflow placement after startup:
the task detail refresh returned Review, but the canonical task used by Proceed
still held the boot-time In Progress step. Reconcile accepted detail reads into
the canonical overview using its existing read journal and timestamp guards.
The focused regression reproduces the missing action and preserves a newer live
move while the refresh is in flight. Existing desktop crash and phone toolbar
E2E assertions remain intact; CI owns execution.

The next CI run passed the crash and phone toolbar cases, but both new cancelled
recovery tests exposed a page-load race: rejected automatic resume and workspace
restore requests created a request-local generic card that displaced the block's
Resume/Retry/Stop controls. Track request-local model provenance and let persisted
session recovery cards, not automatic request failures, displace the fallback.
The regression failed before correction; 72 focused tests now pass. Desktop and
mobile E2E assertions are unchanged and remain CI-only.
