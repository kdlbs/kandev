---
id: "05-preserve-restart-idle-interval"
title: "Preserve the idle interval across backend restart"
status: done
wave: 5
depends_on: ["04-resume-safe-idle-reclaim"]
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-IDLE-PARKING-001
acceptance_criteria:
  - AC-EXECUTORS-IDLE-PARKING-001.6
  - AC-EXECUTORS-IDLE-PARKING-001.7
  - AC-EXECUTORS-IDLE-PARKING-001.8
  - AC-EXECUTORS-IDLE-PARKING-001.10
  - AC-EXECUTORS-IDLE-PARKING-001.11
system_design:
  - ../../specs/executors/system-design/idle-runtime-parking.md
---

# Task 05: Preserve the restart idle interval

## Outcome and scope

Carry the original P46 idle slice forward from candidate `3f46817f62307675d98b7bc2809bcafeb89819e3` (base `71688b0f178839052be1e19cfaf0afca23fc7e99`) onto base `4de689d338a496c8b5139ff3e308d43d318a0024`. The original fork slices are `52237290730452c6fec9501b879fd82829ad3acc` and the idle part of `d840571f2f`.

Preserve the idle interval from trustworthy activity, not passive row timestamps or replay time. Reuse the candidate's internal recovered-event distinction, clock guard changes, and bounded Info skip diagnostics. #4406 already supplies automatic focus origin; #4112 token protection remains. #4479 accepted-work and primary-ownership work is separate.

The candidate's transcript fallback is deliberately not carried forward. `GetLastMessageTimeBySessionIDs` measures `MAX(updated_at)` across all message types; metadata-only writes refresh it. Turn completion timestamps can also be rewritten on replay. Neither proves the last real idle activity. Instead, reuse lifecycle's existing activity clock and normal runtime-row persistence, the existing durable journal observation time, and the existing retained outcome slot.

## Responsibilities and exclusions

- Existing production paths: orchestrator `idle_session_reaper.go`; lifecycle `types.go`, `manager_events.go`, `manager_recovery_turn_outcome.go`; streams `agent.go`.
- Necessary existing owner seams: lifecycle `persistence.go` saves the clock through its normal upsert; `manager_lifecycle.go` restores it before local replay; `remote_recovery.go` uses the same helper with the authoritative runtime record for remote reconstruction; `executor_backend.go` keeps it only within the same session; `durable_delivery_stream.go` carries journal observation time; instance `turn_outcome.go` records retained observation time. These six additions replace an untrustworthy fallback and preserve executor coverage without adding per-event database writes.
- Preserve current-execution guards, active/known-work exclusions, resume tokens, provider coverage, policy/focus floors, activity evidence and epochs, apply-before-ACK, and ControlTurnID deduplication. Unknown or invalid history receives a full interval from observation.
- No new table, journal, endpoint, shared message timestamp rewrite, metric framework, model invocation, runtime operation, or limit change. No UI changes.
- Do not port the candidate's two added standing test files. Original #143 explicitly removed unauthorized standing idle tests; all existing upstream tests and fixtures remain. New verification cases exist only outside automatic collectors in an explicitly invoked Go overlay and are removed after their results are recorded.

## Implementation acceptance

1. Normal lifecycle persistence and recovery reconstruct the same execution's real activity clock; unrelated row/message metadata updates cannot reset it, and another session/execution cannot inherit it.
2. Retained and durable terminal recovery reach the existing completion path with their original observation time, preserving ACK/dedup and normal live-completion behavior.
3. No-history candidates get a full, stable interval; existing policy/focus floors, known-work, and token protections continue to operate.

## Verification commands

Run from `apps/backend` unless indicated otherwise. Exact selected commands and outcomes are appended after execution; no whole suite or permanent new case is required.

