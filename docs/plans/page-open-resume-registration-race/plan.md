---
created: 2026-11-13
status: done
requirements:
  - REQ-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001
  - REQ-EXECUTORS-RUNTIME-SELECTION-002
  - REQ-PLATFORM-DURABLE-AGENT-DELIVERY-006
system_design:
  - ../../specs/executors/system-design/runtime-selection.md
  - ../../specs/platform/system-design/interrupted-prompt-recovery.md
legacy_specs: []
---

# Implementation Plan: Seamless resume after restart

## Overview

After a backend restart, opening task `78444a94` showed "Session recovery failed:
Workspace restored in read-only mode". The automatic resume was refused with
`selected executor runtime "standalone" has no verifiable runner identity:
retained execution identity does not match`, and the session moved from
`WAITING_FOR_INPUT` to `FAILED`.

Intended behavior (user-confirmed): a restart starts no agent by itself, but
opening the task page resumes the session seamlessly whenever possible
(`AC-AGENTS-AGENT-RESUME-RUNTIME-RECOVERY-001.10`).

A second defect blocks manual Resume on task `fc7cfeee` (session `cbf39cbf`,
ACP `01a1ccb6`): Resume reattaches the same Codex conversation, but the delivery
recovery block stays open, so the banner remains and prompts stay refused.
Intended behavior (user-confirmed): the single existing **Resume** action keeps
the current ACP session and implicitly settles an interrupted prompt of unknown
outcome. No new recovery button, and no new native conversation
(`AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.12`, amended).

## Evidence and root cause: registration race (Task 01)

| Time (UTC) | Event |
| --- | --- |
| 18:53:11.706 | Shutdown keeps the `executors_running` row for session `68e5e2bf`, naming execution `7b198c37`. |
| 19:00:23.864 | Task page opens; workspace handlers call `GetOrEnsureExecution`, which starts creating execution `ff21d9b9`. |
| 19:00:23.996 | Automatic resume begins (`origin: automatic`). |
| 19:00:24.011 | Row rewritten to `ff21d9b9` (`updated_at`). |
| 19:00:24.015 | Session `WAITING_FOR_INPUT → FAILED`; `failed to launch session ... retained execution identity does not match`. |

`registerAndPublishCreatedExecution` (`lifecycle/manager_execution.go`) calls
`executionStore.Add` before `persistExecutorRunningResult`. The orchestrator's
resume admission (`orchestrator/executor/executor_runtime_selection.go`,
`admitRetainedExecutionRuntime`) calls `Manager.ValidateExecutionRuntime`
outside the session's coalesced slot and without the execution's
`remoteInstanceLifecycleMu`. In that gap it sees the new in-memory execution and
the predecessor's row, and `validateRetainedRow`
(`lifecycle/manager_runtime_selection.go`) refuses the mismatch.

The comparison is right to fail closed on a settled mismatch. The defect is that
it judges a registration that has not finished.

## Evidence and root cause: identity-less block (Tasks 02 and 03)

| Store | Evidence |
| --- | --- |
| `session_recovery_blocks` | `7fcf8d55`: `unresolved_durable_work`, `open`, consumer `agent_delivery`, incarnation `9ac66109`, generation 1, **empty** submission ID, stream ID and sequence 0. Created 2026-11-12 11:14:58. |
| `agent_delivery_submissions` | `prompt:adb41134` (`{"text":"continue"}`), `interrupted_unknown`, incarnation `9ac66109`, generation 1, terminal sequence 0. All older rows are terminal. |
| `harness_session_generations` | Generation 1, native session `01a1ccb6`, equal to the resumed ACP session. |
| Backend log 19:11:21 | Native start and boot succeed, then `agent.boot_ready: delivery recovery remains required` (DEBUG, no reason). |

`prepareBootDeliveryReadiness` and `reconcileUnresolvedDeliveryProtocol`
(`orchestrator/native_resume_readiness.go`) previously discarded the refusal
error. Task 02 instrumentation reproduced the refusal as `peer_record_mismatch`
at `reconcileNativeResumeSQLDispatch`: the SQL row was `interrupted_unknown`,
but its peer journal record was absent. The logs from block creation have
rotated, so the origin of the block is not reconstructed.

## Scope

### In scope

- A per-execution registration-settled signal and a context-bounded wait in
  `ValidateExecutionRuntime`, per the runtime-selection design section
  "Registration must settle before comparison".
- A regression test that reproduces the gap with a channel barrier.
- WARN logging of a closed refusal reason when Resume or boot-ready leaves a
  delivery block open (Task 02).
