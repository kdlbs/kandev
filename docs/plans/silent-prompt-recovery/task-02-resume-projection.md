---
id: "02-resume-projection"
title: "Wait for resume projection"
status: pending
wave: 2
depends_on:
  - "01-silent-turns"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.5
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.25
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.26
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 02: Wait for resume projection

## Summary

Allow one explicit Resume to wait up to 10 seconds for authenticated received
events to finish projection. Rebuild recovery evidence before resolving a block.

## In scope

- Introduce a typed internal projection-lag result and a bounded,
  context-aware wait at `GetNativeResumeDeliveryEvidence`.
- Preserve all current owner, journal, submission, payload, native identity,
  cursor and transactional resolution checks. Do not compare error text.
- Add lifecycle tests with deterministic timing and real SQL/journal
  orchestrator integration, then desktop and phone E2E for one-click Resume.
- Update the public recovery guidance introduced by Task 01.

## Out of scope

Changing historical outcomes, sending old prompts, bypassing live work,
extending the browser's existing 60-second request timeout, or adding controls.

## Acceptance

1. Verified lag that clears within the 10-second budget succeeds from one
   Resume request with current proof, preserves native identity and unknown
   history, and never dispatches the interrupted instruction.
2. Deadline, cancellation, changed ownership, unavailable storage, conflicting
   evidence, or live work leaves recovery blocked. A late result cannot update
   a successor or resolve an unrelated block.
3. Desktop and phone show existing busy state, then success or existing failure
   feedback. A repeated request cannot release work twice.

## UI preview

[UI-02 in the plan](plan.md#ui-preview): the existing **Resume** action becomes
busy on activation and remains busy while projection catches up. Success
restores normal input. Timeout retains the blocked composer and recovery
feedback. Desktop and phone share the same state and controls.
Platform criteria .25 and .26 own this flow.

## Verification

Run from the repository root. Add failing tests first; use a controllable
projection barrier instead of a real 10-second sleep.

```bash
(cd apps/backend && go test -race ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'NativeResume|NativeDeliveryRecovery|ProjectionWait|RecoverSessionUnknownPrompt' -count=1)
(cd apps/backend && go test -race ./internal/task/repository/sqlite -run 'NativeResume|DeliveryRecovery' -count=1)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --host --project chromium e2e/tests/session/native-resume-projection.spec.ts)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-native-resume-projection.spec.ts)
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with actual changed Go paths. If frontend
logic or copy changes, add its targeted unit, eslint, and i18n commands to
Results and run them. A “no tests to run” package is not verification evidence.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/native_resume_proof.go`
- New lifecycle `native_resume_projection_wait_test.go`
- `apps/backend/internal/orchestrator/session_delivery_recovery.go` only if
  necessary to preserve context/ownership at the proof boundary
- New orchestrator `native_resume_projection_wait_test.go`
- Existing `native_resume_proof_test.go`, `native_resume_readiness_test.go`,
  and `session_native_delivery_recovery_test.go`
- New desktop and phone native-resume-projection E2E files and fixture support
- `docs/public/agents-and-profiles.md`

## Dependencies

Task 01. Sequence reuses its controlled mock and uncertain-cancellation fixture.

## Risks

Do not hold event projection, session cancellation, or stream locks across
the wait. Revalidate after every unlocked boundary. Retained client references
do not prove current execution ownership.

## Parallelism

sequential

## Inputs

- [Platform requirements](../../specs/platform/requirements/durable-agent-delivery.md)
- [Recovery design](../../specs/platform/system-design/interrupted-prompt-recovery.md)
- Existing `TestNativeResumeProofRejectsConcurrentStreamProgress` and
  `TestNativeResumeAcknowledgesRealJournalBeforeNewWork`.

## Results

Pending. No implementation or product verification performed.
