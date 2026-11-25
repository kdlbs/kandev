---
id: "03-resume-resolves-identity-less-blocks"
title: "Resume resolves identity-less delivery blocks"
status: done
wave: 2
depends_on: ["02-log-delivery-recovery-refusals"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
acceptance_criteria:
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.9
  - AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12
system_design:
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
---

# Task 03: Resume resolves identity-less delivery blocks

## Summary

The existing **Resume** action must keep the current ACP session and clear an
`unresolved_durable_work` block whose only unresolved work is an
`interrupted_unknown` prompt, including a block with no submission or stream
identity. No new UI control, no new native conversation, no resend.

## Fixture (from session `cbf39cbf`)

- Block: `agent_delivery`, `unresolved_durable_work`, `open`, incarnation `I`,
  generation 1, empty `delivery_submission_id` / `delivery_stream_id`,
  sequence 0.
- SQL submissions: one `interrupted_unknown` prompt (`{"text":"continue"}`,
  generation 1, terminal sequence 0); all earlier rows `completed` or
  `cancelled` with terminal sequences.
- `harness_session_generations`: generation 1, native session `N`; the resumed
  execution reports native session `N`.
- Cursor: received = projected (no projection lag).
- Peer journal: two variants, (a) holds the submission record unretired with
  matching hash, (b) has no record for that ID.

## Regression test (write first, must fail)

New file `apps/backend/internal/orchestrator/native_resume_identityless_block_test.go`
(plus a lifecycle-level test file if the guard lives there), using real
temporary SQLite and the existing fake agentctl/journal helpers from
`native_resume_generic_block_test.go` and `native_resume_proof_test.go`:

- `TestExplicitResumeResolvesIdentityLessBlock_PeerRecordPresent` (variant a).
- `TestExplicitResumeResolvesIdentityLessBlock_PeerRecordMissing` (variant b).
- Each asserts after explicit Resume: block `resolved` with authorized action
  `resume`; submission stays `interrupted_unknown` with payload retained; the
  peer journal reports the ID retired (duplicate admission of the ID fails);
  native session ID unchanged; no prompt dispatched; session
  `WAITING_FOR_INPUT`.
- Negative cases keep the block open: a `dispatching` or `accepted` row; a
  generation-2 harness row; a native identity mismatch.

First run the tests against current code and record which Task 02 refusal
reason fires. That confirms the guard.

## Implementation

Fix the confirmed guard at
`reconcileNativeResumeSQLDispatch` (`native_resume_dispatch_reconciliation.go:43`)
and carry a missing-peer interrupted prompt through the process-owner
retirement boundary. Only the not-found branch for an original SQL
`interrupted_unknown` row may proceed. It must write a retired-unknown peer
record for that exact ID under dispatch serialization before SQL block
resolution.

- A missing peer record for an original `dispatching`, `prepared`, or
  `accepted` row remains refused.
- A present peer record that disagrees on payload hash, session, incarnation,
  or generation remains refused as `peer_record_mismatch`.
- Preserve refusals for live work, newer generations, and native identity
  mismatches.
- Verify the retired-unknown record after journal reopen, including that it
  cannot be replayed and backend adoption does not count it as active work.

This scope amendment was approved by the task coordinator. It follows the
existing system-design section "Identity-less incident blocks"; it does not
add a recovery policy.

## Out of scope

- Frontend. The existing Resume button already routes through
  `session.recover` action `resume`.
- Automatic page-open resolution of delivery blocks.

## Acceptance

- New tests pass with `-race`; they failed before the fix with the recorded
  refusal reason.
- Existing native-resume, generic-block and silent-restore tests pass
  unchanged.
- Manual check on the live install: clicking Resume on task `fc7cfeee` clears
  the banner and keeps ACP session `01a1ccb6-86db-7033-abf2-6f232e14c06b`.

## Investigation result

- The exact SQLite and journal fixture passed with the peer record present.
- With the peer record absent, Task 02 logged `peer_record_mismatch` from
  `reconcileNativeResumeSQLDispatch` because no peer row matched the SQL
  `interrupted_unknown` row.
- The coordinator approved a narrower amendment: only a missing peer record
  for an original SQL `interrupted_unknown` row may proceed through the
  process-owner retirement boundary. Missing records for `dispatching`,
  `prepared`, or `accepted` rows and all present-record identity/hash
  mismatches remain refused.
- The missing SQL row is carried as provisional evidence through complete
  inventory validation. The existing retirement endpoint asks the process
  owner to create a payload-free retired-unknown record under dispatcher
  serialization before SQL block resolution.
- After journal reopen, the ID remains non-replayable and backend adoption
  excludes the row from active work. The regression suite also covers a
  present record with a different payload hash, missing peer records for live
  SQL states, and a newer SQL generation.
- The explicit Resume fixture resolves the identity-less block with action
  `resume`, keeps the same native session, leaves the prompt
  `interrupted_unknown` with its payload retained, dispatches no prompt, and
  returns the session to `WAITING_FOR_INPUT`.
- The requested live-install check could not run: the current local backend
  does not contain a task with the supplied `fc7cfeee` prefix. Automated
  SQLite, process-owner, and orchestrator fixtures cover the behavior.
- Review fixes: refusal reasons now distinguish missing execution, unavailable
  SQL evidence, and Office-owned recovery; catch-all adoption sentinels no
  longer guess a peer-record reason. SQL generation comparison explicitly
  ignores generation zero. Client errors retain only the recovery-block reason.
- Review fixes: the work-order test command now selects the identity-less
  explicit Resume fixture by its exact Go test name.

## Commands

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/orchestrator/ -run 'IdentitylessIncidentBlock|IdentityLess|NativeResume|GenericBlock|Refusal' -count=1
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'NativeResume|LoadedNative|IdentityLess' -count=1
go test -trimpath -tags fts5 -race ./internal/agentctl/server/process/ -run 'NativeResume' -count=1
go test -trimpath -tags fts5 -race ./internal/agentctl/server/api/ -run 'InterruptedRetirement|NativeResumeCanRetireMissing|NativeResumeDoesNotCreateMissing' -count=1
go test -trimpath -tags fts5 -race ./internal/agentctl/journal/ -count=1
go test -trimpath -tags fts5 ./internal/task/repository/sqlite/ -run 'NativeResume|RecoveryBlock|SilentRestore' -count=1
cd ../.. && make -C apps/backend build
cd ../.. && bash scripts/lint-go-changed <changed-go-files>
```
