---
id: "02-classify-cursor-stream-resets"
title: "Classify Cursor stream resets"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.14
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 02: Classify Cursor Stream Resets

## Summary

Add a dedicated deterministic fingerprint for Cursor's safe stream-reset
diagnostic and map it to the existing transient transport-loss cause. Preserve
the generic transport matcher and the context-cancellation safety veto.

## In scope

- Dedicated runtime rule and stable classifier rule ID.
- Exact positive, prose and partial negatives, and collision tests.
- Full classification invariants and transient helper coverage.
- Explicit `[canceled]` non-veto coverage.

## Out of scope

- ACP event detection or suppression.
- Retry scheduling or effect-safety changes.
- Widening `transportLostRe`.

## Acceptance

- The stable emitted diagnostic classifies as high-confidence
  `CodeAgentTransportLost` with class `ClassTransient` and `AutoRetryable=true`.
- The rule does not match prose or incomplete transport tokens and does not
  change the priority of more specific existing rules.
- `[canceled]` remains distinct from context cancellation, while existing
  context-cancelled composite envelopes remain non-retryable.

## Verification

```bash
cd apps/backend && go test -race ./internal/agent/runtime/routingerr -run 'Test(ClassifyCursorRetriable|MatchRuntimeEnvironmentRules_CursorRetriable|IsTransientProviderError_Cursor)'
```

## Files likely touched

- `apps/backend/internal/agent/runtime/routingerr/runtime_rules.go`
- `apps/backend/internal/agent/runtime/routingerr/cursor_retriable_stream_reset_test.go`

## Dependencies

None.

## Risks

- An overly broad expression can classify user-authored prose or unrelated
  cancellation output as transport loss.
- Rule ordering can accidentally supersede overload or resume-corruption
  fingerprints.

## Parallelism

`parallel-safe` with Task 01. The files are disjoint.

## Inputs

- Provider Error Recovery criterion `.14`.
- Cursor classification section in the system design.
- Existing transport-loss rule and tests.

## Results

- Added `cursor.retriable_stream_reset.v1` with complete normalized-diagnostic
  matching and the existing cancellation veto.
- Verification passed:
  `cd apps/backend && go test -race ./internal/agent/runtime/routingerr -run 'Test(ClassifyCursorRetriable|MatchRuntimeEnvironmentRules_CursorRetriable|IsTransientProviderError_Cursor)'`.
