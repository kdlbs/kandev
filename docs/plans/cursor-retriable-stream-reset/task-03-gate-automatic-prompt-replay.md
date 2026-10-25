---
id: "03-gate-automatic-prompt-replay"
title: "Gate automatic prompt replay"
status: done
wave: 2
depends_on:
  - "01-project-cursor-terminal-evidence"
  - "02-classify-cursor-stream-resets"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-PROVIDER-ERROR-RECOVERY-001
acceptance_criteria:
  - AC-PLATFORM-PROVIDER-ERROR-RECOVERY-001.15
system_design:
  - ../../specs/platform/system-design/provider-error-recovery.md
---

# Task 03: Gate Automatic Prompt Replay

## Summary

Make the concrete interactive retry owner consume current prompt replay
evidence before it schedules the existing retry loop. Preserve same-provider
resume, current retry bounds, and dynamic routing's no-fallback invariant.

## In scope

- Provider-neutral prompt attempt evidence for concrete and dynamic sessions.
- Session, execution, and prompt-generation fencing.
- Output and tool-activity observation through the real stream path.
- Safe scheduling through the existing transient retry owner.
- Manual recovery for missing, stale, output-bearing, or tool-bearing evidence.
- End-to-end coverage from the Cursor structured diagnostic through classifier
  and retry decision.

## Out of scope

- A new retry loop, delay schedule, attempt cap, or persistent retry state.
- Dynamic candidate switching for `agent_transport_lost`.
- Cursor-specific orchestration branches.
- UI or localization changes.

## Acceptance

- A current, known, output-free and tool-free transport reset schedules the
  existing same-provider retry and no second owner.
- Missing, stale, output, or tool evidence cannot schedule automatic replay and
  falls through to the existing manual recovery surface.
- The observed sequence with thoughts and a `Read File` call is unsafe, while
  Cursor-owned progress before prompt settlement never creates a Kandev failure.

## Verification

```bash
(cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'Test(HandleTransientFailure.*Replay|PromptAttemptEvidence|CursorTransportLost)')
make -C apps/backend test
make -C apps/backend lint
(cd apps/backend && golangci-lint run ./... --new-from-rev="$(git merge-base HEAD origin/main)" --timeout=5m)
git diff --name-only -- '*.go' | xargs -r gofmt -l
```

## Files likely touched

- `apps/backend/internal/orchestrator/dynamic_evidence.go`
- `apps/backend/internal/orchestrator/service.go`
- `apps/backend/internal/orchestrator/task_operations.go`
- `apps/backend/internal/orchestrator/event_handlers_streaming.go`
- `apps/backend/internal/orchestrator/event_handlers_transient.go`
- `apps/backend/internal/orchestrator/event_handlers_transient_replay_safety_test.go`

## Dependencies

- Task 01 supplies the structured Cursor provider error.
- Task 02 supplies the transient transport-loss classification.

## Risks

- Evidence initialized too late can turn a safe no-output failure into unknown
  and manual recovery.
- Evidence retained too long can authorize or block the wrong prompt generation.
- Existing transient tests must explicitly establish safe attempt evidence after
  the fail-closed gate is applied.

## Parallelism

`sequential` after Tasks 01 and 02.

## Inputs

- Provider Error Recovery criterion `.15`.
- Cursor retry-safety section in the system design.
- Existing dynamic attempt evidence and transient retry owner.
- Completed Task 01 and Task 02 contracts.

## Results

- Generalized process-local prompt evidence to cover concrete and dynamic
  attempts with execution and prompt-generation fencing.
- Added lifecycle-owned terminal evidence so NATS delivery order between
  stream and failure subscriptions cannot authorize replay after prior activity.
- Registered the cached prompt and replay identity before model-switch startup,
  then bound it to the replacement execution.
- Automatic concrete replay now requires known, output-free, tool-free evidence
  and continues to use the existing same-provider retry owner and budget.
- Tightened Cursor matching to the complete normalized diagnostic and covered
  tool-update preservation, consumed-event handling, and timestamp capture.
- Verification passed:
  `cd apps/backend && go test -race -tags fts5 ./internal/orchestrator -run 'Test(HandleTransientFailure.*Replay|PromptAttemptEvidence|CursorTransportLost)'`.
- The remediation identity-fence suite passed, including the dynamic missing-record
  regression. The full lifecycle race suite passed with 2088 tests, and the full
  orchestrator race suite passed with 2261 tests.
- Backend lint and changed-lines lint passed with 0 issues. The full backend test target was
  attempted; only unrelated host-config and missing-fixture failures remained.
