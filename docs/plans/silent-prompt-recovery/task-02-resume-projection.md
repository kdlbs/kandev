---
id: "02-resume-projection"
title: "Wait for resume projection"
status: done
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

- Introduce a distinct internal projection-lag result and a bounded,
  context-aware wait at `GetNativeResumeDeliveryEvidence`.
- Preserve all current owner, journal, submission, payload, native identity,
  cursor and transactional resolution checks. Do not compare error text.
- Add lifecycle tests with controlled projection and real SQL/journal
  integration, retain orchestrator transaction coverage, then desktop and phone
  E2E for one-click Resume.
- Add `TestNativeResumeProjectionWaitRejectsUnresolvedJournal`: authenticated
  unresolved journal work plus cursor lag returns blocked before any wait or
  scheduled retry.
- Add `TestNativeResumeProjectionWaitContinuousProgressExhaustsOriginalBudget`:
  repeated same-owner progress cannot reset the deadline or cause a tight loop.
  One complete proof with matching opening/closing status succeeds without
  waiting for an extra quiet polling interval.
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
python3 scripts/lint-spec-files.test.py
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
- Existing `native_resume_proof_status_test.go`, `native_resume_delivery_test.go`,
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

Implemented the bounded proof loop at `GetNativeResumeDeliveryEvidence`.
Distinct sentinels preserve existing blocked/stale error compatibility. A single
10-second budget and 200 ms cadence apply after verified lag or monotonic
same-owner stream progress, including on the first proof attempt. The caller can
shorten the deadline. Every attempt rechecks the execution and authenticated
client, then rebuilds the full proof. Client leases are released between
attempts so connection replacement can proceed during the wait.

The regression tests cover one-request success, unresolved journal rejection,
continuous progress, cancellation, deadlines, changed native/execution ownership,
storage failure, the 10-second cap, and client lease release. A real SQLite and
bbolt journal test exercises received/projected cursor divergence and the
transactional recovery resolver. It verifies one resolution, stable native
identity, preserved unknown history, and no active historical submission.
This test lives at the lifecycle boundary so it can use the real client without
adding a production API solely for an orchestrator test. Existing orchestrator
recovery tests cover the caller and transaction path.

Browser tests use an initialized, blocked native session and hold the Resume
request to check disabled/busy controls,
duplicate-click prevention, failure feedback, retry, and real native recovery.
The backend integration test controls actual SQL lag; browser fault injection
checks the request contract and does not claim to reproduce SQL timing.
The fixture uses a 5-second SQLite busy timeout for concurrent backend writes.
A startup-event race observed during verification was rejected by the final SQL
snapshot check. This final guard remains unchanged: evidence that changes after
the proof can still require another explicit Resume.

PR review identified an early return when the first proof observed valid stream
progress without prior cursor lag. That result now starts the same bounded wait.
Regressions cover first-attempt acknowledgement progress that settles, and both
initial lag and initial stream progress that continue until the caller deadline.
The deadline, polling cadence, owner checks, and final SQL guard are unchanged.
Focused lifecycle and caller race tests, changed-package Go lint, and both
desktop and both phone recovery cases passed after this correction. Browser
retries were disabled. Catalog validation and specification lint also passed.

Verification passed:

- The work-order race filter passed in lifecycle, orchestrator, and SQLite.
- The final lifecycle `NativeResumeProjectionWait|NativeResumeProof` race run
  passed after the client-lease and lint corrections.
- Changed-package Go lint: zero issues.
- After integrating current main, the focused lifecycle/orchestrator race
  filter also included slow native restore, stream retry, and cleanup.
- Final desktop and phone browser runs: 5 passed on each viewport, covering
  both work orders. The upstream equivalent SQLite timeout fix was retained.
- TypeScript, targeted ESLint, and 21 focused action/recovery UI tests passed.
- Public docs, catalog validation, specification lint, all 36 specification-linter
  tests, documentation coverage, and whitespace checks passed.

Browser setup defects found during verification were corrected: the status
selector now targets the busy message, failure feedback uses the existing
localized message, and SQLite fixture writes tolerate a bounded lock wait.
Desktop and phone cancellation screenshots were inspected before publication.