- Explicit Resume resolving identity-less `unresolved_durable_work` blocks per
  the design section "Identity-less incident blocks" (Task 03).
- For Task 03, a missing peer record is eligible only for an original SQL
  `interrupted_unknown` row with matching session/incarnation evidence and no
  newer generation. The process owner writes its payload-free retired-unknown
  record under dispatch serialization before block resolution.
- Missing peer records for `dispatching`, `prepared`, or `accepted` rows,
  present-record identity/hash mismatches, live work, newer generations, and
  native identity mismatches remain refused.

### Out of scope

- Any startup resume, sweep, or drain. A restart still starts no agent.
- Frontend recovery UI. Once resume succeeds, no banner appears. No new
  recovery control; removing existing ones is a separate UX decision.
- Automatic (page-open) resolution of delivery blocks. Automatic open stays
  subject to recovery admission (`AC-PLATFORM-DURABLE-AGENT-DELIVERY-006.8`).
- Relaxing the identity predicate for settled rows.
- Repairing sessions already in `FAILED`; **Resume session** works for them now.

## Work orders

| Task | Title | Status | Depends on |
| --- | --- | --- | --- |
| [01](task-01-settle-registration-before-runtime-check.md) | Settle registration before runtime comparison | done | none |
| [02](task-02-log-delivery-recovery-refusals.md) | Log why delivery recovery stays blocked | done | none |
| [03](task-03-resume-resolves-identity-less-blocks.md) | Resume resolves identity-less delivery blocks | done | 02 |

## Verification

Backend package tests that initialize the complete SQLite schema require the
`fts5` build tag used by the backend Makefile; verification commands include it.

```bash
cd apps/backend
go test -trimpath -tags fts5 -race ./internal/agent/runtime/lifecycle/ -run 'RegistrationSettle|ValidateExecutionRuntime|NativeResume|LoadedNative|IdentityLess' -count=1
go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle/ ./internal/orchestrator/executor/ -count=1
go test -trimpath -tags fts5 -race ./internal/orchestrator/ -run 'DeliveryRecovery|NativeResume|IdentityLess|GenericBlock|Refusal|PrepareBootDelivery' -count=1
go test -trimpath -tags fts5 -race ./internal/agentctl/server/process/ -run 'NativeResume' -count=1
go test -trimpath -tags fts5 -race ./internal/agentctl/server/api/ -run 'InterruptedRetirement|NativeResumeCanRetireMissing|NativeResumeDoesNotCreateMissing' -count=1
go test -trimpath -tags fts5 -race ./internal/agentctl/journal/ -count=1
go test -trimpath -tags fts5 ./internal/task/repository/sqlite/ -run 'NativeResume|RecoveryBlock|SilentRestore' -count=1
cd ../.. && make -C apps/backend build
cd ../.. && bash scripts/lint-go-changed <changed-go-files>
```

## Results

- Task 01: registration settlement now gates runtime identity comparison; the
  channel-barrier regression and lifecycle/orchestrator package tests pass.
- Task 02: boot-ready and explicit Resume emit a WARN with a closed refusal
  reason when delivery recovery remains blocked; refusal-log tests pass.
- Task 03: explicit Resume resolves the identity-less incident block after the
  process owner retires the missing peer record for an original
  `interrupted_unknown` row. The hash-mismatch, live-state, newer-generation,
  identity-mismatch, journal-reopen, backend-adoption, and Resume behavior tests
  pass.
- Review follow-up: recovery causes are hidden from client errors and logged
  server-side; refusal reasons are specific and closed, and boot-ready uses
  DEBUG when an explicit Resume attempt already owns the refusal warning.
- Changed-Go lint passes with zero issues, and `make -C apps/backend build`
  passes. The build reports that `codesign/rcodesign` is unavailable and leaves
  Darwin artifacts unchanged.
- The manual live-install check for task `fc7cfeee` was unavailable because
  that task is not present in the local backend; automated fixtures cover the
  scenario.

## Risks

- A registration that never settles would stall the validator. Every exit of
  `registerAndPublishCreatedExecution` must close the signal (use `defer`), and
  the wait honors the caller's context.
- Rollback removes the execution from the store; the validator must re-read the
  store after waiting rather than reuse the stale pointer.
- Task 03 only accepts a missing peer record for the original SQL
  `interrupted_unknown` row, and only after the process owner retires it under
  dispatch serialization. Missing records for other SQL states, present-record
  mismatches, live work, newer generations, or native identity mismatches keep
  the block open.