- `go test -trimpath -p 2 ./internal/agent/runtime/lifecycle -run 'TestApplyRecoveredTurnOutcomeAppliesAndAcks|TestApplyRecoveredTurnOutcomeDedupesLiveRedelivery|TestGetPromptActivityForSessionReturnsTrackedSnapshot|TestExecutionStore_ClaimPromptActivityRequiresCurrentIdentity' -count=1`
- `go test -trimpath -p 2 ./internal/orchestrator -run 'TestIdleParkingClockHonorsPolicyChangeAndFocus|TestIdleParkingKeepsDisabledAndKnownWorkSessionsRunning|TestIdleParkingSuspendsSettledSessionsAcrossACPProviders|TestPromptAfterIdleReclaimLaunchesPreparedSession' -count=1`
- Explicit temporary overlay checks for normal upsert/recovery, metadata-only update independence, retained completion and durable replay, full-interval fallback, and same-session isolation. Use the existing repository/manager fixtures and clean the overlay sources afterward.
- `go build -trimpath -p 2 ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/agentctl/server/instance ./internal/agentctl/types/streams`
- `go vet -trimpath -p 2 ./internal/orchestrator ./internal/agent/runtime/lifecycle ./internal/agentctl/server/instance ./internal/agentctl/types/streams`
- Root: existing `scripts/lint-go-changed` scoped by changed package; existing specification metadata and `.github/scripts/pr-docs.cjs` coverage check for these changed documents.

## Results

Independent source review also identified that remote startup reconstruction bypassed the local restoration path. The remote constructor now uses the same helper on the lifecycle-owned record after identity validation. A temporary normal SQLite upsert/readback to the real remote constructor and back to persistence failed before the fix across SSH, Sprites, remote Docker, Kubernetes, and plugin-remote variants, then passed all five (`3.067s`), alongside the existing remote inventory and run-owner-boundary tests. Adapter metadata intentionally omitted the clock, proving the authoritative record is consumed. No remote ownership, attach guard, or provider exclusion changed.

The eleven production paths are implemented on the stated base. Normal upsert to a fresh `Manager.Start` recovery retained the activity clock despite message-metadata and runtime-row updates. Retained completion, durable replay through completion, payload identity/cursor, no-history timing, producer readback, snapshot isolation, and persistence of an explicitly cleared clock were checked with nine temporary cases (including five remote runtime variants). Existing lifecycle four, idle/focus/known-work/token four, and instance-retention five checks passed. Narrow build and vet returned exit 0.

The baseline overlay failed the intended persisted-clock, retained-replay-time, passive-row-reset, and zero-history-skip assertions. Two temporary fixtures initially lacked normal parent/turn rows or repository-normalized idle-state readback; only those affected cases were corrected and rerun. These fixture failures were not production defects and the mixed initial command was not reported as PASS. Actual tool stdout/exit receipts are retained in the internal handoff record; the temporary sources, overlays, and baseline blobs were removed.

Lifecycle, agentctl, and orchestrator scoped lint returned `0 issues.` with exit 0. The first orchestrator check timed out during cold package loading and was not a lint PASS; the later check used the built cache and sequential package scopes. Specification metadata returned zero violations and the current PR-docs checker returned covered/errors empty for this work order.

Final source inspection identified that the prior-row metadata merge could restore a clock intentionally removed by an unknown replay. The same persistence file now excludes that lifecycle-owned key from prior fallback, keeping absence authoritative. An additional temporary normal-upsert to next-`Manager.Start` case failed before the correction and passed afterward, together with existing revoked-MCP-claim and upsert-error tests (`0.754s`). This change has its own focused persistence lint/vet readback after the earlier checks; it changes no public or workflow behavior beyond preserving the original no-history rule.

Compatibility: old runtime inventory without this snapshot and old retained outcomes without observation time receive a full interval from settled observation. This is the explicit no-history behavior, not inferred transcript activity. No operational process, task, profile, or settings were changed. No permanent test files were added or existing tests removed.

Historical candidate author reports are not current-head PASS; review `702a5803-db48-444d-85c5-875a5bd44ed5` failed technically with file_count 0. Independent source review and normal Git integration remain with the current root; original issue-first publication remains with the existing upstream publisher. The original goal and guards remain; only the untrustworthy transcript fallback and unauthorized standing-test additions were excluded, with six necessary existing persistence/recovery owner seams added as explained above.
